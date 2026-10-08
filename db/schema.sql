-- computer database schema
-- Derived from computer_porting_guide.md section 2 and prisma/schema.prisma.
-- IDs are text (CUID/UUID). CASCADE / SetNull per the guide.

-- Run against an EMPTY database. Never drops or recreates a database.
BEGIN;

-- 2.1 legacy enum
CREATE TYPE "KanbanStatus" AS ENUM ('INBOX','TODO','IN_PROGRESS','DONE','ARCHIVED');

-- 2.2 User
CREATE TABLE "User" (
  id text PRIMARY KEY,
  name text,
  email text UNIQUE,
  "emailVerified" timestamptz,
  image text,
  password text,
  role text NOT NULL DEFAULT 'USER' CHECK (role IN ('USER','ADMIN')),
  "createdAt" timestamptz NOT NULL DEFAULT now(),
  "updatedAt" timestamptz NOT NULL DEFAULT now()
);

-- 2.3 Account, Session, VerificationToken
CREATE TABLE "Account" (
  id text PRIMARY KEY,
  "userId" text NOT NULL REFERENCES "User"(id) ON DELETE CASCADE,
  type text NOT NULL,
  provider text NOT NULL,
  "providerAccountId" text NOT NULL,
  "refresh_token" text,
  "access_token" text,
  "expires_at" int,
  "token_type" text,
  scope text,
  "id_token" text,
  "session_state" text,
  UNIQUE(provider, "providerAccountId")
);
CREATE INDEX "Account_userId_idx" ON "Account"("userId");

CREATE TABLE "Session" (
  id text PRIMARY KEY,
  "sessionToken" text NOT NULL UNIQUE,
  "userId" text NOT NULL REFERENCES "User"(id) ON DELETE CASCADE,
  expires timestamptz NOT NULL
);
CREATE INDEX "Session_userId_idx" ON "Session"("userId");

CREATE TABLE "VerificationToken" (
  identifier text NOT NULL,
  token text NOT NULL UNIQUE,
  expires timestamptz NOT NULL,
  UNIQUE(identifier, token)
);

