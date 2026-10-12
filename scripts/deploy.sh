#!/usr/bin/env bash
# Deploy the computer stack to eidcweb3:/var/www/computer.
#
# Run from anywhere in the repository on your workstation:
#
#   ./scripts/deploy.sh          (or: make deploy)
#
# Flow: rsync the source tree to the server, then run scripts/server-deploy.sh
# there under sudo (one password prompt) to provision packages, build the
# Compose stack, and configure nginx + Let's Encrypt for the domain.
#
# Requirements: ssh access to eidcweb3, rsync locally.
# Overrides: HOST=... DOMAIN=... CERTBOT_EMAIL=... ./scripts/deploy.sh
set -euo pipefail

HOST="${HOST:-eidcweb3}"
REMOTE_DIR="${REMOTE_DIR:-/var/www/computer}"
DOMAIN="${DOMAIN:-computer.erwininteractive.com}"
CERTBOT_EMAIL="${CERTBOT_EMAIL:-admin@erwininteractive.com}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

REMOTE_USER="$(ssh -G "$HOST" | awk '/^user /{print $2; exit}')"
[ -n "$REMOTE_USER" ] || die "could not determine ssh user for $HOST"
command -v rsync >/dev/null 2>&1 || die "rsync is required locally (e.g. sudo apt install rsync / sudo pacman -S rsync)"

# --- 1. Prepare the remote directory (may prompt for your sudo password) -------
log "Ensuring $HOST:$REMOTE_DIR exists and belongs to $REMOTE_USER"
ssh -t "$HOST" "sudo sh -c 'mkdir -p \"$REMOTE_DIR\" && chown \"$REMOTE_USER:\" \"$REMOTE_DIR\"'"

# --- 2. Sync the source tree ----------------------------------------------------
log "Syncing repository to $HOST:$REMOTE_DIR"
# --delete mirrors the repo; excluded files (the server's .env, build output)
# are protected from deletion.
rsync -az --delete \
  --exclude '.git' \
  --exclude '.env' \
  --exclude '.env.*' \
  --exclude 'node_modules' \
  --exclude 'web/.next' \
  --exclude 'web/out' \
  --exclude 'web/*.tsbuildinfo' \
  --exclude '*.log' \
  "$REPO_ROOT/" "$HOST:$REMOTE_DIR/"

# --- 3. Run the server-side deploy as root (may prompt for your sudo password) --
log "Running server-side deploy (build + nginx + TLS)"
ssh -t "$HOST" "sudo bash '$REMOTE_DIR/scripts/server-deploy.sh' '$REMOTE_DIR' '$DOMAIN' '$CERTBOT_EMAIL' '$REMOTE_USER'"

# --- 4. Final check from this machine ---------------------------------------------
log "Verifying https://$DOMAIN/login from this machine"
if curl -fsS -o /dev/null --max-time 15 "https://$DOMAIN/login"; then
  log "Deploy complete: https://$DOMAIN"
else
  cat >&2 <<EOF
warning: https://$DOMAIN/login is not reachable from this machine even though
the server-side checks passed. This usually means DNS for $DOMAIN does not
point at the server from here, or a firewall blocks 80/443. Verify with:
  dig +short $DOMAIN
EOF
  exit 1
fi
