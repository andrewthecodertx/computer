package auth

import (
	"context"
	"net/http"

	"github.com/andrew/go-computer/internal/config"
	"github.com/andrew/go-computer/internal/db"
	"github.com/andrew/go-computer/internal/handlers"
)

// SessionUser is the verified identity from the signed cookie.
type SessionUser struct {
	ID   string
	Role string
}

// AuthMiddleware wraps a handler, resolving the session user from the signed
// cookie and attaching it to the request context. 401 if missing/invalid.
//
// Every data endpoint follows the same pattern: resolve the effective user,
// return 401 if none, check ownership, return 404 if not theirs, then act
// (porting guide 5.1).
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret := config.Load().Secret()
		su, err := ReadSession(r, secret)
		if err != nil {
			handlers.WriteJSON(w, http.StatusUnauthorized, handlers.Unauthorized("Unauthorized"))
			return
		}
		// Re-verify the user still exists and refresh the role from the DB so
		// promotions (ADMIN_EMAILS) take effect without re-login.
		d := db.FromContext(r.Context())
		if d != nil {
			repo := db.NewUserRepo(d)
			u, err := repo.FindByID(r.Context(), su.ID)
			if err != nil {
				handlers.WriteJSON(w, http.StatusUnauthorized, handlers.Unauthorized("Unauthorized"))
				return
			}
			su.Role = u.Role
		}
		ctx := WithSession(r.Context(), su)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SessionFromContext pulls the verified session user from the request context.
func SessionFromContext(ctx context.Context) (*SessionUser, bool) {
	v := ctx.Value(sessionKey)
	if v == nil {
		return nil, false
	}
	su, ok := v.(*SessionUser)
	return su, ok
}

type sessionKeyType struct{}

var sessionKey sessionKeyType

// WithSession attaches the session user to the context.
func WithSession(ctx context.Context, su *SessionUser) context.Context {
	return context.WithValue(ctx, sessionKey, su)
}

// RequireAdmin wraps a handler and returns 403 unless the session user is ADMIN.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		su, ok := SessionFromContext(r.Context())
		if !ok || su.Role != "ADMIN" {
			handlers.WriteJSON(w, http.StatusForbidden, handlers.Forbidden("Forbidden"))
			return
		}
		next.ServeHTTP(w, r)
	})
}
