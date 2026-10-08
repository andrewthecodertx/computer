package handlers

import "net/http"

func HandlePublicBookmark(w http.ResponseWriter, r *http.Request) {
	v, e := bookmark(r, r.PathValue("id"), true)
	// bookmark's public projection is an allowlist; private metadata is never loaded.
	respond(w, 200, v, e)
}
