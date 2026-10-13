#!/usr/bin/env bash
# Server-side half of the deployment, run as root on the target server by
# scripts/deploy.sh:  ssh -t HOST sudo bash .../server-deploy.sh DIR DOMAIN EMAIL USER [REVISION]
#
# Provisions missing packages (docker, compose plugin, nginx, certbot),
# writes .env on first deploy, builds/starts the Compose stack, waits for
# health, and installs the nginx reverse proxy with a Let's Encrypt cert.
set -euo pipefail

REMOTE_DIR="${1:?usage: server-deploy.sh DIR DOMAIN EMAIL RUN_USER [REVISION]}"
DOMAIN="$2"
EMAIL="$3"
RUN_USER="$4"
REVISION="${5:-}"
# Host-side ports for the Compose bindings. The defaults from docker-compose.yml
# (8080/3000) collide with other apps on shared servers, so production pins
# these via .env; override here if 18080/13000 are ever taken too.
APP_HOST_PORT="${APP_HOST_PORT:-18080}"
WEB_HOST_PORT="${WEB_HOST_PORT:-13000}"
# Pinned upstream tool versions (review OPS-02): fallback downloads install
# exactly these releases and verify them against the checksums Docker
# publishes with the release. Overrides must be deliberate.
COMPOSE_VERSION="${COMPOSE_VERSION:-v5.6.0}"
BUILDX_VERSION="${BUILDX_VERSION:-v0.38.0}"

log() { printf '\033[1;34m[server]==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[server] error:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" = "0" ] || die "must run as root (the local deploy.sh does: ssh -t ... sudo bash $0)"
[ -f "$REMOTE_DIR/docker-compose.yml" ] || die "$REMOTE_DIR/docker-compose.yml missing - rsync first"

# --- 1. Provision packages ----------------------------------------------------
log "Checking required packages"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq

# Install one package, but only if apt actually has it (mirrors differ).
install_pkg() {
  apt-cache show "$1" >/dev/null 2>&1 || return 1
  apt-get install -y -qq "$1"
}

command -v docker >/dev/null 2>&1 || install_pkg docker.io || die "docker not installed and docker.io unavailable via apt"
if ! docker compose version >/dev/null 2>&1; then
  install_pkg docker-compose-v2 || install_pkg docker-compose-plugin || true
fi
command -v nginx >/dev/null 2>&1 || install_pkg nginx || die "nginx not installed and unavailable via apt"
if ! command -v certbot >/dev/null 2>&1; then
  install_pkg certbot || die "certbot unavailable via apt"
  install_pkg python3-certbot-nginx || die "python3-certbot-nginx unavailable via apt"
fi
if ! docker compose version >/dev/null 2>&1; then
  log "Compose plugin not in apt; installing pinned upstream $COMPOSE_VERSION (checksum-verified)"
  # GitHub release assets use uname -m names (x86_64/aarch64), not dpkg arches.
  arch="$(uname -m)"
  case "$arch" in
    x86_64 | aarch64 | armv7l | s390x | ppc64le) ;;
    *) die "no upstream Compose binary for architecture '$arch'" ;;
  esac
  asset="docker-compose-linux-$arch"
  tmp="$(mktemp -d)"
  curl -fsSL "https://github.com/docker/compose/releases/download/$COMPOSE_VERSION/$asset" -o "$tmp/$asset"
  curl -fsSL "https://github.com/docker/compose/releases/download/$COMPOSE_VERSION/$asset.sha256" -o "$tmp/$asset.sha256"
  (cd "$tmp" && sha256sum -c "$asset.sha256") || die "Compose $COMPOSE_VERSION checksum verification failed; refusing to install"
  mkdir -p /usr/local/libexec/docker/cli-plugins
  install -m 755 "$tmp/$asset" /usr/local/libexec/docker/cli-plugins/docker-compose
  rm -rf "$tmp"
  docker compose version >/dev/null 2>&1 || die "docker compose still unavailable"
fi

