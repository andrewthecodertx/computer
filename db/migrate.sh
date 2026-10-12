#!/bin/sh
# Ordered, checksummed migration runner (review MIG-01).
#
# Applies every /db/migrations/*.sql file exactly once, in name order, each
# in its own transaction (psql -1 with ON_ERROR_STOP), and records it in the
# _migration ledger. A recorded file is immutable: if its checksum no longer
# matches, the runner aborts before applying anything else. Migration files
# must remain idempotent so a crash between apply and record is recoverable.
#
# Environment: PGPASSWORD (required), DB_HOST, DB_USER, DB_NAME, MIGRATIONS.
set -eu

DB_HOST="${DB_HOST:-db}"
DB_USER="${DB_USER:-computer}"
DB_NAME="${DB_NAME:-computer}"
MIGRATIONS="${MIGRATIONS:-/db/migrations}"
[ -n "${PGPASSWORD:-}" ] || { echo "error: PGPASSWORD is required" >&2; exit 1; }

psql() {
  command psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 "$@"
}

psql -qc 'CREATE TABLE IF NOT EXISTS _migration(name text PRIMARY KEY, checksum text NOT NULL, "appliedAt" timestamptz NOT NULL DEFAULT now())'

for f in "$MIGRATIONS"/*.sql; do
  [ -e "$f" ] || { echo "error: no migrations found in $MIGRATIONS" >&2; exit 1; }
  name="$(basename "$f")"
  sum="$(sha256sum "$f" | cut -d' ' -f1)"
  recorded="$(psql -Atc "SELECT checksum FROM _migration WHERE name='$name'")"
  if [ -n "$recorded" ]; then
    if [ "$recorded" != "$sum" ]; then
      echo "error: migration $name changed after being applied (ledger $recorded, file $sum)." >&2
      echo "       Applied migrations are immutable; ship the change as a new numbered file." >&2
      exit 1
    fi
    continue
  fi
  echo "applying $name"
  psql -1 -q -f "$f"
  # ON CONFLICT covers a crash between apply and record; files are idempotent.
  psql -qc "INSERT INTO _migration(name,checksum) VALUES('$name','$sum') ON CONFLICT(name) DO NOTHING"
done
echo "migrations up to date"
