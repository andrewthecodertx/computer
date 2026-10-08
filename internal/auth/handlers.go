package auth

import (
	"encoding/json"
	"errors"
	"github.com/andrew/go-computer/internal/config"
	"github.com/andrew/go-computer/internal/db"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
)

const bcryptCost = 12

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func failure(w http.ResponseWriter, status int, s string) {
	writeJSON(w, status, map[string]string{"error": s})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if json.NewDecoder(r.Body).Decode(v) != nil {
		failure(w, 400, "Invalid JSON")
		return false
	}
	return true
}
func HandleSignup(w http.ResponseWriter, r *http.Request) {
	var req SignupRequest
	if !decode(w, r, &req) {
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" {
		failure(w, 400, "Email and password are required")
		return
	}
	if a, e := mail.ParseAddress(req.Email); e != nil || a.Address != req.Email {
		failure(w, 400, "Invalid email")
		return
	}
	if len(req.Password) > 72 {
		failure(w, 400, "Password must be at most 72 bytes")
		return
	}
	if config.IsAdminEmail(req.Email) {
		failure(w, 403, "This administrator account must be provisioned by the operator")
		return
	}
	repo := db.NewUserRepo(db.FromContext(r.Context()))
	if _, e := repo.FindByEmail(r.Context(), req.Email); e == nil {
		failure(w, 409, "User already exists")
		return
	} else if !errors.Is(e, db.ErrNotFound) {
		failure(w, 500, "Database unavailable")
		return
	}
	hash, e := bcryptHashImpl(req.Password)
	if e != nil {
		failure(w, 500, "Unable to hash password")
		return
	}
	if req.Name == "" {
		req.Name = strings.SplitN(req.Email, "@", 2)[0]
	}
	id, e := repo.Create(r.Context(), &db.NewUserInput{ID: db.NewID(), Name: req.Name, Email: req.Email, Password: hash, Role: "USER"})
	if e != nil {
		var pg *pgconn.PgError
		if errors.As(e, &pg) && pg.Code == "23505" {
			failure(w, 409, "User already exists")
		} else {
			failure(w, 500, "Unable to create user")
		}
		return
	}
	writeJSON(w, 201, SignupResponse{true, id})
}
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	if strings.ToLower(strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0])) != "application/json" {
		failure(w, 415, "Content-Type must be application/json")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		allowed, err := url.Parse(config.Load().NextAuthURL)
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if origin != scheme+"://"+r.Host && (err != nil || allowed.Host == "" || origin != allowed.Scheme+"://"+allowed.Host) {
			failure(w, 403, "Invalid request origin")
			return
		}
	}
	var req SigninRequest
	if !decode(w, r, &req) {
		return
	}
	u, e := db.NewUserRepo(db.FromContext(r.Context())).FindByEmail(r.Context(), strings.ToLower(strings.TrimSpace(req.Email)))
	if e != nil || u.Password == nil || !bcryptCheckImpl(req.Password, *u.Password) {
		failure(w, 401, "Invalid credentials")
		return
	}
	if e = WriteSessionCookie(w, SessionUser{ID: u.ID, Role: u.Role}, config.Load().Secret()); e != nil {
		failure(w, 500, "Session unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "user": identity(u)})
}
func HandleAuth(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/auth/session":
		su, e := ReadSession(r, config.Load().Secret())
		if e != nil {
			writeJSON(w, 200, nil)
			return
		}
		u, e := db.NewUserRepo(db.FromContext(r.Context())).FindByID(r.Context(), su.ID)
		if e != nil {
			writeJSON(w, 200, nil)
			return
		}
		writeJSON(w, 200, map[string]any{"user": identity(u)})
	case "/api/auth/signout":
		if r.Method != "POST" {
			failure(w, 405, "Use POST to sign out")
			return
		}
		ClearSessionCookie(w)
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		failure(w, 404, "Unknown authentication endpoint")
	}
}
func HandleMe(w http.ResponseWriter, r *http.Request) {
	s, _ := SessionFromContext(r.Context())
	var viewing any
	if s.ID != s.RealID {
		viewing = map[string]any{"id": s.ID, "name": s.Name, "email": s.Email}
	}
	writeJSON(w, 200, map[string]any{"user": s.RealUser, "effectiveUser": map[string]any{"id": s.ID, "name": s.Name, "email": s.Email, "role": s.Role}, "isAdmin": s.RealRole == "ADMIN", "viewingAs": viewing})
}