# Upstream compose builds via buildx and requires >= 0.17.0; Debian's
# docker.io ships an older (or no) buildx, so install the upstream plugin.
buildx_ok() {
  docker buildx version >/dev/null 2>&1 || return 1
  local v
  v="$(docker buildx version | sed -n 's/.* v\([0-9][0-9.]*\).*/\1/p' | head -n1)"
  [ -n "$v" ] || return 1
  [ "$(printf '%s\n0.17.0\n' "$v" | sort -V | head -n1)" = "0.17.0" ]
}
if ! buildx_ok; then
  log "Installing pinned upstream buildx $BUILDX_VERSION (compose build needs >= 0.17.0; checksum-verified)"
  case "$(uname -m)" in
    x86_64) barch=amd64 ;;
    aarch64) barch=arm64 ;;
    armv7l) barch=arm ;;
    *) die "no upstream buildx binary for architecture '$(uname -m)'" ;;
  esac
  asset="buildx-$BUILDX_VERSION.linux-$barch"
  tmp="$(mktemp -d)"
  curl -fsSL "https://github.com/docker/buildx/releases/download/$BUILDX_VERSION/$asset" -o "$tmp/$asset"
  curl -fsSL "https://github.com/docker/buildx/releases/download/$BUILDX_VERSION/checksums.txt" -o "$tmp/checksums.txt"
  # checksums.txt covers every asset; verify exactly the pinned one (exact
  # field match — a substring grep would also catch .provenance/.sbom assets
  # and fail the check).
  awk -v name="*$asset" '$2 == name' "$tmp/checksums.txt" > "$tmp/$asset.sha256"
  [ -s "$tmp/$asset.sha256" ] || die "no published checksum for $asset"
  (cd "$tmp" && sha256sum -c "$asset.sha256") || die "buildx $BUILDX_VERSION checksum verification failed; refusing to install"
  mkdir -p /usr/local/libexec/docker/cli-plugins
  install -m 755 "$tmp/$asset" /usr/local/libexec/docker/cli-plugins/docker-buildx
  rm -rf "$tmp"
  buildx_ok || die "docker buildx still unavailable or too old"
fi
# Record what provisioning settled on (review OPS-02).
log "compose plugin: $(docker compose version 2>/dev/null | head -n1)"
log "buildx plugin: $(docker buildx version 2>/dev/null | head -n1)"
systemctl enable --now docker nginx
# Let RUN_USER drive docker without sudo for day-to-day commands.
id -nG "$RUN_USER" | tr ' ' '\n' | grep -qx docker || usermod -aG docker "$RUN_USER"

# --- 2. .env (created once; never touch AUTH_SECRET afterwards) ---------------
cd "$REMOTE_DIR"
if [ ! -f .env ]; then
  # A lost .env while the data volume remains would generate a fresh
  # POSTGRES_PASSWORD that the existing database does not have, locking the
  # app out of its own data. Refuse instead.
  if docker volume inspect computer_pgdata >/dev/null 2>&1; then
    die ".env is missing but the computer_pgdata volume exists - restore the .env backup (it holds POSTGRES_PASSWORD and AUTH_SECRET); refusing to regenerate"
  fi
  log "Creating .env with generated secrets (back it up with the database!)"
  umask 077
  cat > .env <<EOF
POSTGRES_PASSWORD=$(openssl rand -hex 24)
AUTH_SECRET=$(openssl rand -hex 32)
NEXTAUTH_URL=https://$DOMAIN
COOKIE_SECURE=1
ADMIN_EMAILS=
POSTGRES_VOLUME=computer_pgdata
POSTGRES_VOLUME_EXTERNAL=false
APP_HOST_PORT=$APP_HOST_PORT
WEB_HOST_PORT=$WEB_HOST_PORT
OIDC_ISSUER=REPLACE_ME
OIDC_CLIENT_ID=REPLACE_ME
OIDC_CLIENT_SECRET=REPLACE_ME
EOF
  umask 022
else
  log "Keeping existing .env"
  grep -q '^AUTH_SECRET=..*' .env || die ".env exists but AUTH_SECRET is empty"
  # Pin the host ports even on an .env created before port overrides existed.
  grep -q '^APP_HOST_PORT=' .env || echo "APP_HOST_PORT=$APP_HOST_PORT" >> .env
  grep -q '^WEB_HOST_PORT=' .env || echo "WEB_HOST_PORT=$WEB_HOST_PORT" >> .env
  url="$(sed -n 's/^NEXTAUTH_URL=//p' .env | tail -n1)"
  secure="$(sed -n 's/^COOKIE_SECURE=//p' .env | tail -n1)"
  [ "$url" = "https://$DOMAIN" ] || log "warning: NEXTAUTH_URL is '$url', expected 'https://$DOMAIN'"
  [ "$secure" = "1" ] || log "warning: COOKIE_SECURE is '$secure', expected '1' behind HTTPS"
fi
# Day-to-day `docker compose` runs as RUN_USER, which must be able to read .env.
chown "$RUN_USER:" .env
chmod 600 .env

