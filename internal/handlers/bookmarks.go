package handlers

import (
	"encoding/json"
	"net/http"
)

// Bookmarks handler group. Porting guide 4.3.
//
// Every query is scoped to the effective user; never trust a user ID from the
// client. "Owner" means ownerId = effective user.
//
// Visible if mine, shared with me, or public (GET :id).
// A new URL re-fetches the preview. tagIds replaces all tags.
// Setting alertAt resets alertSent. Sharing only adds; no unshare yet.

// HandleBookmarksList implements GET /api/bookmarks.
// Query: search, tagId, contactId, kanban, date, shared.
// shared=true -> bookmarks shared WITH me; otherwise mine.
// search matches title/url/notes/tag name, case-insensitive.
// kanban filters the legacy status. date = that calendar day of dueDate.
// Includes tags, contact, shares, owner. Newest-updated first, max 200.
func HandleBookmarksList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	// TODO: resolve effective user (401 if none), query with filters.
	writeJSON(w, http.StatusOK, []interface{}{})
}

// HandleBookmarksCreate implements POST /api/bookmarks.
// 400 without url; fetches the preview; title falls back to ogTitle, then the
// url. Returns 201.
func HandleBookmarksCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req BookmarkCreateRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	// TODO: validate url, fetch preview, create.
	writeJSON(w, http.StatusCreated, OK{OK: true})
}

// HandleBookmarksGet implements GET /api/bookmarks/:id.
func HandleBookmarksGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	// TODO: visible if mine, shared with me, or public. Includes tags, contact,
	// shares with user info, owner, and pageLinks (only MY pages).
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

// HandleBookmarksUpdate implements PUT /api/bookmarks/:id.
// Owner only. Only fields present in the body change. kanbanColumnId must be
// my column (400). A new URL re-fetches the preview. tagIds replaces all tags.
// Setting alertAt resets alertSent.
func HandleBookmarksUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	// TODO: owner-only check, partial update.
	writeJSON(w, http.StatusOK, OK{OK: true})
}

// HandleBookmarksDelete implements DELETE /api/bookmarks/:id.
// Owner only.
func HandleBookmarksDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	// TODO: owner-only check, delete.
	writeJSON(w, http.StatusOK, OK{OK: true})
}

// HandleBookmarksShare implements POST /api/bookmarks/:id/share.
// Owner only; upserts shares (adding only, never removing); sets public flag.
func HandleBookmarksShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	// TODO: owner-only, upsert shares, set isPublic.
	writeJSON(w, http.StatusOK, OK{OK: true})
}

// HandleBookmarksMarkdown implements GET /api/bookmarks/:id/markdown.
// .md download, mine or shared with me.
func HandleBookmarksMarkdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	// TODO: build markdownResponse, stream with text/markdown content type.
}

// HandlePreview implements GET /api/preview?url=.
// Runs fetchPreview (3.6).
func HandlePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	// TODO: call preview.FetchPreview(r.URL.Query().Get("url")).
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

// BookmarkCreateRequest is the POST /api/bookmarks body.
type BookmarkCreateRequest struct {
	URL               string   `json:"url"`
	Title             *string  `json:"title"`
	Description       *string  `json:"description"`
	Notes             *string  `json:"notes"`
	DueDate           *string  `json:"dueDate"`
	AlertAt           *string  `json:"alertAt"`
	KanbanStatus      string   `json:"kanbanStatus"`
	ContactID         *string  `json:"contactId"`
	TagIDs            []string `json:"tagIds"`
	IsPublic          *bool    `json:"isPublic"`
	ImapWatchEnabled  *bool    `json:"imapWatchEnabled"`
	ImapQuery         *string  `json:"imapQuery"`
}
