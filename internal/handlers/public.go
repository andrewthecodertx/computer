package handlers

import (
	"net/http"
)

// HandlePublicBookmark implements GET /share/bookmark/:id (4.9).
// No login required. Shows a read-only page for a bookmark where isPublic =
// true; otherwise not found.
func HandlePublicBookmark(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	// TODO: look up bookmark by id, 404 if !isPublic, return the public view.
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}
