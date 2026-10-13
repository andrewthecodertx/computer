-- 002_collections.sql — collections, typed properties, memberships and saved
-- views (Notion-like upgrades phase 2: N-01/N-02/N-03 data layer).
--
-- Design decisions (computer_notion_upgrades.md §4, locked before shipping):
-- * Membership is contextual: the same bookmark can belong to many
--   collections with independent property values; removing a membership or a
--   whole collection never touches the bookmark (N-01).
-- * Membership never broadens access: item reads re-check BookmarkVisible at
--   query time, and creation validates visibility (decision #4, N-08).
-- * Views are queries, not copies: filters/sorts/grouping live on the view
--   and never duplicate record data (decision #3).
-- * Deviation from the proposal draft, per its own API rule ("PATCH must
--   require the expected version", the DATA-05 lesson): CollectionProperty,
--   CollectionItem and CollectionView carry a version column bumped by every
--   mutating PATCH; a stale base is a 409. Property rows also gained
--   updatedAt for the same reason.
-- * Property types are a CHECK constraint, not a Postgres enum (easier to
--   evolve). RELATION is reserved in the constraint but the API rejects it
--   until cross-collection access checks are designed (draft §6 open issue).
-- * Date semantics (decision #7): DATE properties declare config.mode
--   'date' (YYYY-MM-DD, calendar-day stable across timezones) or 'datetime'
--   (RFC3339, normalized to UTC on write). Values live in CollectionItem.
--   propertyValues (JSONB keyed by property id) and are validated server-side
--   against CollectionProperty on every write.

CREATE TABLE IF NOT EXISTS "Collection" (
  id text PRIMARY KEY,
  name text NOT NULL,
  description text,
  icon text,
  "ownerId" text NOT NULL REFERENCES "User"(id) ON DELETE CASCADE,
  version integer NOT NULL DEFAULT 1,
  "createdAt" timestamptz NOT NULL DEFAULT now(),
  "updatedAt" timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS "Collection_ownerId_idx" ON "Collection"("ownerId");

CREATE TABLE IF NOT EXISTS "CollectionProperty" (
  id text PRIMARY KEY,
  "collectionId" text NOT NULL REFERENCES "Collection"(id) ON DELETE CASCADE,
  name text NOT NULL,
  type text NOT NULL CHECK (type IN ('TEXT','NUMBER','SELECT','MULTI_SELECT','DATE','CHECKBOX','URL','RELATION')),
  position integer NOT NULL DEFAULT 0,
  config jsonb NOT NULL DEFAULT '{}',
  version integer NOT NULL DEFAULT 1,
  "createdAt" timestamptz NOT NULL DEFAULT now(),
  "updatedAt" timestamptz NOT NULL DEFAULT now(),
  UNIQUE ("collectionId", name)
);

CREATE TABLE IF NOT EXISTS "CollectionItem" (
  id text PRIMARY KEY,
  "collectionId" text NOT NULL REFERENCES "Collection"(id) ON DELETE CASCADE,
  "bookmarkId" text NOT NULL REFERENCES "Bookmark"(id) ON DELETE CASCADE,
  propertyValues jsonb NOT NULL DEFAULT '{}',
  position integer NOT NULL DEFAULT 0,
  version integer NOT NULL DEFAULT 1,
  "addedAt" timestamptz NOT NULL DEFAULT now(),
  "updatedAt" timestamptz NOT NULL DEFAULT now(),
  UNIQUE ("collectionId", "bookmarkId")
);
CREATE INDEX IF NOT EXISTS "CollectionItem_bookmarkId_idx" ON "CollectionItem"("bookmarkId");

CREATE TABLE IF NOT EXISTS "CollectionView" (
  id text PRIMARY KEY,
  "collectionId" text NOT NULL REFERENCES "Collection"(id) ON DELETE CASCADE,
  name text NOT NULL,
  type text NOT NULL DEFAULT 'TABLE' CHECK (type IN ('TABLE','BOARD','CALENDAR','LIST')),
  position integer NOT NULL DEFAULT 0,
  filters jsonb NOT NULL DEFAULT '{"operator":"AND","clauses":[]}',
  sorts jsonb NOT NULL DEFAULT '[]',
  "groupByPropertyId" text REFERENCES "CollectionProperty"(id) ON DELETE SET NULL,
  "displaySettings" jsonb NOT NULL DEFAULT '{}',
  version integer NOT NULL DEFAULT 1,
  "createdAt" timestamptz NOT NULL DEFAULT now(),
  "updatedAt" timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS "CollectionView_collectionId_idx" ON "CollectionView"("collectionId");