-- 2.4 Bookmark
CREATE TABLE "Bookmark" (
  id text PRIMARY KEY,
  url text NOT NULL,
  title text,
  description text,
  favicon text,
  "ogImage" text,
  "ogTitle" text,
  "ogDescription" text,
  notes text,
  "dueDate" timestamptz,
  "alertAt" timestamptz,
  "alertSent" boolean NOT NULL DEFAULT false,
  "kanbanStatus" "KanbanStatus" NOT NULL DEFAULT 'INBOX',
  "kanbanColumnId" text,
  "imapWatchEnabled" boolean NOT NULL DEFAULT false,
  "imapQuery" text,
  "isPublic" boolean NOT NULL DEFAULT false,
  "ownerId" text NOT NULL REFERENCES "User"(id) ON DELETE CASCADE,
  "contactId" text,
  "createdAt" timestamptz NOT NULL DEFAULT now(),
  "updatedAt" timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX "Bookmark_ownerId_idx" ON "Bookmark"("ownerId");
CREATE INDEX "Bookmark_contactId_idx" ON "Bookmark"("contactId");
CREATE INDEX "Bookmark_kanbanStatus_idx" ON "Bookmark"("kanbanStatus");
CREATE INDEX "Bookmark_kanbanColumnId_idx" ON "Bookmark"("kanbanColumnId");
CREATE INDEX "Bookmark_dueDate_idx" ON "Bookmark"("dueDate");
CREATE INDEX "Bookmark_alertAt_idx" ON "Bookmark"("alertAt");

-- 2.5 Tag
CREATE TABLE "Tag" (
  id text PRIMARY KEY,
  name text NOT NULL,
  color text NOT NULL DEFAULT '#6366f1',
  "ownerId" text NOT NULL REFERENCES "User"(id) ON DELETE CASCADE,
  "createdAt" timestamptz NOT NULL DEFAULT now(),
  "updatedAt" timestamptz NOT NULL DEFAULT now(),
  UNIQUE(name, "ownerId")
);
CREATE INDEX "Tag_ownerId_idx" ON "Tag"("ownerId");

-- 2.6 BookmarkTag
CREATE TABLE "BookmarkTag" (
  "bookmarkId" text NOT NULL REFERENCES "Bookmark"(id) ON DELETE CASCADE,
  "tagId" text NOT NULL REFERENCES "Tag"(id) ON DELETE CASCADE,
  PRIMARY KEY ("bookmarkId", "tagId")
);
CREATE INDEX "BookmarkTag_tagId_idx" ON "BookmarkTag"("tagId");

-- 2.7 BookmarkShare
CREATE TABLE "BookmarkShare" (
  "bookmarkId" text NOT NULL REFERENCES "Bookmark"(id) ON DELETE CASCADE,
  "userId" text NOT NULL REFERENCES "User"(id) ON DELETE CASCADE,
  "createdAt" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("bookmarkId", "userId")
);
CREATE INDEX "BookmarkShare_userId_idx" ON "BookmarkShare"("userId");

-- 2.8 TagShare
CREATE TABLE "TagShare" (
  "tagId" text NOT NULL REFERENCES "Tag"(id) ON DELETE CASCADE,
  "userId" text NOT NULL REFERENCES "User"(id) ON DELETE CASCADE,
  "createdAt" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("tagId", "userId")
);
CREATE INDEX "TagShare_userId_idx" ON "TagShare"("userId");

-- 2.9 Contact
CREATE TABLE "Contact" (
  id text PRIMARY KEY,
  uid text NOT NULL,
  "displayName" text NOT NULL,
  email text,
  phone text,
  "cardDavUrl" text,
  "userId" text NOT NULL REFERENCES "User"(id) ON DELETE CASCADE,
  "createdAt" timestamptz NOT NULL DEFAULT now(),
  "updatedAt" timestamptz NOT NULL DEFAULT now(),
  UNIQUE(uid, "userId")
);
CREATE INDEX "Contact_userId_idx" ON "Contact"("userId");
ALTER TABLE "Bookmark" ADD CONSTRAINT "Bookmark_contactId_fkey"
  FOREIGN KEY ("contactId") REFERENCES "Contact"(id) ON DELETE SET NULL;

-- 2.10 ImapConfig
CREATE TABLE "ImapConfig" (
  id text PRIMARY KEY,
  host text NOT NULL,
  port int NOT NULL DEFAULT 993,
  tls boolean NOT NULL DEFAULT true,
  username text NOT NULL,
  password text NOT NULL,
  folder text NOT NULL DEFAULT 'INBOX',
  "userId" text UNIQUE NOT NULL REFERENCES "User"(id) ON DELETE CASCADE
);

-- 2.11 KanbanColumn
CREATE TABLE "KanbanColumn" (
  id text PRIMARY KEY,
  label text NOT NULL,
  color text NOT NULL DEFAULT '#64748b',
  position int NOT NULL DEFAULT 0,
  "key" text,
  "userId" text NOT NULL REFERENCES "User"(id) ON DELETE CASCADE,
  "createdAt" timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX "KanbanColumn_userId_position_idx" ON "KanbanColumn"("userId", position);

-- Bookmark.kanbanColumnId -> KanbanColumn (SetNull)
ALTER TABLE "Bookmark" ADD CONSTRAINT "Bookmark_kanbanColumnId_fkey"
  FOREIGN KEY ("kanbanColumnId") REFERENCES "KanbanColumn"(id) ON DELETE SET NULL;

-- 2.12 Page
CREATE TABLE "Page" (
  id text PRIMARY KEY,
  title text NOT NULL,
  content text NOT NULL DEFAULT '',
  icon text,
  pinned boolean NOT NULL DEFAULT false,
  position int NOT NULL DEFAULT 0,
  "ownerId" text NOT NULL REFERENCES "User"(id) ON DELETE CASCADE,
  "createdAt" timestamptz NOT NULL DEFAULT now(),
  "updatedAt" timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX "Page_ownerId_position_idx" ON "Page"("ownerId", position);

-- 2.13 PageBookmark
CREATE TABLE "PageBookmark" (
  "pageId" text NOT NULL REFERENCES "Page"(id) ON DELETE CASCADE,
  "bookmarkId" text NOT NULL REFERENCES "Bookmark"(id) ON DELETE CASCADE,
  position int NOT NULL DEFAULT 0,
  "createdAt" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("pageId", "bookmarkId")
);
CREATE INDEX "PageBookmark_bookmarkId_idx" ON "PageBookmark"("bookmarkId");

-- 2.14 SignalSource
CREATE TABLE "SignalSource" (
  id text PRIMARY KEY,
  type text NOT NULL,
  config json NOT NULL DEFAULT '{}',
  enabled boolean NOT NULL DEFAULT true,
  "lastCheckedAt" timestamptz,
  "lastStatus" text,
  "bookmarkId" text NOT NULL REFERENCES "Bookmark"(id) ON DELETE CASCADE,
  "ownerId" text NOT NULL REFERENCES "User"(id) ON DELETE CASCADE,
  "createdAt" timestamptz NOT NULL DEFAULT now(),
  "updatedAt" timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX "SignalSource_bookmarkId_idx" ON "SignalSource"("bookmarkId");
CREATE INDEX "SignalSource_ownerId_idx" ON "SignalSource"("ownerId");

-- 2.15 Signal
CREATE TABLE "Signal" (
  id text PRIMARY KEY,
  "sourceType" text NOT NULL,
  title text NOT NULL,
  summary text,
  url text,
  "externalId" text,
  "occurredAt" timestamptz NOT NULL DEFAULT now(),
  read boolean NOT NULL DEFAULT false,
  "sourceId" text,
  "bookmarkId" text NOT NULL REFERENCES "Bookmark"(id) ON DELETE CASCADE,
  "ownerId" text NOT NULL REFERENCES "User"(id) ON DELETE CASCADE,
  "createdAt" timestamptz NOT NULL DEFAULT now(),
  UNIQUE("sourceId", "externalId")
);
CREATE INDEX "Signal_bookmarkId_occurredAt_idx" ON "Signal"("bookmarkId", "occurredAt");
CREATE INDEX "Signal_ownerId_occurredAt_idx" ON "Signal"("ownerId", "occurredAt");
ALTER TABLE "Signal" ADD CONSTRAINT "Signal_sourceId_fkey"
  FOREIGN KEY ("sourceId") REFERENCES "SignalSource"(id) ON DELETE SET NULL;

-- 2.16 SharedDate
CREATE TABLE "SharedDate" (
  id text PRIMARY KEY,
  date date NOT NULL,
  note text,
  "creatorId" text NOT NULL REFERENCES "User"(id) ON DELETE CASCADE,
  "recipientId" text NOT NULL REFERENCES "User"(id) ON DELETE CASCADE,
  "createdAt" timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX "SharedDate_recipientId_idx" ON "SharedDate"("recipientId");
CREATE INDEX "SharedDate_creatorId_idx" ON "SharedDate"("creatorId");
CREATE INDEX "SharedDate_date_idx" ON "SharedDate"(date);
COMMIT;
