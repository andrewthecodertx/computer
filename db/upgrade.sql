-- Idempotent, additive upgrade for the existing Docker database.
BEGIN;
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='Bookmark_contactId_fkey' AND conrelid='"Bookmark"'::regclass) THEN
  ALTER TABLE "Bookmark" ADD CONSTRAINT "Bookmark_contactId_fkey" FOREIGN KEY ("contactId") REFERENCES "Contact"(id) ON DELETE SET NULL;
 END IF;
END $$;
COMMIT;
