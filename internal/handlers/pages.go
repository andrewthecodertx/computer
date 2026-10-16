package handlers

import (
	"context"
	"database/sql"
	"github.com/andrew/go-computer/internal/db"
	"github.com/andrew/go-computer/internal/markdown"
	"net/http"
	"strings"
	"time"
)

// pageView is the canonical page projection over any queryer. HandlePagesUpdate
// reads it INSIDE the write transaction (review CR-18): a post-commit query
// could observe another writer's committed revision, and the acknowledgement
// would then describe state this write did not produce — a client adopting it
// could overwrite the other writer on its next save.
func pageView(ctx context.Context, q db.Queryer, id, uid string) (object, error) {
	// Linked bookmarks use the viewer projection (review PRIV-02): a page can
	// link bookmarks shared with its owner, and those arrive narrowed.
	return db.One(ctx, q, `SELECT to_jsonb(p)||jsonb_build_object('bookmarks',COALESCE((SELECT jsonb_agg(to_jsonb(pb)||jsonb_build_object('bookmark',`+db.BookmarkViewerView+`) ORDER BY pb.position) FROM "PageBookmark" pb JOIN "Bookmark" b ON b.id=pb."bookmarkId" WHERE pb."pageId"=p.id AND `+db.BookmarkVisible+`),'[]'::jsonb)) FROM "Page" p WHERE p.id=$1 AND p."ownerId"=$2`, id, uid)
}
func page(r *http.Request, id string) (object, error) {
	return pageView(r.Context(), database(r), id, user(r))
}
func HandlePagesList(w http.ResponseWriter, r *http.Request) {
	// Review PERF-01: the list is a metadata projection — content bodies stay
	// out of it (the sidebar/tab strip only render titles) and are fetched on
	// demand via GET /api/pages/{id}. Search still matches content server-side.
	v, e := db.Many(r.Context(), database(r), `SELECT (to_jsonb(p)-'content')||jsonb_build_object('_count',jsonb_build_object('bookmarks',(SELECT count(*) FROM "PageBookmark" pb WHERE pb."pageId"=p.id))) FROM "Page" p WHERE "ownerId"=$1 AND (title ILIKE $2 OR content ILIKE $2) ORDER BY pinned DESC,position,"createdAt"`, user(r), "%"+r.URL.Query().Get("search")+"%")
	respond(w, 200, v, e)
}
func pageData(b object) (object, error) {
	d := fields(b, "title content icon pinned")
	if e := stringFields(d, "icon", "title content"); e != nil {
		return nil, e
	}
	if e := boolFields(d, "pinned"); e != nil {
		return nil, e
	}
	if _, ok := d["title"]; ok {
		s := strings.TrimSpace(str(d, "title"))
		if s == "" {
			s = "Untitled page"
		}
		rs := []rune(s)
		if len(rs) > 120 {
			rs = rs[:120]
		}
		d["title"] = string(rs)
	}
	return d, nil
}
func HandlePagesCreate(w http.ResponseWriter, r *http.Request) {
	b, ok := read(w, r)
	if !ok {
		return
	}
	d, e := pageData(b)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	if _, ok := d["title"]; !ok {
		d["title"] = "Untitled page"
	}
	d["id"], d["ownerId"] = db.NewID(), user(r)
	var v object
	e = db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		if e := lockUser(r, tx); e != nil {
			return e
		}
		var n int
		if e := tx.QueryRowContext(r.Context(), `SELECT COALESCE(max(position),-1)+1 FROM "Page" WHERE "ownerId"=$1`, user(r)).Scan(&n); e != nil {
			return e
		}
		d["position"] = n
		var e error
		v, e = db.Insert(r.Context(), tx, "Page", d)
		return e
	})
	respond(w, 201, v, e)
}
func HandlePagesReorder(w http.ResponseWriter, r *http.Request) {
	b, ok := read(w, r)
	if !ok {
		return
	}
	list, e := ids(b, "order")
	if e == nil {
		e = reorder(r, "Page", "ownerId", list)
	}
	respond(w, 200, OK{true}, e)
}
func HandlePagesGet(w http.ResponseWriter, r *http.Request) {
	v, e := page(r, r.PathValue("id"))
	respond(w, 200, v, e)
}
func HandlePagesUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	b, ok := read(w, r)
	if !ok {
		return
	}
	d, e := pageData(b)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	// Optimistic concurrency (review DATA-01): a client that sends the
	// version it edited from gets a 409 instead of silently overwriting a
	// newer save from another tab or device. The revision counter tracks
	// content edits — metadata patches (title/pin/links) neither require nor
	// bump it, so they cannot invalidate an editor's in-flight draft.
	baseVersion, hasVersion := 0, false
	if v, ok := b["version"]; ok {
		f, isNum := v.(float64)
		if !isNum || f < 0 || f != float64(int64(f)) {
			respond(w, 0, nil, bad("version must be a non-negative integer"))
			return
		}
		baseVersion, hasVersion = int(f), true
	}
	// Review DATA-05: concurrency must not be optional at the API boundary.
	// A content write without a base version is rejected outright — otherwise
	// any script or stale client could silently clobber newer content by
	// omitting the field. Explicit overwrites re-fetch the latest version and
	// send it (web `resolveOverwrite`); they never bypass this check.
	if _, hasContent := d["content"]; hasContent && !hasVersion {
		respond(w, 0, nil, bad("version is required when saving content; refetch the page and retry"))
		return
	}
	d["updatedAt"] = time.Now().UTC()
	// Review CR-18: the response snapshot is captured inside the same
	// transaction as the write, so the acknowledged revision always
	// describes exactly the state this request produced — never a later
	// commit by a competing writer.
	var v object
	e = db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		// Same locking discipline as page creation/reorder: serializes
		// max(position)+1 so concurrent link adds cannot duplicate positions.
		if e := lockUser(r, tx); e != nil {
			return e
		}
		current, e := owned(r, tx, "Page", id, "ownerId")
		if e != nil {
			return e
		}
		if hasVersion {
			stored, _ := current["version"].(float64)
			if int(stored) != baseVersion {
				return conflict("Page was modified elsewhere; reload to see the latest version")
			}
		}
		if _, e := db.Update(r.Context(), tx, "Page", id, "ownerId", user(r), d); e != nil {
			return e
		}
		if _, ok := d["content"]; ok {
			if _, e := tx.ExecContext(r.Context(), `UPDATE "Page" SET version=version+1 WHERE id=$1`, id); e != nil {
				return e
			}
		}
		if bid := str(b, "addBookmarkId"); bid != "" {
			if _, e := db.One(r.Context(), tx, `SELECT to_jsonb(b) FROM "Bookmark" b WHERE b.id=$1 AND `+db.BookmarkVisible, bid, user(r)); e != nil {
				return bad("Bookmark is not accessible")
			}
			_, e := tx.ExecContext(r.Context(), `INSERT INTO "PageBookmark"("pageId","bookmarkId",position) SELECT $1,$2,COALESCE(max(position),-1)+1 FROM "PageBookmark" WHERE "pageId"=$1 ON CONFLICT DO NOTHING`, id, bid)
			if e != nil {
				return e
			}
		}
		if bid := str(b, "removeBookmarkId"); bid != "" {
			_, e := tx.ExecContext(r.Context(), `DELETE FROM "PageBookmark" WHERE "pageId"=$1 AND "bookmarkId"=$2`, id, bid)
			if e != nil {
				return e
			}
		}
		v, e = pageView(r.Context(), tx, id, user(r))
		return e
	})
	respond(w, 200, v, e)
}
func HandlePagesDelete(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, OK{true}, remove(r, "Page", r.PathValue("id"), "ownerId"))
}
func HandlePagesMarkdown(w http.ResponseWriter, r *http.Request) {
	v, e := page(r, r.PathValue("id"))
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	markdown.Send(w, markdown.PageDocument(v), str(v, "title"))
}
