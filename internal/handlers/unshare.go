package handlers

import (
	"github.com/andrew/go-computer/internal/db"
	"net/http"
)

func unshare(w http.ResponseWriter, r *http.Request, parent, join, key string) {
	id := r.PathValue("id")
	if _, e := owned(r, database(r), parent, id, "ownerId"); e != nil {
		respond(w, 0, nil, e)
		return
	}
	_, e := database(r).ExecContext(r.Context(), `DELETE FROM `+db.Quote(join)+` WHERE `+db.Quote(key)+`=$1 AND "userId"=$2`, id, r.PathValue("userId"))
	respond(w, 200, OK{true}, e)
}
func HandleBookmarkUnshare(w http.ResponseWriter, r *http.Request) {
	unshare(w, r, "Bookmark", "BookmarkShare", "bookmarkId")
}
func HandleTagUnshare(w http.ResponseWriter, r *http.Request) {
	unshare(w, r, "Tag", "TagShare", "tagId")
}
func HandleSharedDateDelete(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, OK{true}, remove(r, "SharedDate", r.PathValue("id"), "creatorId"))
}
