package auth

import (
	"encoding/json"
	"errors"
	"github.com/andrew/go-computer/internal/config"
	"github.com/andrew/go-computer/internal/db"
	"github.com/jackc/pgx/v5/pgconn"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"
)

const bcryptCost = 12

// dummyPasswordHash is bcrypt-cost-12 hash of a value no user can supply.
// Login compares against it for unknown accounts so timing does not reveal
// which emails are registered.
const dummyPasswordHash = "$2a$12$OeKCcVovKAAb7gKAhWqA..yGR3eoqGQAKzSrWwLryJgAro0Vc60WW"

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
	if !checkAuthRequest(w, r) {
		return
	}
	if os.Getenv("SIGNUP_ENABLED") == "0" {
		failure(w, 403, "Registration is disabled")
		return
	}
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
	if len(req.Password) < 8 {
		failure(w, 400, "Password must be at least 8 characters")
		return
	}
	if len(req.Password) > 72 {
		failure(w, 400, "Password must be at most 72 bytes")
		return
	}
	repo := db.NewUserRepo(db.FromContext(r.Context()))
	// Account enumeration (review SEC-4, CR-03): a distinct 409 — or a
	// distinct response BODY — for a registered address is an unauthenticated
	// existence oracle. Duplicate registrations (and claims of
	// operator-reserved addresses) are logged for the operator and answered
	// with the exact same 201 {"ok":true} body as a real creation; the
	// internal user id never leaves the server before authentication
	// establishes ownership (/api/me). The subsequent login then either
	// succeeds (the caller is the owner) or fails on credentials like any
	// wrong password.
	var existing bool
	if _, e := repo.FindByEmail(r.Context(), req.Email); e == nil {
		existing = true
	} else if !errors.Is(e, db.ErrNotFound) {
		failure(w, 500, "Database unavailable")
		return
	}
	if config.IsAdminEmail(req.Email) {
		log.Printf("signup: rejected claim of operator-reserved address %s", req.Email)
		existing = true
	}
	if existing {
		log.Printf("signup: duplicate registration attempt")
		// Same bcrypt cost as real registrations, so timing does not
		// re-open the existence oracle.
		_, _ = bcryptHashImpl(req.Password)
		writeJSON(w, 201, SignupResponse{true})
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
	if _, e := repo.Create(r.Context(), &db.NewUserInput{ID: db.NewID(), Name: req.Name, Email: req.Email, Password: hash, Role: "USER"}); e != nil {
		var pg *pgconn.PgError
		if errors.As(e, &pg) && pg.Code == "23505" {
			// Racing creation: still indistinguishable.
			writeJSON(w, 201, SignupResponse{true})
			return
		}
		failure(w, 500, "Unable to create user")
		return
	}
	writeJSON(w, 201, SignupResponse{true})
}

// checkAuthRequest enforces the JSON Content-Type and same-origin guards on
// the unauthenticated auth endpoints. Applied to login and signup alike
// (review SEC-4): both are reachable directly on the Go API when the Next
// proxy's own Origin checks are bypassed.
func checkAuthRequest(w http.ResponseWriter, r *http.Request) bool {
	if strings.ToLower(strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0])) != "application/json" {
		failure(w, 415, "Content-Type must be application/json")
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		allowed, err := url.Parse(config.Load().NextAuthURL)
		scheme := "http"
		// Behind a TLS-terminating proxy r.TLS is nil; X-Forwarded-Proto only
		// affects the scheme half — the host must still match Origin, so this
		// grants a spoofer nothing they control end-to-end.
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		if origin != scheme+"://"+r.Host && (err != nil || allowed.Host == "" || origin != allowed.Scheme+"://"+allowed.Host) {
			failure(w, 403, "Invalid request origin")
			return false
		}
	}
	return true
}
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	if !checkAuthRequest(w, r) {
		return
	}
	var req SigninRequest
	if !decode(w, r, &req) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	// Review SEC-1: the failure lockout applies to every peer, loopback
	// included — the exemption previously removed all guessing protection
	// from same-host deployments.
	if retry, locked := loginFailures.locked(EffectiveClientIP(r)+"|"+email, time.Now()); locked {
		tooManyRequests(w, retry)
		return
	}
	u, e := db.NewUserRepo(db.FromContext(r.Context())).FindByEmail(r.Context(), email)
	if e != nil && !errors.Is(e, db.ErrNotFound) {
		failure(w, 500, "Database unavailable")
		return
	}
	// Unknown or password-less accounts still pay a bcrypt comparison so
	// response latency does not reveal which emails are registered.
	ok := false
	if e == nil && u.Password != nil {
		ok = bcryptCheckImpl(req.Password, *u.Password)
	} else {
		bcryptCheckImpl(req.Password, dummyPasswordHash)
	}
	if !ok {
		loginFailures.noteFailure(EffectiveClientIP(r)+"|"+email, time.Now())
		failure(w, 401, "Invalid credentials")
		return
	}
	loginFailures.noteSuccess(EffectiveClientIP(r) + "|" + email)
	if e = WriteSessionCookie(w, SessionUser{ID: u.ID, Role: u.Role, Ver: u.TokenVersion}, config.Load().Secret()); e != nil {
		failure(w, 500, "Session unavailable")
		return
	}
	// tokenVersion rides along so the Next server can embed it in its session
	// JWT and in every assertion it signs (review SEC-02); a revocation bump
	// then invalidates NextAuth-backed sessions on their next backend call.
	writeJSON(w, 200, map[string]any{"ok": true, "user": identity(u), "tokenVersion": u.TokenVersion})
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
			// Session error classification (review follow-up): distinguish
			// authentication absence from backend degradation, exactly like
			// AuthMiddleware — an unreachable database must answer 503 so
			// client retry logic and monitoring see the difference instead of
			// a degraded backend masquerading as an ordinary logout.
			if errors.Is(e, db.ErrNotFound) {
				writeJSON(w, 200, nil)
			} else {
				failure(w, 503, "Service unavailable")
			}
			return
		}
		if su.Ver > 0 && su.Ver != u.TokenVersion {
			writeJSON(w, 200, nil)
			return
		}
		writeJSON(w, 200, map[string]any{"user": identity(u)})
	case "/api/auth/signout":
		if r.Method != "POST" {
			failure(w, 405, "Use POST to sign out")
			return
		}
		// Revoke every cookie JWT issued for this user, not just this
		// browser's copy: signout means "sign out everywhere".
		if su, e := ReadSession(r, config.Load().Secret()); e == nil {
			if e := db.NewUserRepo(db.FromContext(r.Context())).BumpTokenVersion(r.Context(), su.ID); e != nil {
				writeJSON(w, 500, map[string]string{"error": "Unable to sign out"})
				return
			}
		}
		ClearSessionCookie(w)
		// Review SEC-6: sign-out ends impersonation too — otherwise the next
		// admin login on that browser would silently resume view-as.
		http.SetCookie(w, &http.Cookie{Name: ViewAsCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
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
