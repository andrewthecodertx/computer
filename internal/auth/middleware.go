package auth

import (
	"context"
	"errors"
	"github.com/andrew/go-computer/internal/config"
	"github.com/andrew/go-computer/internal/db"
	"github.com/andrew/go-computer/internal/models"
	"log"
	"net/http"
)

const ViewAsCookie = "computer_view_as"

type SessionUser struct {
	ID, Role, RealID, RealRole string
	Name, Email                *string
	RealUser                   map[string]any
	// Ver is the tokenVersion embedded in the credential (0 = no claim:
	// pre-revocation legacy cookies only — assertions must carry it), checked
	// against the user row so signout revokes cookie sessions AND NextAuth
	// assertions alike (review SEC-02).
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
			// Review CORR-03: a missing user is a credential problem (401),
			// but backend degradation must not masquerade as one — surface it
			// as 503 so client retry logic and monitoring see the difference.
			if errors.Is(e, db.ErrNotFound) {
				failure(w, 401, "Unauthorized")
			} else {
				log.Printf("auth: user lookup failed for session %s: %v", s.ID, e)
				failure(w, 503, "Service unavailable")
			}
			return
		}
		// Revoked session: the credential's tokenVersion predates a signout
		// bump. Applies to cookie sessions and NextAuth assertions alike.
		if s.Ver > 0 && s.Ver != u.TokenVersion {
			failure(w, 401, "Unauthorized")
			return
		}
		// Promotion via ADMIN_EMAILS is persistent by design: removing an
		// address from the env var later does NOT demote the account (use the
		// Admin screen). Silent demotion on env drift would be worse than an
		// extra explicit revocation step.
		//
		// Code review SEC-04 flagged that this re-promotes an account demoted
		// via the Admin screen while its address remains in ADMIN_EMAILS. The
		// env-driven promotion is a deliberate architectural decision here —
		// demoting an operator account means editing ADMIN_EMAILS too; do not
		// "fix" this silently.
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
