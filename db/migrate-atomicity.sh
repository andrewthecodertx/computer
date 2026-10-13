#!/bin/sh
# Proves migrate.sh apply+record atomicity (review MIG-03). Runs inside the
# converge service (postgres image, /db mounted) against the compose db, on
# scratch databases it creates and drops; app data is never touched. Invoked
# by 'make db-converge' alongside the schema convergence check.
set -u
FAIL_DB=computer_conv_atomicity
PSQL="psql -h db -U computer -v ON_ERROR_STOP=1"
$PSQL -d postgres -qc "DROP DATABASE IF EXISTS $FAIL_DB WITH (FORCE)"
$PSQL -d postgres -qc "CREATE DATABASE $FAIL_DB"
mkdir -p /tmp/migs

echo "--- case 1: failing migration leaves no ledger row and no partial DDL"
printf 'CREATE TABLE conv_probe(id text);\nSELECT 1/0;\n' > /tmp/migs/999_probe.sql
MIGRATIONS=/tmp/migs DB_HOST=db DB_USER=computer DB_NAME=$FAIL_DB sh /db/migrate.sh
rc=$?
echo "migrate exit=$rc"
[ "$rc" -ne 0 ] || { echo "FAIL: broken migration exited 0"; exit 1; }
rows=$($PSQL -At -d $FAIL_DB -c "SELECT count(*) FROM _migration")
probe=$($PSQL -At -d $FAIL_DB -c "SELECT count(*) FROM pg_tables WHERE tablename='conv_probe'")
[ "$rows" = "0" ] || { echo "FAIL: ledger recorded a rolled-back migration ($rows)"; exit 1; }
[ "$probe" = "0" ] || { echo "FAIL: partial DDL survived the rollback"; exit 1; }
echo "case 1 OK"

echo "--- case 2: successful migration applies and records atomically"
printf 'CREATE TABLE conv_probe(id text);\n' > /tmp/migs/999_probe.sql
MIGRATIONS=/tmp/migs DB_HOST=db DB_USER=computer DB_NAME=$FAIL_DB sh /db/migrate.sh
rc=$?
[ "$rc" -eq 0 ] || { echo "FAIL: valid migration exited $rc"; exit 1; }
rows=$($PSQL -At -d $FAIL_DB -c "SELECT count(*) FROM _migration")
probe=$($PSQL -At -d $FAIL_DB -c "SELECT count(*) FROM pg_tables WHERE tablename='conv_probe'")
{ [ "$rows" = "1" ] && [ "$probe" = "1" ]; } || { echo "FAIL: apply/record mismatch rows=$rows probe=$probe"; exit 1; }
echo "case 2 OK"

echo "--- case 3: re-run is a no-op; tampered checksum aborts before applying"
MIGRATIONS=/tmp/migs DB_HOST=db DB_USER=computer DB_NAME=$FAIL_DB sh /db/migrate.sh || { echo "FAIL: idempotent re-run failed"; exit 1; }
printf 'CREATE TABLE conv_probe2(id text);\n' > /tmp/migs/999_probe.sql
MIGRATIONS=/tmp/migs DB_HOST=db DB_USER=computer DB_NAME=$FAIL_DB sh /db/migrate.sh
rc=$?
[ "$rc" -ne 0 ] || { echo "FAIL: tampered migration did not abort"; exit 1; }
probe2=$($PSQL -At -d $FAIL_DB -c "SELECT count(*) FROM pg_tables WHERE tablename='conv_probe2'")
[ "$probe2" = "0" ] || { echo "FAIL: tampered migration applied anyway"; exit 1; }
echo "case 3 OK"

echo "--- case 4: concurrent runners — exactly one applies, the loser aborts"
printf 'CREATE TABLE conv_race(id text);\n' > /tmp/migs/999_probe.sql
RACE_DB=computer_conv_race
$PSQL -d postgres -qc "DROP DATABASE IF EXISTS $RACE_DB WITH (FORCE)"
$PSQL -d postgres -qc "CREATE DATABASE $RACE_DB"
MIGRATIONS=/tmp/migs DB_HOST=db DB_USER=computer DB_NAME=$RACE_DB sh /db/migrate.sh >/tmp/race1.log 2>&1 &
p1=$!
MIGRATIONS=/tmp/migs DB_HOST=db DB_USER=computer DB_NAME=$RACE_DB sh /db/migrate.sh >/tmp/race2.log 2>&1 &
p2=$!
rc1=0; rc2=0
wait $p1 || rc1=$?
wait $p2 || rc2=$?
# The loser may abort loudly (primary-key violation after the lock handoff)
# or no-op via the ledger check; what must hold: at least one succeeds, the
# ledger has exactly one row, and the file applied exactly once.
{ [ $rc1 -eq 0 ] || [ $rc2 -eq 0 ]; } || { echo "FAIL: both runners failed"; cat /tmp/race1.log /tmp/race2.log; exit 1; }
rows=$($PSQL -At -d $RACE_DB -c "SELECT count(*) FROM _migration")
race=$($PSQL -At -d $RACE_DB -c "SELECT count(*) FROM pg_tables WHERE tablename='conv_race'")
{ [ "$rows" = "1" ] && [ "$race" = "1" ]; } || { echo "FAIL: race left rows=$rows table=$race"; cat /tmp/race1.log /tmp/race2.log; exit 1; }
echo "case 4 OK"
$PSQL -d postgres -qc "DROP DATABASE IF EXISTS $RACE_DB WITH (FORCE)"

$PSQL -d postgres -qc "DROP DATABASE IF EXISTS $FAIL_DB WITH (FORCE)"
echo "ATOMICITY TESTS PASSED"
