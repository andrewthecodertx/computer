package handlers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/andrew/go-computer/internal/db"
	"github.com/andrew/go-computer/internal/markdown"
	"github.com/andrew/go-computer/internal/preview"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func bookmark(r *http.Request, id string, public bool) (object, error) {
	if public {
		// Review PRIV-04: the public projection is an explicit allowlist and
		// carries NO owner attribution — the README promises owner identity
		// stays private, and API, share page, and docs must agree. Notes and
		// dueDate ARE published by product decision; the share panel states
		// exactly what a public link exposes before the owner toggles it.
		return db.One(r.Context(), database(r), `SELECT jsonb_build_object(
		 'id',b.id,'url',b.url,'title',b.title,'description',b.description,'notes',b.notes,
		 'favicon',b.favicon,'ogImage',b."ogImage",'ogTitle',b."ogTitle",'ogDescription',b."ogDescription",
		 'dueDate',b."dueDate",'isPublic',b."isPublic",
		 'tags',COALESCE((SELECT jsonb_agg(jsonb_build_object('tag',jsonb_build_object('id',t.id,'name',t.name,'color',t.color))) FROM "BookmarkTag" bt JOIN "Tag" t ON t.id=bt."tagId" WHERE bt."bookmarkId"=b.id),'[]'::jsonb))
		 FROM "Bookmark" b WHERE b.id=$1 AND b."isPublic"`, id)
	}
	// Review PRIV-02: shared readers get the narrowed allowlist projection;
	// only the owner receives the full row with contact and recipient list.
	where := `b.id=$1 AND ` + db.BookmarkVisible
	uid := user(r)
	return db.One(r.Context(), database(r), `SELECT `+db.BookmarkViewerView+` || jsonb_build_object('pageLinks',COALESCE((SELECT jsonb_agg(to_jsonb(pb)||jsonb_build_object('page',jsonb_build_object('id',p.id,'title',p.title))) FROM "PageBookmark" pb JOIN "Page" p ON p.id=pb."pageId" WHERE pb."bookmarkId"=b.id AND p."ownerId"=$2),'[]'::jsonb)) FROM "Bookmark" b WHERE `+where, id, uid)
}
func HandleBookmarksList(w http.ResponseWriter, r *http.Request) {
	args := []any{nil, user(r)}
	sp := r.URL.Query()
	limit := 200
	if raw := sp.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			respond(w, 0, nil, bad("Limit must be between 1 and 200"))
			return
		}
		limit = n
	}
	where := `b."ownerId"=$2`
	if sp.Get("shared") == "true" {
		where = db.BookmarkVisible + ` AND b."ownerId"<>$2`
	}
	add := func(s string, v any) { args = append(args, v); where += " AND " + fmt.Sprintf(s, len(args)) }
	if s := sp.Get("search"); s != "" {
		// Escape LIKE wildcards (review PERF-2): an unescaped query pattern
		// matched everything and defeated the search's selectivity.
		args = append(args, "%"+likeEscaper.Replace(s)+"%")
		n := len(args)
		where += fmt.Sprintf(` AND (b.title ILIKE $%d ESCAPE '\' OR b.url ILIKE $%d ESCAPE '\' OR b.notes ILIKE $%d ESCAPE '\' OR EXISTS(SELECT 1 FROM "BookmarkTag" bt JOIN "Tag" t ON t.id=bt."tagId" WHERE bt."bookmarkId"=b.id AND t.name ILIKE $%d ESCAPE '\'))`, n, n, n, n)
	}
	if s := sp.Get("tagId"); s != "" {
		add(`EXISTS(SELECT 1 FROM "BookmarkTag" bt WHERE bt."bookmarkId"=b.id AND bt."tagId"=$%d)`, s)
		if sp.Get("shared") != "true" {
			where = strings.Replace(where, `b."ownerId"=$2`, db.BookmarkVisible, 1)
		}
	}
	if s := sp.Get("contactId"); s != "" {
		add(`b."contactId"=$%d`, s)
	}
	if s := sp.Get("kanban"); s != "" {
		if !validStatus(s) {
			respond(w, 0, nil, bad("Invalid status"))
			return
		}
		add(`b."kanbanStatus"=$%d`, s)
	}
	if s := sp.Get("date"); s != "" {
		v, e := dateValue(s)
		if e != nil {
			respond(w, 0, nil, e)
			return
		}
		// Pin both sides to UTC: bare ::date casts use the Postgres session
		// timezone, which is not pinned and would shift "due on date X".
		add(`(b."dueDate" AT TIME ZONE 'UTC')::date=($%d::timestamptz AT TIME ZONE 'UTC')::date`, v)
		// Guarded exactly like tagId (review COR-7): re-injecting the
		// visibility predicate into a list already widened by it would nest
		// BookmarkVisible inside itself.
		if sp.Get("shared") != "true" && !strings.Contains(where, db.BookmarkVisible) {
			where = strings.Replace(where, `b."ownerId"=$2`, db.BookmarkVisible, 1)
		}
	}
	// Port artifact: db.BookmarkVisible binds the user as $2, so $1 stays a
	// permanently-nil placeholder and this predicate is always true. Removing
	// it would require renumbering BookmarkVisible's parameters everywhere.
	where += ` AND $1::text IS NULL`
	if raw := sp.Get("cursor"); raw != "" {
		var cursor struct {
			UpdatedAt time.Time `json:"updatedAt"`
			ID        string    `json:"id"`
		}
		data, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.UpdatedAt.IsZero() || cursor.ID == "" {
			respond(w, 0, nil, bad("Invalid cursor"))
			return
		}
		args = append(args, cursor.UpdatedAt, cursor.ID)
		where += fmt.Sprintf(` AND (b."updatedAt",b.id)<($%d,$%d)`, len(args)-1, len(args))
	}
	args = append(args, limit+1)
	v, e := db.Many(r.Context(), database(r), `SELECT `+db.BookmarkViewerView+` FROM "Bookmark" b WHERE `+where+fmt.Sprintf(` ORDER BY b."updatedAt" DESC,b.id DESC LIMIT $%d`, len(args)), args...)
	if e == nil && len(v) > limit {
		v = v[:limit]
		last := v[len(v)-1]
		cursor, _ := json.Marshal(object{"updatedAt": last["updatedAt"], "id": last["id"]})
		w.Header().Set("X-Next-Cursor", base64.RawURLEncoding.EncodeToString(cursor))
	}
	respond(w, 200, v, e)
}
func validStatus(s string) bool {
	return s == "INBOX" || s == "TODO" || s == "IN_PROGRESS" || s == "DONE" || s == "ARCHIVED"
}
func bookmarkFields(b object) (object, error) {
	d := fields(b, "url title description notes dueDate alertAt kanbanStatus kanbanColumnId contactId isPublic imapWatchEnabled imapQuery")
	if e := stringFields(d, "title description notes kanbanColumnId contactId imapQuery", "url kanbanStatus"); e != nil {
		return nil, e
	}
	if e := boolFields(d, "isPublic imapWatchEnabled"); e != nil {
		return nil, e
	}
	if v, ok := d["url"]; ok && !validURL(v.(string)) {
		return nil, bad("A valid HTTP(S) URL is required")
	}
	if s, ok := d["kanbanStatus"]; ok && !validStatus(s.(string)) {
		return nil, bad("Invalid status")
	}
	for _, k := range []string{"dueDate", "alertAt"} {
		if v, ok := d[k]; ok {
			v, e := dateValue(v)
			if e != nil {
				return nil, e
			}
			d[k] = v
			if k == "alertAt" {
				d["alertSent"] = false
			}
		}
	}
	for _, k := range []string{"contactId", "kanbanColumnId"} {
		if d[k] == "" {
			d[k] = nil
		}
	}
	return d, nil
}
func checkRelations(r *http.Request, tx *sql.Tx, d object, tags []string) error {
	for k, table := range map[string]string{"contactId": "Contact", "kanbanColumnId": "KanbanColumn"} {
		if v, ok := d[k]; ok && v != nil {
			if _, e := owned(r, tx, table, v.(string), "userId"); e != nil {
				return bad("Invalid " + k)
			}
		}
	}
	// A tag shared WITH you may be attached to your own bookmarks. Note the
	// group semantics this implies (by design, see db.BookmarkVisible): the
	// bookmark then becomes visible to every recipient of that tag.
	for _, tag := range tags {
		if _, e := db.One(r.Context(), tx, `SELECT to_jsonb(t) FROM "Tag" t WHERE id=$1 AND ("ownerId"=$2 OR EXISTS(SELECT 1 FROM "TagShare" s WHERE s."tagId"=t.id AND s."userId"=$2))`, tag, user(r)); e != nil {
			return bad("Invalid tag")
		}
	}
	return nil
}
func setTags(r *http.Request, tx *sql.Tx, id string, tags []string) error {
	if _, e := tx.ExecContext(r.Context(), `DELETE FROM "BookmarkTag" WHERE "bookmarkId"=$1`, id); e != nil {
		return e
	}
	for _, t := range tags {
		if _, e := tx.ExecContext(r.Context(), `INSERT INTO "BookmarkTag"("bookmarkId","tagId") VALUES($1,$2)`, id, t); e != nil {
			return e
		}
	}
	return nil
}

