#!/usr/bin/env bash
# Deploy the computer stack to a target server of your choice.
#
# Run from anywhere in the repository on your workstation. The deployment
# target is explicit — there are no built-in host or domain defaults:
#
#   HOST=app-server DOMAIN=computer.example.tld CERTBOT_EMAIL=ops@example.tld \
#     ./scripts/deploy.sh          (or: make deploy with the same variables)
#
# Flow: record the exact git revision, rsync the source tree to the server,
# then run scripts/server-deploy.sh there under sudo (one password prompt) to
# provision packages, back up the database, build the Compose stack (images
# are tagged with the revision), and configure nginx + Let's Encrypt from
# deploy/nginx/computer.conf.template.
#
# Review OPS-04: a release is a known revision — dirty working trees are
# refused unless ALLOW_DIRTY=1 acknowledges them, the revision is recorded on
# the server (.deployed-revision) and the built images are tagged with it.
# Rollback: re-deploy an older revision (git checkout <rev> && ./scripts/deploy.sh,
# which reuses/rebuilds that known source), or restore one of the last 15
# pre-deploy pg_dumps (+ its .env secret snapshot) from /var/backups/computer
# on the server. Database migrations are additive.
#
# Requirements: ssh access to HOST (an ssh config alias works well), rsync and
# git locally. DNS for DOMAIN must already point at the server (certbot
# HTTP-01). Optional overrides: REMOTE_DIR (default /var/www/computer),
# ALLOW_DIRTY=1.
set -euo pipefail

usage="usage: HOST=ssh-host DOMAIN=app.domain.tld CERTBOT_EMAIL=ops@domain.tld ./scripts/deploy.sh"
HOST="${HOST:?${usage}}"
DOMAIN="${DOMAIN:?${usage}}"
CERTBOT_EMAIL="${CERTBOT_EMAIL:?${usage}}"
REMOTE_DIR="${REMOTE_DIR:-/var/www/computer}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

# --- 0. Record the revision being deployed (review OPS-04) ----------------------
REV="$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null)" || die "git is required to deploy a known revision"
if [ -n "$(git -C "$REPO_ROOT" status --porcelain)" ]; then
  [ "${ALLOW_DIRTY:-0}" = "1" ] || die "working tree has uncommitted changes; commit or stash them so the release is a known revision (ALLOW_DIRTY=1 overrides)"
  REV="$REV-dirty"
  log "warning: deploying a DIRTY working tree as $REV"
fi
log "Deploying revision $REV"

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
  --exclude '.deployed-revision' \
  --exclude 'node_modules' \
  --exclude 'web/.next' \
  --exclude 'web/out' \
  --exclude 'web/*.tsbuildinfo' \
  --exclude '*.log' \
  "$REPO_ROOT/" "$HOST:$REMOTE_DIR/"

# --- 3. Run the server-side deploy as root (may prompt for your sudo password) --
log "Running server-side deploy (backup + build + nginx + TLS)"
ssh -t "$HOST" "sudo bash '$REMOTE_DIR/scripts/server-deploy.sh' '$REMOTE_DIR' '$DOMAIN' '$CERTBOT_EMAIL' '$REMOTE_USER' '$REV'"

# Server-owned release marker (excluded from rsync --delete above).
ssh "$HOST" "printf '%s\n' '$REV' > '$REMOTE_DIR/.deployed-revision'"

# --- 4. Final check from this machine ---------------------------------------------
log "Verifying https://$DOMAIN/login from this machine"
if curl -fsS -o /dev/null --max-time 15 "https://$DOMAIN/login"; then
  log "Deploy complete: https://$DOMAIN (revision $REV)"
else
  cat >&2 <<EOF
warning: https://$DOMAIN/login is not reachable from this machine even though
the server-side checks passed. This usually means DNS for $DOMAIN does not
point at the server from here, or a firewall blocks 80/443. Verify with:
  dig +short $DOMAIN
EOF
  exit 1
fi
