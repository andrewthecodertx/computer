package handlers

import (
	"database/sql"
	"github.com/andrew/go-computer/internal/db"
	"net/http"
	"strings"
	"time"
)

func HandleTagsList(w http.ResponseWriter, r *http.Request) {
	v, e := db.Many(r.Context(), database(r), `SELECT to_jsonb(t)||jsonb_build_object('_count',jsonb_build_object('bookmarks',(SELECT count(*) FROM "BookmarkTag" bt WHERE bt."tagId"=t.id)),'sharedWith',COALESCE((SELECT jsonb_agg(to_jsonb(s)) FROM "TagShare" s WHERE s."tagId"=t.id),'[]'::jsonb)) FROM "Tag" t WHERE "ownerId"=$1 OR EXISTS(SELECT 1 FROM "TagShare" s WHERE s."tagId"=t.id AND s."userId"=$1) ORDER BY name`, user(r))
	respond(w, 200, v, e)
}
func tagData(b object) (object, error) {
	d := fields(b, "name color")
	if e := stringFields(d, "", "name color"); e != nil {
		return nil, e
	}
	if _, ok := d["name"]; ok {
		d["name"] = strings.TrimSpace(str(d, "name"))
		if d["name"] == "" {
			return nil, bad("Name required")
		}
	}
	return d, validColor(d)
}
func HandleTagsCreate(w http.ResponseWriter, r *http.Request) {
	b, ok := read(w, r)
	if !ok {
		return
	}
	d, e := tagData(b)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	if str(d, "name") == "" {
		respond(w, 0, nil, bad("Name required"))
		return
	}
	d["id"], d["ownerId"] = db.NewID(), user(r)
	v, e := db.Insert(r.Context(), database(r), "Tag", d)
	respond(w, 201, v, e)
}
func HandleTagsUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, e := owned(r, database(r), "Tag", id, "ownerId"); e != nil {
		respond(w, 0, nil, e)
		return
	}
	b, ok := read(w, r)
	if !ok {
		return
	}
	d, e := tagData(b)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	list, e := ids(b, "shareUserIds")
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	d["updatedAt"] = time.Now().UTC()
	var v object
	e = db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		var e error
		v, e = db.Update(r.Context(), tx, "Tag", id, "ownerId", user(r), d)
		if e != nil {
			return e
		}
		return shareRows(r, tx, "TagShare", "tagId", id, list)
	})
	respond(w, 200, v, e)
}
func HandleTagsDelete(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, OK{true}, remove(r, "Tag", r.PathValue("id"), "ownerId"))
}
