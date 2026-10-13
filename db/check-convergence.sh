#!/bin/sh
# schema.sql <-> migration-chain convergence check (review MIG-03).
#
# A fresh volume is built from db/schema.sql (initdb) and then runs every
# migration on top; an existing volume only runs migrations. Both paths must
# land on the same structure — i.e. schema.sql has to be kept current with
# the chain, so each migration is a structural no-op against it. This builds
# two scratch databases, compares columns/indexes/constraints, and fails on
# any drift. Runs inside the postgres image against the compose db; needs
# CREATEDB rights for DB_USER (the compose superuser has them).
#
# Environment: PGPASSWORD (required), DB_HOST, DB_USER.
set -eu

DB_HOST="${DB_HOST:-db}"
DB_USER="${DB_USER:-computer}"
[ -n "${PGPASSWORD:-}" ] || { echo "error: PGPASSWORD is required" >&2; exit 1; }

BASE_DB=computer_conv_schema
CHAIN_DB=computer_conv_chain

psql() { command psql -h "$DB_HOST" -U "$DB_USER" -v ON_ERROR_STOP=1 "$@"; }

drop() { psql -q -d postgres -c "DROP DATABASE IF EXISTS $1 WITH (FORCE)" >/dev/null 2>&1 || true; }
drop "$BASE_DB"; drop "$CHAIN_DB"
trap 'drop "$BASE_DB"; drop "$CHAIN_DB"' EXIT INT TERM

psql -q -d postgres -c "CREATE DATABASE $BASE_DB"
psql -q -d postgres -c "CREATE DATABASE $CHAIN_DB"

# Path 1: schema.sql alone. Path 2: schema.sql + the full ledger chain
# (exactly what initdb + migrate.sh do on a fresh install). schema.sql
# carries its own BEGIN/COMMIT, so no -1 wrapper here.
psql -q -d "$BASE_DB" -f /db/schema.sql
psql -q -d "$CHAIN_DB" -f /db/schema.sql
DB_NAME="$CHAIN_DB" sh /db/migrate.sh >/dev/null

# Structural catalog, minus the ledger itself (chain path only).
catalog() {
  psql -Atq -d "$1" -c "
    SELECT 'column '||table_name||'.'||column_name||' '||data_type||' '||is_nullable||' '||COALESCE(column_default,'-')
      FROM information_schema.columns
     WHERE table_schema='public' AND table_name<>'_migration'
    UNION ALL
    SELECT 'index '||indexname||': '||indexdef FROM pg_indexes
     WHERE schemaname='public' AND tablename<>'_migration'
    UNION ALL
    SELECT 'constraint '||c.relname||' '||con.conname||': '||pg_get_constraintdef(con.oid)
      FROM pg_constraint con JOIN pg_class c ON c.oid=con.conrelid
     WHERE con.connamespace='public'::regnamespace AND c.relname<>'_migration'
    UNION ALL
    SELECT 'sequence '||sequence_name FROM information_schema.sequences
     WHERE sequence_schema='public'
    ORDER BY 1"
}

catalog "$BASE_DB" > /tmp/base.catalog
catalog "$CHAIN_DB" > /tmp/chain.catalog

fail() {
  echo "error: db/schema.sql and db/migrations disagree (diff above)." >&2
  echo "       Update schema.sql (fresh installs) AND ship a new numbered migration (existing installs)." >&2
  exit 1
}

# Pass 1: the chain is a structural no-op against the current schema.sql.
diff -u /tmp/base.catalog /tmp/chain.catalog || fail

# Pass 2 — upgraded-install simulation: pass 1 alone cannot see the DDL
# inside CREATE TABLE IF NOT EXISTS blocks (schema.sql always wins there).
# Drop every migration-created table, wipe the ledger, and re-run the chain
# so those definitions materialize for real, then compare again. This is what
# an existing install upgraded through the ledger ends up with. Caveat: if a
# schema.sql table ever gains an FK to a migration-created table, the CASCADE
# drop removes that constraint and this check fails conservatively.
tables=$(sed -n 's/.*CREATE TABLE IF NOT EXISTS "\([^"]*\)".*/\1/p' /db/migrations/*.sql | sort -u)
if [ -n "$tables" ]; then
  for tb in $tables; do
    psql -q -d "$CHAIN_DB" -c "DROP TABLE IF EXISTS \"$tb\" CASCADE"
  done
  psql -q -d "$CHAIN_DB" -c "DELETE FROM _migration"
  DB_NAME="$CHAIN_DB" sh /db/migrate.sh >/dev/null
  catalog "$CHAIN_DB" > /tmp/chain.catalog
  diff -u /tmp/base.catalog /tmp/chain.catalog || fail
fi

echo "schema.sql and the migration chain converge (fresh and upgraded paths)"
