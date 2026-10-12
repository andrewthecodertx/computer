package db

// PostgreSQL builds the camelCase JSON shape consumed by the existing UI.
// The password-bearing User and ImapConfig rows are never embedded.
//
// Review PRIV-02: bookmark projections are permission-specific. Permission to
// read a bookmark (direct share, shared tag, or shared date) does not imply
// permission to read the owner's address-book entry, enumerate the other
// recipients, or see the owner's operational metadata (alerts, mailbox watch,
// board placement). BookmarkSharedView is an allowlist: new columns are NOT
// exposed to shared readers without a deliberate addition there.

// BookmarkOwnerView is the full projection, for the owner only. It embeds the
// attached contact record and the recipient list with names and emails — the
// share panel renders those for the owner.
const BookmarkOwnerView = `to_jsonb(b) || jsonb_build_object(
 'tags',COALESCE((SELECT jsonb_agg(to_jsonb(bt)||jsonb_build_object('tag',to_jsonb(t))) FROM "BookmarkTag" bt JOIN "Tag" t ON t.id=bt."tagId" WHERE bt."bookmarkId"=b.id),'[]'::jsonb),
 'contact',(SELECT to_jsonb(c) FROM "Contact" c WHERE c.id=b."contactId"),
 'sharedWith',COALESCE((SELECT jsonb_agg(to_jsonb(s)||jsonb_build_object('user',jsonb_build_object('id',u.id,'name',u.name,'email',u.email))) FROM "BookmarkShare" s JOIN "User" u ON u.id=s."userId" WHERE s."bookmarkId"=b.id),'[]'::jsonb),
 'owner',(SELECT jsonb_build_object('id',u.id,'name',u.name,'email',u.email,'image',u.image) FROM "User" u WHERE u.id=b."ownerId"))`

// BookmarkSharedView is the narrowed projection for authenticated non-owner
// readers: the link itself, its presentation metadata, the content the owner
// attached for readers (description, notes, due date, tags), and enough owner
// identity to attribute the share inside the workspace. No contact record, no
// recipient list, no alerts, no mailbox-watch settings, no board placement.
const BookmarkSharedView = `jsonb_build_object(
 'id',b.id,'url',b.url,'title',b.title,'description',b.description,'notes',b.notes,
 'favicon',b.favicon,'ogImage',b."ogImage",'ogTitle',b."ogTitle",'ogDescription',b."ogDescription",
 'dueDate',b."dueDate",'isPublic',b."isPublic",'kanbanStatus',b."kanbanStatus",
 'ownerId',b."ownerId",'createdAt',b."createdAt",'updatedAt',b."updatedAt",
 'tags',COALESCE((SELECT jsonb_agg(to_jsonb(bt)||jsonb_build_object('tag',to_jsonb(t))) FROM "BookmarkTag" bt JOIN "Tag" t ON t.id=bt."tagId" WHERE bt."bookmarkId"=b.id),'[]'::jsonb),
 'owner',(SELECT jsonb_build_object('id',u.id,'name',u.name) FROM "User" u WHERE u.id=b."ownerId"))`

// BookmarkViewerView picks the projection by ownership. Like BookmarkVisible
// it binds the requesting user as $2, so it drops into the same queries.
const BookmarkViewerView = `CASE WHEN b."ownerId"=$2 THEN ` + BookmarkOwnerView + ` ELSE ` + BookmarkSharedView + ` END`

// BookmarkVisible binds the requesting user as $2. Tag sharing is group-wide
// by product decision: any recipient of a tag sees every bookmark carrying
// it, including bookmarks other recipients added. The share UI says so.
const BookmarkVisible = `(b."ownerId"=$2 OR EXISTS(SELECT 1 FROM "BookmarkShare" s WHERE s."bookmarkId"=b.id AND s."userId"=$2)
 OR EXISTS(SELECT 1 FROM "BookmarkTag" bt JOIN "TagShare" ts ON ts."tagId"=bt."tagId" WHERE bt."bookmarkId"=b.id AND ts."userId"=$2)
 OR EXISTS(SELECT 1 FROM "SharedDate" sd WHERE sd."creatorId"=b."ownerId" AND sd."recipientId"=$2 AND (b."dueDate" AT TIME ZONE 'UTC')::date=sd.date))`
