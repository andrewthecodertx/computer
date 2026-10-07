package authguard

import (
	"encoding/json"
	"net/http"
)

// HandleAdminListUsers implements GET /api/admin/users (4.2).
// All users except @example.com, with counts of bookmarks, pages and tags,
// oldest first. Uses the REAL user; 403 unless ADMIN.
func HandleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	// TODO: require real user with role ADMIN (403 otherwise), list users with counts.
	writeJSON(w, http.StatusOK, []interface{}{})
}

// HandleAdminPatchUser implements PATCH /api/admin/users (4.2).
// {userId, role} — role must be USER/ADMIN; you cannot demote yourself.
func HandleAdminPatchUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		UserID string `json:"userId"`
		Role   string `json:"role"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	// TODO: validate role, prevent self-demotion.
	writeJSON(w, http.StatusOK, OK{OK: true})
}

// HandleAdminViewAs implements POST /api/admin/view-as (4.2).
// {userId | null} — null or own ID clears the cookie. Otherwise checks the user
// exists and sets an httpOnly, secure, SameSite=Lax cookie (path /, 8 hours).
func HandleAdminViewAs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	// TODO: require ADMIN real user; set/clear VIEW_AS_COOKIE.
	writeJSON(w, http.StatusOK, OK{OK: true})
}

// OK is the success response shape for mutations.
type OK struct {
	OK bool `json:"ok"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}