# Effective ports are whatever .env says (they may predate this run).
APP_PORT="$(sed -n 's/^APP_HOST_PORT=//p' .env | tail -n1)"; APP_PORT="${APP_PORT:-8080}"
WEB_PORT="$(sed -n 's/^WEB_HOST_PORT=//p' .env | tail -n1)"; WEB_PORT="${WEB_PORT:-3000}"

# Fail early with a clear message if another process squats the ports while
# our stack is not the thing holding them.
port_busy() { ss -tln | awk '{print $4}' | grep -qE "[:.]$1\$"; }
if [ -z "$(docker compose ps -q app 2>/dev/null)" ]; then
  port_busy "$APP_PORT" && die "port $APP_PORT is already in use by another process - set APP_HOST_PORT and re-run"
  port_busy "$WEB_PORT" && die "port $WEB_PORT is already in use by another process - set WEB_HOST_PORT and re-run"
fi

# --- 2b. Back up the database before touching the stack -------------------------
# The volume survives rebuilds, but a volume is not a backup: take a pg_dump
# before every deploy and keep the 15 most recent dumps plus matching .env
# secret snapshots (review OPS-03: AUTH_SECRET/ENCRYPTION_KEY are required to
# decrypt integration data when restoring). Skipping is never silent: an
# existing database that is not running gets started for the backup, and a
# failed dump aborts the deploy. Dumps are 0600 in a 0700 directory; copy
# them off-host — a same-disk backup does not survive disk loss.
BACKUP_DIR="${BACKUP_DIR:-/var/backups/computer}"
pg_volume="$(sed -n 's/^POSTGRES_VOLUME=//p' .env 2>/dev/null | tail -n1)"; pg_volume="${pg_volume:-computer_pgdata}"
have_existing_db() {
  [ -n "$(docker compose ps -aq db 2>/dev/null)" ] || docker volume inspect "$pg_volume" >/dev/null 2>&1
}
if have_existing_db; then
  if ! docker compose ps --status running -q db 2>/dev/null | grep -q .; then
    log "Database exists but is not running; starting it so the pre-deploy backup actually happens"
    docker compose up -d db
    db_deadline=$((SECONDS + 90))
    until [ "$(docker inspect -f '{{.State.Health.Status}}' "$(docker compose ps -q db)" 2>/dev/null)" = "healthy" ]; do
      [ $SECONDS -lt $db_deadline ] || die "database never became healthy for the pre-deploy backup"
      sleep 3
    done
  fi
  log "Backing up database to $BACKUP_DIR"
  mkdir -p "$BACKUP_DIR"
  chmod 700 "$BACKUP_DIR"
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  dump="$BACKUP_DIR/computer-$stamp.sql.gz"
  if docker compose exec -T db pg_dump -U computer -d computer | gzip > "$dump"; then
    chmod 600 "$dump"
    # Capture the secrets required to decrypt/restore integration data.
    cp -p .env "$BACKUP_DIR/env-$stamp.bak"
    chmod 600 "$BACKUP_DIR/env-$stamp.bak"
    ls -1t "$BACKUP_DIR"/computer-*.sql.gz 2>/dev/null | tail -n +16 | xargs -r rm -f
    ls -1t "$BACKUP_DIR"/env-*.bak 2>/dev/null | tail -n +16 | xargs -r rm -f
    log "Backup written: $dump ($(du -h "$dump" | cut -f1)) + env-$stamp.bak; copy both off-host"
  else
    rm -f "$dump"
    die "pg_dump failed - aborting deploy to protect the database"
  fi
else
  log "No existing database container or volume ($pg_volume) found - first deploy, nothing to back up"
fi

# --- 3. Build and start the stack ---------------------------------------------
log "docker compose up -d --build (first build takes several minutes)"
docker compose up -d --build
docker compose ps -a
# Retain a named artifact of this release for rollback (review OPS-04):
# tag the built images with the deployed revision when deploy.sh passed one.
if [ -n "$REVISION" ]; then
  for svc in app web; do
    img="$(docker compose images -q "$svc" 2>/dev/null | head -n1)"
    if [ -n "$img" ]; then
      docker tag "$img" "computer-$svc:$REVISION"
      log "tagged computer-$svc:$REVISION"
    fi
  done
fi

