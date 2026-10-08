package handlers

import (
	"database/sql"
	"github.com/andrew/go-computer/internal/db"
	"github.com/andrew/go-computer/internal/kanban"
	"net/http"
	"strings"
)

func HandleKanbanColumnsList(w http.ResponseWriter, r *http.Request) {
	v, e := kanban.GetColumns(r.Context(), database(r), user(r))
	respond(w, 200, v, e)
}
func columnData(b object) (object, error) {
	d := fields(b, "label color")
	if e := stringFields(d, "", "label color"); e != nil {
		return nil, e
	}
	if _, ok := d["label"]; ok {
		s := strings.TrimSpace(str(d, "label"))
		if s == "" {
			return nil, bad("Label required")
		}
		r := []rune(s)
		if len(r) > 40 {
			r = r[:40]
		}
		d["label"] = string(r)
	}
	return d, validColor(d)
}
func HandleKanbanColumnsCreate(w http.ResponseWriter, r *http.Request) {
	b, ok := read(w, r)
	if !ok {
		return
	}
	d, e := columnData(b)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	if str(d, "label") == "" {
		respond(w, 0, nil, bad("Label required"))
		return
	}
	if _, ok := d["color"]; !ok {
		d["color"] = "#8b5cf6"
	}
	d["id"], d["userId"] = db.NewID(), user(r)
	var v object
	e = db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		if e := lockUser(r, tx); e != nil {
			return e
		}
		var n int
		if e := tx.QueryRowContext(r.Context(), `SELECT COALESCE(max(position),-1)+1 FROM "KanbanColumn" WHERE "userId"=$1`, user(r)).Scan(&n); e != nil {
			return e
		}
		d["position"] = n
		var e error
		v, e = db.Insert(r.Context(), tx, "KanbanColumn", d)
		return e
	})
	respond(w, 201, v, e)
}
func HandleKanbanColumnsReorder(w http.ResponseWriter, r *http.Request) {
	b, ok := read(w, r)
	if !ok {
		return
	}
	order, e := ids(b, "order")
	if e == nil {
		e = reorder(r, "KanbanColumn", "userId", order)
	}
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	HandleKanbanColumnsList(w, r)
}
func HandleKanbanColumnsUpdate(w http.ResponseWriter, r *http.Request) {
	b, ok := read(w, r)
	if !ok {
		return
	}
	d, e := columnData(b)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	v, e := db.Update(r.Context(), database(r), "KanbanColumn", r.PathValue("id"), "userId", user(r), d)
	respond(w, 200, v, e)
}
func HandleKanbanColumnsDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var dest string
	var moved int
	e := db.Transaction(r.Context(), database(r), func(tx *sql.Tx) error {
		if e := lockUser(r, tx); e != nil {
			return e
		}
		cols, e := db.Many(r.Context(), tx, `SELECT to_jsonb(c) FROM "KanbanColumn" c WHERE "userId"=$1 ORDER BY position,id`, user(r))
		if e != nil {
			return e
		}
		found := false
		remaining := []object{}
		for _, c := range cols {
			if str(c, "id") == id {
				found = true
			} else {
				remaining = append(remaining, c)
			}
		}
		if !found {
			return db.ErrNotFound
		}
		if len(remaining) == 0 {
			return bad("A board needs at least one column")
		}
		dest = str(remaining[0], "id")
		bms, e := db.Many(r.Context(), tx, `SELECT to_jsonb(b) FROM "Bookmark" b WHERE "ownerId"=$1`, user(r))
		if e != nil {
			return e
		}
		for _, b := range bms {
			if kanban.ResolveColumnID(b, cols) == id {
				if _, e := tx.ExecContext(r.Context(), `UPDATE "Bookmark" SET "kanbanColumnId"=$1,"updatedAt"=now() WHERE id=$2`, dest, b["id"]); e != nil {
					return e
				}
				moved++
			}
		}
		if _, e := tx.ExecContext(r.Context(), `DELETE FROM "KanbanColumn" WHERE id=$1 AND "userId"=$2`, id, user(r)); e != nil {
			return e
		}
		for i, c := range remaining {
			if _, e := tx.ExecContext(r.Context(), `UPDATE "KanbanColumn" SET position=$1 WHERE id=$2`, i, c["id"]); e != nil {
				return e
			}
		}
		return nil
	})
	respond(w, 200, object{"ok": true, "movedTo": dest, "moved": moved}, e)
}
