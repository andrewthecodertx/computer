-- 001_legacy_additive.sql — additive upgrades historically shipped as
-- db/upgrade.sql, adopted into the ordered ledger (review MIG-01).
-- Idempotent: every statement is guarded by an existence check, so first
-- ledger adoption on an already-upgraded database is a no-op. From ledger
-- adoption onward recorded migrations are immutable; new changes go into
-- new numbered files. The runner (db/migrate.sh) applies each file in its
-- own transaction with ON_ERROR_STOP.

-- Review MIG-02: report orphaned bookmark->contact references with an
-- actionable error before adding the foreign key, instead of failing with a
-- bare constraint violation. Never erase relationships silently — repair is
-- a deliberate operator decision.
DO $$
DECLARE orphans bigint;
BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='Bookmark_contactId_fkey' AND conrelid='"Bookmark"'::regclass) THEN
  SELECT count(*) INTO orphans FROM "Bookmark" b
   WHERE b."contactId" IS NOT NULL
     AND NOT EXISTS (SELECT 1 FROM "Contact" c WHERE c.id=b."contactId");
  IF orphans > 0 THEN
   RAISE EXCEPTION 'migration 001: % bookmark(s) reference missing Contact rows; repair them deliberately (e.g. UPDATE "Bookmark" SET "contactId"=NULL WHERE "contactId" IS NOT NULL AND NOT EXISTS (SELECT 1 FROM "Contact" c WHERE c.id="Bookmark"."contactId")) and re-run', orphans;
  END IF;
  ALTER TABLE "Bookmark" ADD CONSTRAINT "Bookmark_contactId_fkey" FOREIGN KEY ("contactId") REFERENCES "Contact"(id) ON DELETE SET NULL;
 END IF;
END $$;
-- Bring pre-port mailbox switches into the background signal scheduler.
-- Existing sources (including paused watches) and modern watchers are preserved.
INSERT INTO "SignalSource"(id,type,config,enabled,"bookmarkId","ownerId")
SELECT 'legacy-'||b.id,'email',json_build_object('subject',COALESCE(b."imapQuery",''),'legacy',true),true,b.id,b."ownerId"
FROM "Bookmark" b
WHERE b."imapWatchEnabled" AND NOT EXISTS (
 SELECT 1 FROM "SignalSource" s WHERE s."bookmarkId"=b.id AND s."ownerId"=b."ownerId" AND s.config->>'legacy'='true'
)
ON CONFLICT(id) DO NOTHING;
CREATE INDEX IF NOT EXISTS "Bookmark_ownerId_updatedAt_id_idx" ON "Bookmark"("ownerId","updatedAt" DESC,id DESC);
-- Revocable sessions: cookie JWTs embed this version; signout bumps it.
ALTER TABLE "User" ADD COLUMN IF NOT EXISTS "tokenVersion" integer NOT NULL DEFAULT 1;
-- Query-pattern indexes: shared-date visibility join and the alerts poll.
CREATE INDEX IF NOT EXISTS "SharedDate_creatorId_recipientId_date_idx" ON "SharedDate"("creatorId","recipientId",date);
CREATE INDEX IF NOT EXISTS "Bookmark_ownerId_alertAt_unsent_idx" ON "Bookmark"("ownerId","alertAt") WHERE NOT "alertSent";
-- Prisma's Json maps to jsonb; enables GIN indexing and dedups keys.
-- Re-running on an already-jsonb column is a validated no-op.
ALTER TABLE "SignalSource" ALTER COLUMN config TYPE jsonb USING config::jsonb;
-- Optimistic concurrency for page saves (review DATA-01): content saves bump
-- this revision and a stale base version in PATCH is rejected with 409.
ALTER TABLE "Page" ADD COLUMN IF NOT EXISTS version integer NOT NULL DEFAULT 1;
-- Durable per-source mailbox ingestion checkpoints (review SYNC-02): polls
-- process only UIDs newer than lastUid and advance only after durable inserts.
CREATE TABLE IF NOT EXISTS "SignalCheckpoint" (
  "sourceId" text PRIMARY KEY REFERENCES "SignalSource"(id) ON DELETE CASCADE,
  "uidValidity" bigint NOT NULL,
  "lastUid" bigint NOT NULL,
  "updatedAt" timestamptz NOT NULL DEFAULT now()
);
-- One-shot administrator bootstrap state (review AUTH-06): once recorded,
-- signup never auto-promotes again; recovery is operator-driven.
CREATE TABLE IF NOT EXISTS "AppSetting" (
  key text PRIMARY KEY,
  value jsonb NOT NULL DEFAULT '{}',
  "updatedAt" timestamptz NOT NULL DEFAULT now()
);
-- Installations that already have a qualifying administrator completed
-- bootstrap before this ledger existed; record it so demoting that
-- administrator cannot re-open first-signup promotion.
INSERT INTO "AppSetting"(key,value)
SELECT 'adminBootstrap',jsonb_build_object('completed',true)
WHERE EXISTS (SELECT 1 FROM "User" WHERE role='ADMIN' AND email NOT LIKE '%@example.com')
ON CONFLICT(key) DO NOTHING;
