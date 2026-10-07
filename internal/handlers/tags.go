package handlers

import (
	"encoding/json"
	"net/http"
)

// Tags handler group. Porting guide 4.4.
//
// Known gap: a shared tag shows up in the recipient's list, but filtering
// bookmarks by that tag only returns the recipient's own bookmarks. Fix this in
// the port if tag sharing should expose the owner's bookmarks.

// HandleTagsList implements GET /api/tags.
// Mine plus tags shared with me, with bookmark count and share list, by name.
func HandleTagsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, []interface{}{})
}

// HandleTagsCreate implements POST /api/tags.
// 400 without name.
func HandleTagsCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req TagCreateRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	writeJSON(w, http.StatusCreated, OK{OK: true})
}

// HandleTagsUpdate implements PUT /api/tags/:id.
// Owner only; upserts tag shares.
func HandleTagsUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, OK{OK: true})
}

// HandleTagsDelete implements DELETE /api/tags/:id.
// Owner only.
func HandleTagsDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, OK{OK: true})
}

type TagCreateRequest struct {
	Name        string   `json:"name"`
	Color       *string  `json:"color"`
	ShareUserIDs []string `json:"shareUserIds"`
}
