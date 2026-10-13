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

if diff -u /tmp/base.catalog /tmp/chain.catalog; then
  echo "schema.sql and the migration chain converge"
else
  echo "error: db/schema.sql and db/migrations disagree (diff above)." >&2
  echo "       Update schema.sql (fresh installs) AND ship a new numbered migration (existing installs)." >&2
  exit 1
fi
