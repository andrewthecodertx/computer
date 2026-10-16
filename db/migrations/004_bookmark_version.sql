-- 004_bookmark_version.sql — optimistic concurrency for bookmark updates
-- (review CR-04). Owner-driven PUT /api/bookmarks/{id} (fields and tag
-- replacement) now requires the expected version and bumps it; the
-- column-deletion move bumps it too, so a client holding a stale snapshot
-- gets a 409 instead of silently overwriting a concurrent edit. Sharing and
-- server-driven flags (alertSent) stay outside the contract.
ALTER TABLE "Bookmark" ADD COLUMN IF NOT EXISTS version integer NOT NULL DEFAULT 0;
