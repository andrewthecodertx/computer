#!/bin/sh
# Ordered, checksummed migration runner (review MIG-01).
#
# Applies every /db/migrations/*.sql file exactly once, in name order, and
# records it in the _migration ledger. Apply and record share ONE transaction
# under a fixed advisory lock (review MIG-03): a crash can no longer leave an
# applied file unrecorded (or a recorded file unapplied), and a concurrent
# runner blocks on the lock, then aborts on the ledger primary key instead of
# double-applying. A recorded file is immutable: if its checksum no longer
# matches, the runner aborts before applying anything else.
#
# Migration files must therefore be transaction-safe and must NOT carry
# their own BEGIN/COMMIT (the runner wraps each file): statements that cannot
# run inside a transaction block (CREATE INDEX CONCURRENTLY, VACUUM, ...) are
# NOT supported by this runner — handle those as explicit operator steps
# outside the ledger. Files stay idempotent by convention (defense in depth;
# migration 001 must remain so for first ledger adoption on databases already
# upgraded via the legacy upgrade.sql).
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
  # One transaction: psql executes interleaved -c/-f options under -1 as a
  # single transaction (PostgreSQL >= 12). Take the lock (arbitrary fixed
  # key), record, then apply — any failure rolls back both. Deliberately no
  # ON CONFLICT: a runner that lost the race must fail loudly on the primary
  # key rather than silently re-execute an already-recorded file.
  psql -1 -q \
    -c 'DO $$ BEGIN PERFORM pg_advisory_xact_lock(742417); END $$' \
    -c "INSERT INTO _migration(name,checksum) VALUES('$name','$sum')" \
    -f "$f"
done
echo "migrations up to date"