// addPreview fetches og/favicon metadata for the bookmark's URL through the
// shared preview budget (review CR-06): bookmark mutations and the dedicated
// preview route draw from the SAME per-identity rate and global concurrency
// limits, so saves cannot bypass the route's throttle. Preview work is
// decorative — when the budget is exhausted the bookmark still persists,
// just without fresh og metadata.
func addPreview(ctx context.Context, key string, d object, create bool) {
	p := preview.FetchPreview(ctx, key, str(d, "url"))
	d["favicon"], d["ogImage"], d["ogTitle"], d["ogDescription"] = p.Favicon, p.OgImage, p.OgTitle, p.OgDescription
	if create {
		if d["title"] == nil {
			if p.OgTitle != nil {
				d["title"] = *p.OgTitle
			} else {
				d["title"] = d["url"]
			}
		}
		if d["description"] == nil {
			d["description"] = p.OgDescription
		}
	}
}
func HandleBookmarksCreate(w http.ResponseWriter, r *http.Request) {
	b, ok := read(w, r)
	if !ok {
		return
	}
	d, e := bookmarkFields(b)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	if str(d, "url") == "" {
		respond(w, 0, nil, bad("URL required"))
		return
	}
	tags, e := ids(b, "tagIds")
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	id := db.NewID()
	d["id"], d["ownerId"] = id, user(r)
	addPreview(r.Context(), previewKey(r), d, true)
	e = db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		// One locking protocol for board mutations (review CR-05): column
		// deletion holds lockUser while it snapshots the affected bookmarks,
		// moves them and drops the column. Bookmark writes that validate and
		// assign kanbanColumnId take the SAME lock first, so a concurrent
		// create can no longer land on a column the deletion already counted
		// as moved (or escape its move set). Lock order is always user row →
		// bookmarks/columns, matching every other lockUser transaction.
		if e := lockUser(r, tx); e != nil {
			return e
		}
		if e := checkRelations(r, tx, d, tags); e != nil {
			return e
		}
		row, e := db.Insert(r.Context(), tx, "Bookmark", d)
		if e != nil {
			return e
		}
		if e = syncLegacyWatch(r, tx, row); e != nil {
			return e
		}
		return setTags(r, tx, id, tags)
	})
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	v, e := bookmark(r, id, false)
	respond(w, 201, v, e)
}
func HandleBookmarksGet(w http.ResponseWriter, r *http.Request) {
	v, e := bookmark(r, r.PathValue("id"), false)
	respond(w, 200, v, e)
}
func HandleBookmarksUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, e := owned(r, database(r), "Bookmark", id, "ownerId")
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	b, ok := read(w, r)
	if !ok {
		return
	}
	d, e := bookmarkFields(b)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	// Optimistic concurrency (review CR-04): the PUT is the owner-driven
	// mutation surface (fields AND tag replacement), and concurrency is not
	// optional on it — the same DATA-05 rule pages enforce. The client sends
	// the version it edited from; a stale base is a 409 and the row is left
	// untouched, so overlapping edits can no longer silently overwrite each
	// other. Sharing (POST /share) and server-driven flags (alertSent) are
	// deliberately outside this contract; the kanban column-deletion move
	// bumps the version itself.
	baseVersion, hasVersion := 0, false
	if v, present := b["version"]; present {
		f, isNum := v.(float64)
		if !isNum || f < 0 || f != float64(int64(f)) {
			respond(w, 0, nil, bad("version must be a non-negative integer"))
			return
		}
		baseVersion, hasVersion = int(f), true
	}
	if !hasVersion {
		respond(w, 0, nil, bad("version is required when updating a bookmark; refetch it and retry"))
		return
	}
	tags, e := ids(b, "tagIds")
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	if s := str(d, "url"); s != "" && s != str(existing, "url") {
		addPreview(r.Context(), previewKey(r), d, false)
	}
	d["updatedAt"] = time.Now().UTC()
	d["version"] = baseVersion + 1
	e = db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		// Review CR-05: same board-mutation locking protocol as create and
		// column deletion — placement is validated (checkRelations) and
		// written under the user lock, so a column deletion can neither race
		// this assignment nor move a bookmark this write just placed.
		if e := lockUser(r, tx); e != nil {
			return e
		}
		// Re-read under the lock: the version check must see the latest
		// committed state, not the pre-transaction snapshot.
		cur, e := owned(r, tx, "Bookmark", id, "ownerId")
		if e != nil {
			return e
		}
		if stored, _ := cur["version"].(float64); int(stored) != baseVersion {
			return conflict("Bookmark was modified elsewhere; reload to see the latest version")
		}
		if e := checkRelations(r, tx, d, tags); e != nil {
			return e
		}
		row, e := db.Update(r.Context(), tx, "Bookmark", id, "ownerId", user(r), d)
		if e != nil {
			return e
		}
		if e = syncLegacyWatch(r, tx, row); e != nil {
			return e
		}
		if _, ok := b["tagIds"]; ok {
			return setTags(r, tx, id, tags)
		}
		return nil
	})
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	v, e := bookmark(r, id, false)
	respond(w, 200, v, e)
}
func HandleBookmarksDelete(w http.ResponseWriter, r *http.Request) {
	e := remove(r, "Bookmark", r.PathValue("id"), "ownerId")
	respond(w, 200, OK{true}, e)
}

