#!/usr/bin/env bash
# Deploy the computer stack to a target server of your choice.
#
# Run from anywhere in the repository on your workstation. The deployment
# target is explicit — there are no built-in host or domain defaults:
#
#   HOST=app-server DOMAIN=computer.example.tld CERTBOT_EMAIL=ops@example.tld \
#     ./scripts/deploy.sh          (or: make deploy with the same variables)
#
# Flow: record the exact git revision, rsync the source tree to a UNIQUE
# staging directory on the server, then run scripts/server-deploy.sh there
# under sudo (one password prompt). The server script takes a deployment
# lock (flock) BEFORE publishing the staged tree into the shared release
# directory, provisions packages, backs up the database, builds the Compose
# stack (images are tagged with the revision), configures nginx + Let's
# Encrypt from deploy/nginx/computer.conf.template, and records the revision
# marker only after everything passed (review CR-11: overlapping deploys can
# no longer mix source and release state — synchronization targets are
# per-release, mutation of the shared directory is serialized, and the last
# successful marker survives failures).
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

# --- 1. Prepare the remote directories (may prompt for your sudo password) ------
# The release directory is shared and mutable; this deploy's staging directory
# is unique (rev + local pid), so overlapping rsyncs never touch shared
# release inputs (review CR-11).
STAGING_ROOT="$REMOTE_DIR-staging"
STAGE="$STAGING_ROOT/$REV-$$"
log "Ensuring $HOST:$REMOTE_DIR and $STAGING_ROOT exist and belong to $REMOTE_USER"
ssh -t "$HOST" "sudo sh -c 'mkdir -p \"$REMOTE_DIR\" \"$STAGING_ROOT\" && chown \"$REMOTE_USER:\" \"$REMOTE_DIR\" \"$STAGING_ROOT\"'"

# --- 2. Sync the source tree to this deploy's staging directory ----------------
log "Syncing repository to $HOST:$STAGE"
# --delete mirrors the repo into the (empty) stage; excluded files (a local
# .env, build output) never ship. The server-side publish step protects the
# server's own .env and release marker.
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
  "$REPO_ROOT/" "$HOST:$STAGE/"

# --- 3. Run the server-side deploy as root (may prompt for your sudo password) --
# The STAGED copy of the script runs (not the old one in $REMOTE_DIR): it
# takes the server-side deployment lock, publishes the stage into the shared
# release directory, and records .deployed-revision only after everything
# passed — a failed deploy preserves the last successful release marker
# (review CR-11/OPS-04).
log "Running server-side deploy (lock + publish + backup + build + nginx + TLS)"
ssh -t "$HOST" "sudo bash '$STAGE/scripts/server-deploy.sh' '$REMOTE_DIR' '$DOMAIN' '$CERTBOT_EMAIL' '$REMOTE_USER' '$REV' '$STAGE'"

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
