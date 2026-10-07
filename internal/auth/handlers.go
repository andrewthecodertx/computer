package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/andrew/go-computer/internal/config"
	"github.com/andrew/go-computer/internal/db"
	"github.com/andrew/go-computer/internal/handlers"
	"github.com/golang-jwt/jwt/v5"
)

// bcryptCost mirrors the original app: bcrypt cost 12 (porting guide 3.1).
const bcryptCost = 12

// HandleSignup implements POST /api/signup (4.1).
// 400 if missing; 409 if email exists; bcrypt(12); name defaults to the part of
// the email before @; role ADMIN if no admin exists yet (ignoring @example.com
// test users). Returns 201 {ok, userId}.
func HandleSignup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.WriteJSON(w, http.StatusMethodNotAllowed, handlers.BadRequest("method not allowed"))
		return
	}
	var req SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.WriteJSON(w, http.StatusBadRequest, handlers.BadRequest("invalid json"))
		return
	}
	if req.Email == "" || req.Password == "" {
		handlers.WriteJSON(w, http.StatusBadRequest, handlers.BadRequest("Email and password are required"))
		return
	}
	if !strings.Contains(req.Email, "@") {
		handlers.WriteJSON(w, http.StatusBadRequest, handlers.BadRequest("Invalid email"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	repo := db.NewUserRepo(dbFromRequest(r))
	if _, err := repo.FindByEmail(ctx, req.Email); err == nil {
		handlers.WriteJSON(w, http.StatusConflict, handlers.Conflict("User already exists"))
		return
	} else if !errors.Is(err, db.ErrNotFound) {
		handlers.WriteJSON(w, http.StatusInternalServerError, handlers.BadRequest("server error"))
		return
	}

	hashed, err := bcryptHash(req.Password)
	if err != nil {
		handlers.WriteJSON(w, http.StatusInternalServerError, handlers.BadRequest("server error"))
		return
	}

	name := req.Name
	if name == "" {
		name = strings.SplitN(req.Email, "@", 2)[0]
	}

	role := "USER"
	if !strings.HasSuffix(req.Email, "@example.com") {
		exists, err := repo.ExistsAnyNonTest(ctx)
		if err == nil && !exists {
			role = "ADMIN"
		}
	}

	user := &db.NewUserInput{
		ID:       cuid(),
		Name:     name,
		Email:    req.Email,
		Password: hashed,
		Role:     role,
	}
	uid, err := repo.Create(ctx, user)
	if err != nil {
		handlers.WriteJSON(w, http.StatusInternalServerError, handlers.BadRequest("server error"))
		return
	}

	handlers.WriteJSON(w, http.StatusCreated, SignupResponse{OK: true, UserID: uid})
}

// HandleLogin implements POST /api/auth/login (4.1).
// Server-side credentials sign-in; 401 on failure.
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.WriteJSON(w, http.StatusMethodNotAllowed, handlers.BadRequest("method not allowed"))
		return
	}
	var req SigninRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.WriteJSON(w, http.StatusBadRequest, handlers.BadRequest("invalid json"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	repo := db.NewUserRepo(dbFromRequest(r))
	u, err := repo.FindByEmail(ctx, req.Email)
	if err != nil || u.Password == nil || !bcryptCheck(req.Password, *u.Password) {
		handlers.WriteJSON(w, http.StatusUnauthorized, handlers.Unauthorized("Invalid credentials"))
		return
	}

	secret := config.Load().Secret()
	if err := WriteSessionCookie(w, SessionUser{ID: u.ID, Role: u.Role}, secret); err != nil {
		handlers.WriteJSON(w, http.StatusInternalServerError, handlers.BadRequest("server error"))
		return
	}
	handlers.WriteJSON(w, http.StatusOK, handlers.OK{OK: true})
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
		handlers.WriteJSON(w, http.StatusMethodNotAllowed, handlers.BadRequest("method not allowed"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	su, ok := SessionFromContext(r.Context())
	if !ok {
		handlers.WriteJSON(w, http.StatusUnauthorized, handlers.Unauthorized("Unauthorized"))
		return
	}
	repo := db.NewUserRepo(dbFromRequest(r))
	u, err := repo.FindByID(ctx, su.ID)
	if err != nil {
		handlers.WriteJSON(w, http.StatusUnauthorized, handlers.Unauthorized("Unauthorized"))
		return
	}
	// Promote via ADMIN_EMAILS on first check (porting guide 3.2).
	role := u.Role
	if role != "ADMIN" && u.Email != nil && db.IsAdminEmail(*u.Email) {
		_ = repo.SetRole(ctx, u.ID, "ADMIN")
		role = "ADMIN"
	}
	out := map[string]interface{}{
		"user": map[string]interface{}{
			"id":    u.ID,
			"name":  u.Name,
			"email": u.Email,
			"image": u.Image,
			"role":  role,
		},
		"isAdmin": role == "ADMIN",
	}
	// TODO: view-as cookie -> viewingAs: {id, name, email} | null.
	out["viewingAs"] = nil
	handlers.WriteJSON(w, http.StatusOK, out)
}

// --- helpers ---

// dbFromRequest pulls the *db.DB from the request context. The http.Server
// wires the DB into the context at startup.
func dbFromRequest(r *http.Request) *db.DB {
	return db.FromContext(r.Context())
}

// --- bcrypt ---

func bcryptHash(password string) (string, error) {
	// golang.org/x/crypto/bcrypt
	return bcryptHashImpl(password)
}

func bcryptCheck(password, hash string) bool {
	return bcryptCheckImpl(password, hash)
}

var _ = jwt.SigningMethodHS256
var _ = fmt.Sprintf
