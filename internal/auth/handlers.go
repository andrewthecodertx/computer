package auth

import (
	"encoding/json"
	"net/http"

	"github.com/andrew/go-computer/internal/handlers"
)

// HandleSignup implements POST /api/signup (4.1).
// 400 if missing; 409 if email exists; bcrypt(12); name defaults to the part of
// the email before @; role ADMIN if no admin exists yet (ignoring @example.com
// test users). Returns 201 {ok, userId}.
func HandleSignup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, handlers.BadRequest("invalid json"))
		return
	}
	// TODO: validate, check existing email (409), hash password, create user,
	// promote first real signup to ADMIN.
	writeJSON(w, http.StatusCreated, handlers.OK{OK: true})
}

// HandleLogin implements POST /api/auth/login (4.1).
// Server-side credentials sign-in; 401 on failure.
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	// TODO: decode {email, password}, call Authorize, set signed session cookie.
	writeJSON(w, http.StatusOK, handlers.OK{})
}

// HandleAuth proxies the auth library endpoints: GET/POST /api/auth/*
// (csrf, callback/credentials, callback/authelia, session, signout).
func HandleAuth(w http.ResponseWriter, r *http.Request) {
	// TODO: route to csrf / callback / session / signout handlers.
}

// HandleMe implements GET /api/me (4.1).
// Returns {user: realUser, isAdmin, viewingAs: {id, name, email} | null}.
func HandleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	// TODO: resolve real user and view-as target from cookie.
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}