// The original mailbox-watch switch now drives an actual email signal source.
// Modern watchers added in the Signals panel are managed independently.
func syncLegacyWatch(r *http.Request, tx *sql.Tx, b object) error {
	enabled, _ := b["imapWatchEnabled"].(bool)
	raw, _ := json.Marshal(object{"subject": str(b, "imapQuery"), "legacy": true})
	res, e := tx.ExecContext(r.Context(), `UPDATE "SignalSource" SET config=$1::json,enabled=$2,"updatedAt"=now() WHERE "bookmarkId"=$3 AND "ownerId"=$4 AND config->>'legacy'='true'`, string(raw), enabled, b["id"], user(r))
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n == 0 && enabled {
		_, e = db.Insert(r.Context(), tx, "SignalSource", object{"id": db.NewID(), "type": "email", "config": string(raw), "enabled": true, "bookmarkId": b["id"], "ownerId": user(r)})
	}
	return e
}
func HandleBookmarksShare(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, e := owned(r, database(r), "Bookmark", id, "ownerId"); e != nil {
		respond(w, 0, nil, e)
		return
	}
	b, ok := read(w, r)
	if !ok {
		return
	}
	list, e := ids(b, "userIds")
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	if e = boolFields(b, "isPublic"); e != nil {
		respond(w, 0, nil, e)
		return
	}
	e = db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		if e := shareRows(r, tx, "BookmarkShare", "bookmarkId", id, list); e != nil {
			return e
		}
		if _, ok := b["isPublic"]; ok {
			_, e := db.Update(r.Context(), tx, "Bookmark", id, "ownerId", user(r), object{"isPublic": b["isPublic"], "updatedAt": time.Now().UTC()})
			return e
		}
		return nil
	})
	respond(w, 200, OK{true}, e)
}
func HandleBookmarksMarkdown(w http.ResponseWriter, r *http.Request) {
	// bookmark() already applies the permission-specific projection (review
	// PRIV-02), so shared readers export only fields they may read.
	v, e := bookmark(r, r.PathValue("id"), false)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	markdown.Send(w, markdown.BookmarkDocument(v), str(v, "title"))
}

// previewKey is the shared-budget identity for preview work (review CR-06):
// per user, across every entry point (route + mutations).
func previewKey(r *http.Request) string { return "user:" + user(r) }

func HandlePreview(w http.ResponseWriter, r *http.Request) {
	u := r.URL.Query().Get("url")
	if !validURL(u) {
		respond(w, 0, nil, bad("A valid HTTP(S) URL is required"))
		return
	}
	respond(w, 200, preview.FetchPreview(r.Context(), previewKey(r), u), nil)
}
