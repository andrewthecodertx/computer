package handlers

import (
	"database/sql"
	"github.com/andrew/go-computer/internal/db"
	"github.com/andrew/go-computer/internal/markdown"
	"net/http"
	"strings"
	"time"
)

func page(r *http.Request, id string) (object, error) {
	return db.One(r.Context(), database(r), `SELECT to_jsonb(p)||jsonb_build_object('bookmarks',COALESCE((SELECT jsonb_agg(to_jsonb(pb)||jsonb_build_object('bookmark',`+db.BookmarkView+`) ORDER BY pb.position) FROM "PageBookmark" pb JOIN "Bookmark" b ON b.id=pb."bookmarkId" WHERE pb."pageId"=p.id AND `+db.BookmarkVisible+`),'[]'::jsonb)) FROM "Page" p WHERE p.id=$1 AND p."ownerId"=$2`, id, user(r))
}
func HandlePagesList(w http.ResponseWriter, r *http.Request) {
	v, e := db.Many(r.Context(), database(r), `SELECT to_jsonb(p)||jsonb_build_object('_count',jsonb_build_object('bookmarks',(SELECT count(*) FROM "PageBookmark" pb WHERE pb."pageId"=p.id))) FROM "Page" p WHERE "ownerId"=$1 AND (title ILIKE $2 OR content ILIKE $2) ORDER BY pinned DESC,position,"createdAt"`, user(r), "%"+r.URL.Query().Get("search")+"%")
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
	d["updatedAt"] = time.Now().UTC()
	e = db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		if _, e := owned(r, tx, "Page", id, "ownerId"); e != nil {
			return e
		}
		if _, e := db.Update(r.Context(), tx, "Page", id, "ownerId", user(r), d); e != nil {
			return e
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
		return nil
	})
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	v, e := page(r, id)
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
