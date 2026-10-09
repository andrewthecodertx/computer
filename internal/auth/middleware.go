package auth

import (
	"context"
	"github.com/andrew/go-computer/internal/config"
	"github.com/andrew/go-computer/internal/db"
	"github.com/andrew/go-computer/internal/models"
	"net/http"
)

const ViewAsCookie = "computer_view_as"

type SessionUser struct {
	ID, Role, RealID, RealRole string
	Name, Email                *string
	RealUser                   map[string]any
	// Ver is the tokenVersion embedded in a cookie JWT (0 = no claim:
	// assertions and legacy cookies), checked against the user row so
	// signout can revoke every issued cookie session.
	Ver int
}

func identity(u *models.User) map[string]any {
	return map[string]any{"id": u.ID, "name": u.Name, "email": u.Email, "image": u.Image, "role": u.Role}
}
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, e := ReadSession(r, config.Load().Secret())
		if e != nil {
			failure(w, 401, "Unauthorized")
			return
		}
		repo := db.NewUserRepo(db.FromContext(r.Context()))
		u, e := repo.FindByID(r.Context(), s.ID)
		if e != nil {
			failure(w, 401, "Unauthorized")
			return
		}
		// Revoked session: the cookie's tokenVersion predates a signout bump.
		if s.Ver > 0 && s.Ver != u.TokenVersion {
			failure(w, 401, "Unauthorized")
			return
		}
		// Promotion via ADMIN_EMAILS is persistent by design: removing an
		// address from the env var later does NOT demote the account (use the
		// Admin screen). Silent demotion on env drift would be worse than an
		// extra explicit revocation step.
		if u.Role != "ADMIN" && u.Email != nil && u.EmailVerified != nil && config.IsAdminEmail(*u.Email) {
			if e = repo.SetRole(r.Context(), u.ID, "ADMIN"); e != nil {
				failure(w, 500, "Unable to resolve role")
				return
			}
			u.Role = "ADMIN"
		}
		s.RealID, s.RealRole, s.RealUser = u.ID, u.Role, identity(u)
		s.ID, s.Role, s.Name, s.Email = u.ID, u.Role, u.Name, u.Email
		if u.Role == "ADMIN" {
			if c, e := r.Cookie(ViewAsCookie); e == nil && c.Value != "" && c.Value != u.ID {
				if target, e := repo.FindByID(r.Context(), c.Value); e == nil {
					s.ID, s.Role, s.Name, s.Email = target.ID, target.Role, target.Name, target.Email
				}
			}
		}
		next.ServeHTTP(w, r.WithContext(WithSession(r.Context(), s)))
	})
}

type sessionKey struct{}

func WithSession(ctx context.Context, s *SessionUser) context.Context {
	return context.WithValue(ctx, sessionKey{}, s)
}
func SessionFromContext(ctx context.Context) (*SessionUser, bool) {
	s, ok := ctx.Value(sessionKey{}).(*SessionUser)
	return s, ok
}
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, ok := SessionFromContext(r.Context())
		if !ok || s.RealRole != "ADMIN" {
			failure(w, 403, "Forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}
