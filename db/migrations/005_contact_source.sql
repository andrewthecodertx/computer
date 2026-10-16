-- 005_contact_source.sql — CardDAV provenance for deletion reconciliation
-- (review CR-07). Sync mirrors the configured address book, scoped by this
-- column: only contacts previously imported from the SAME source URL are
-- candidates for remote-deletion removal; manually created contacts (NULL)
-- and other address books' imports are never touched. Existing rows keep
-- NULL and are adopted (stamped with the source) the next time the server
-- returns them.
ALTER TABLE "Contact" ADD COLUMN IF NOT EXISTS source text;
