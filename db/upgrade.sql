-- Idempotent, additive upgrade for the existing Docker database.
BEGIN;
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='Bookmark_contactId_fkey' AND conrelid='"Bookmark"'::regclass) THEN
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
COMMIT;
