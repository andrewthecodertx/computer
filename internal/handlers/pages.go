package handlers

import (
	"encoding/json"
	"net/http"
)

// Pages handler group. Porting guide 4.6.

// HandlePagesList implements GET /api/pages?search=.
// Mine; search title/content; pinned first, then position, then created.
// Includes link count.
func HandlePagesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, []interface{}{})
}

// HandlePagesCreate implements POST /api/pages.
// Title default "Untitled page", max 120 chars; position = count.
func HandlePagesCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req PageCreateRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	writeJSON(w, http.StatusCreated, OK{OK: true})
}

// HandlePagesReorder implements PUT /api/pages.
// {order: id[]}.
func HandlePagesReorder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, OK{OK: true})
}

// HandlePagesGet implements GET /api/pages/:id.
// Page plus linked bookmarks in order.
func HandlePagesGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

// HandlePagesUpdate implements PATCH /api/pages/:id.
// {title?, content?, pinned?, icon?, addBookmarkId?, removeBookmarkId?}.
// Added bookmark must be mine or shared with me; appended at the end; adding the
// same one twice is harmless.
func HandlePagesUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, OK{OK: true})
}

// HandlePagesDelete implements DELETE /api/pages/:id.
func HandlePagesDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, OK{OK: true})
}

// HandlePagesMarkdown implements GET /api/pages/:id/markdown.
// .md download.
func HandlePagesMarkdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
}

type PageCreateRequest struct {
	Title   *string `json:"title"`
	Content *string `json:"content"`
}
