package handlers

import "net/http"

// HandlePublicBookmark serves the unauthenticated share page. The projection
// in bookmark(..., public=true) is an allowlist; private metadata (contact,
// owner identity, imap settings, alerts, kanban placement, shares, page
// links) is never loaded — review PRIV-04 removed the owner-name attribution
// so the API matches the README's promise that owner identity stays private.
// Notes and dueDate ARE published by product decision — the share UI states
// exactly what a public link exposes.
func HandlePublicBookmark(w http.ResponseWriter, r *http.Request) {
	v, e := bookmark(r, r.PathValue("id"), true)
	respond(w, 200, v, e)
}