# --- 4. Wait for health ---------------------------------------------------------
log "Waiting for app health and web /login"
deadline=$((SECONDS + 300))
app_id="$(docker compose ps -q app)"
until [ -n "$app_id" ] && [ "$(docker inspect -f '{{.State.Health.Status}}' "$app_id" 2>/dev/null)" = "healthy" ]; do
  [ $SECONDS -lt $deadline ] || { docker compose ps -a; docker compose logs --tail=50 app web; die "app never became healthy"; }
  sleep 3
done
until curl -fsS -o /dev/null "http://127.0.0.1:$WEB_PORT/login"; do
  [ $SECONDS -lt $deadline ] || die "web never answered on 127.0.0.1:$WEB_PORT";
  sleep 3
done
log "Stack is healthy"

# --- 5. nginx reverse proxy -------------------------------------------------------
log "Installing nginx config for $DOMAIN"
TEMPLATE="$REMOTE_DIR/deploy/nginx/computer.conf.template"
[ -f "$TEMPLATE" ] || die "missing $TEMPLATE (should have been rsynced)"
# Render the generic template with this deployment's domain and the server's
# actual WEB_HOST_PORT binding.
RENDERED="/tmp/$DOMAIN.$$.conf"
sed -e "s|__DOMAIN__|$DOMAIN|g" -e "s|__WEB_PORT__|$WEB_PORT|g" "$TEMPLATE" > "$RENDERED"
if [ -d /etc/nginx/sites-available ]; then
  install -m 644 "$RENDERED" "/etc/nginx/sites-available/$DOMAIN"
  ln -sfn "/etc/nginx/sites-available/$DOMAIN" "/etc/nginx/sites-enabled/$DOMAIN"
  SITE_CONF="/etc/nginx/sites-available/$DOMAIN"
else
  install -m 644 "$RENDERED" "/etc/nginx/conf.d/$DOMAIN.conf"
  SITE_CONF="/etc/nginx/conf.d/$DOMAIN.conf"
fi
rm -f "$RENDERED"

if [ ! -f "/etc/letsencrypt/live/$DOMAIN/fullchain.pem" ]; then
  # HTTP-01 needs $DOMAIN to resolve to this server before certbot runs.
  server_ip="$(curl -fsS --max-time 10 https://api.ipify.org || true)"
  dns_ip="$(getent ahostsv4 "$DOMAIN" | awk '{print $1; exit}')"
  if [ -n "$server_ip" ] && [ "$dns_ip" != "$server_ip" ]; then
    die "$DOMAIN resolves to '$dns_ip' but this server is '$server_ip' - fix DNS, then re-run"
  fi
  log "No certificate yet: bootstrapping via HTTP-only config + certbot"
  # Park the TLS config (nginx -t fails on missing cert files) and serve :80.
  mv "$SITE_CONF" "/etc/nginx/$DOMAIN.conf.pending"
  rm -f "/etc/nginx/sites-enabled/$DOMAIN"
  cat > "/etc/nginx/conf.d/$DOMAIN-bootstrap.conf" <<BOOT
server {
    listen 80;
    listen [::]:80;
    server_name $DOMAIN;
    location / { proxy_pass http://127.0.0.1:$WEB_PORT; proxy_set_header Host \$host; }
}
BOOT
  nginx -t
  systemctl reload nginx
  certbot certonly --nginx -d "$DOMAIN" --non-interactive --agree-tos -m "$EMAIL" --keep-until-expiring
  rm -f "/etc/nginx/conf.d/$DOMAIN-bootstrap.conf"
  mv "/etc/nginx/$DOMAIN.conf.pending" "$SITE_CONF"
  [ -d /etc/nginx/sites-available ] && ln -sfn "$SITE_CONF" "/etc/nginx/sites-enabled/$DOMAIN"
  # Reload nginx after each successful renewal.
  if [ ! -e /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh ]; then
    mkdir -p /etc/letsencrypt/renewal-hooks/deploy
    printf '#!/bin/sh\nsystemctl reload nginx\n' > /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh
    chmod +x /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh
  fi
fi

nginx -t
systemctl reload nginx

# --- 6. End-to-end check through nginx --------------------------------------------
log "Verifying https://$DOMAIN/login via nginx"
# Retry briefly: right after a reload, early connections can still hit the
# previous config's default 443 vhost (wrong cert / connection refused).
ok=""
for _ in $(seq 10); do
  if curl -fsS -o /dev/null --resolve "$DOMAIN:443:127.0.0.1" "https://$DOMAIN/login"; then
    ok=1; break
  fi
  sleep 3
done
[ -n "$ok" ] || die "nginx -> web TLS check failed after retries"
log "Server-side deploy complete"
