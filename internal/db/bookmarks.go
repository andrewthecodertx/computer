package db

// PostgreSQL builds the camelCase JSON shape consumed by the existing UI.
// The password-bearing User and ImapConfig rows are never embedded.
const BookmarkView = `to_jsonb(b) || jsonb_build_object(
 'tags',COALESCE((SELECT jsonb_agg(to_jsonb(bt)||jsonb_build_object('tag',to_jsonb(t))) FROM "BookmarkTag" bt JOIN "Tag" t ON t.id=bt."tagId" WHERE bt."bookmarkId"=b.id),'[]'::jsonb),
 'contact',(SELECT to_jsonb(c) FROM "Contact" c WHERE c.id=b."contactId"),
 'sharedWith',COALESCE((SELECT jsonb_agg(to_jsonb(s)||jsonb_build_object('user',jsonb_build_object('id',u.id,'name',u.name,'email',u.email))) FROM "BookmarkShare" s JOIN "User" u ON u.id=s."userId" WHERE s."bookmarkId"=b.id),'[]'::jsonb),
 'owner',(SELECT jsonb_build_object('id',u.id,'name',u.name,'email',u.email,'image',u.image) FROM "User" u WHERE u.id=b."ownerId"))`
const BookmarkVisible = `(b."ownerId"=$2 OR EXISTS(SELECT 1 FROM "BookmarkShare" s WHERE s."bookmarkId"=b.id AND s."userId"=$2)
 OR EXISTS(SELECT 1 FROM "BookmarkTag" bt JOIN "TagShare" ts ON ts."tagId"=bt."tagId" WHERE bt."bookmarkId"=b.id AND ts."userId"=$2)
 OR EXISTS(SELECT 1 FROM "SharedDate" sd WHERE sd."creatorId"=b."ownerId" AND sd."recipientId"=$2 AND b."dueDate"::date=sd.date))`
