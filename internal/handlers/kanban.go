package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/andrew/go-computer/internal/kanban"
)

// Kanban columns handler group. Porting guide 4.5.
//
// A user always has at least one column. Deleting a column moves its bookmarks
// and never deletes them. Moving a card = PUT /api/bookmarks/:id {kanbanColumnId}.

// HandleKanbanColumnsList implements GET /api/kanban/columns.
// Returns getColumns(userId).
func HandleKanbanColumnsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	// TODO: resolve effective user, call kanban.GetColumns.
	writeJSON(w, http.StatusOK, []kanban.Column{})
}

// HandleKanbanColumnsCreate implements POST /api/kanban/columns.
// Label trimmed to 40 chars; color default #8b5cf6; appended at the end.
func HandleKanbanColumnsCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Label  string `json:"label"`
		Color  string `json:"color"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	writeJSON(w, http.StatusCreated, OK{OK: true})
}

// HandleKanbanColumnsReorder implements PUT /api/kanban/columns.
// {order: id[]} — reorders my columns (in a transaction); returns the columns.
func HandleKanbanColumnsReorder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, []kanban.Column{})
}

// HandleKanbanColumnsUpdate implements PATCH /api/kanban/columns/:id.
// {label?, color?}.
func HandleKanbanColumnsUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, OK{OK: true})
}

// HandleKanbanColumnsDelete implements DELETE /api/kanban/columns/:id.
// Refuses to delete the last column. Bookmarks that resolve to this column move
// to the first remaining column; the rest are renumbered. Returns
// {ok, moveTo, moved}. Never deletes bookmarks.
func HandleKanbanColumnsDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}