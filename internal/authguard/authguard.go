package authguard

import "github.com/andrew/go-computer/internal/models"

// Porting guide 3.2.
//
// "Session user" = the authenticated person; "effective user" = the account a
// request acts on. These differ only while an admin uses View as.

// VIEW_AS_COOKIE is the cookie holding the target user ID.
const VIEW_AS_COOKIE = "computer_view_as"

// AuthUser is the resolved identity.
type AuthUser struct {
	ID            string
	Name          *string
	Email         *string
	Image         *string
	Role          string // USER | ADMIN
	ActingAdminID *string // set when an admin is viewing as someone else
}

// SessionUser is the identity from the signed session cookie, before View as.
// Defined here to avoid an import cycle with internal/auth.
type SessionUser struct {
	ID    string
	Name  *string
	Email *string
	Image *string
	Role  string
}

// adminEmails splits env ADMIN_EMAILS on commas, trims, lowercases.
func adminEmails() []string { return nil }

// ResolveRole returns ADMIN if the DB role is ADMIN; otherwise if the email is
// in adminEmails(), it WRITES role ADMIN to the DB and returns ADMIN.
func ResolveRole(user *models.User) string { return user.Role }

// GetRealUser returns the session user, ignoring View as. Used by admin
// endpoints and /api/me.
func GetRealUser(session *SessionUser) *AuthUser { return nil }

// GetAuthUser returns the effective user. If the real user is ADMIN and the
// cookie holds another existing user's ID, returns that user with
// ActingAdminID set; otherwise returns the real user. All data endpoints use
// this.
func GetAuthUser(session *SessionUser, viewAsCookie string) *AuthUser { return nil }