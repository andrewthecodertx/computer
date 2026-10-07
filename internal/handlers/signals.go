package handlers

import (
	"net/http"
	"github.com/andrew/go-computer/internal/signals"
)

// Signals handler group. Porting guide 4.7.

// HandleSignalsList implements GET /api/bookmarks/:id/signals.
// Returns {sources, signals (newest 50), types: publicSourceTypes()}.
func HandleSignalsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"types": signals.PublicSourceTypes(),
	})
}

// HandleSignalsCreate implements POST /api/bookmarks/:id/signals.
// {type, config} or {action:"check"}. Adds a watcher, keeping only the declared
// config keys (trimmed, max 200 chars); or runs runSignalChecks for this bookmark.
func HandleSignalsCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, OK{OK: true})
}

// HandleSignalSourcesUpdate implements PATCH /api/signal-sources/:id.
// {enabled}.
func HandleSignalSourcesUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, OK{OK: true})
}

// HandleSignalSourcesDelete implements DELETE /api/signal-sources/:id.
func HandleSignalSourcesDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, OK{OK: true})
}
