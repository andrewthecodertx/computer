package auth

import "github.com/andrew/go-computer/internal/config"

// Session strategy: signed JWT cookie (porting guide 3.1).
//
// The jwt callback stores token.id = user.id and token.provider;
// the session callback copies token.id to session.user.id.
//
// Login page: /login.
// Sign-in providers:
//   - credentials: authorize(email, password) — find user by email; return null
//     if missing, no password, or bcrypt compare fails; otherwise return
//     {id, name, email, image}.
//   - authelia (OIDC): registered only when getOidcStatus().enabled. Scope
//     openid profile email. Profile mapping: id=sub, name = name ??
//     preferred_username ?? sub, email, image = picture. Links to an existing
//     account with the same email.
//
// Secrets: NEXTAUTH_SECRET / AUTH_SECRET (same value) sign the cookie.

// SigninRequest is the credentials login payload.
type SigninRequest struct {
	Email    string
	Password string
}

// SignupRequest is the signup payload (4.1).
type SignupRequest struct {
	Email    string
	Password string
	Name     *string
}

// SignupResponse is returned on successful signup.
type SignupResponse struct {
	OK     bool
	UserID string
}

// Authorize verifies credentials and returns the user identity, or nil.
// TODO: bcrypt(12) compare; 401 on failure.
func Authorize(req SigninRequest, db interface{}) (*UserIdentity, error) {
	return nil, nil
}

// CreateSignup creates a user. 400 if missing; 409 if email exists; role ADMIN
// if no admin exists yet (ignoring @example.com test users). Name defaults to
// the part of the email before @. Returns 201 {ok, userId}.
func CreateSignup(req SignupRequest, db interface{}) (*SignupResponse, error) {
	return nil, nil
}

// UserIdentity is what the session stores.
type UserIdentity struct {
	ID     string
	Name   *string
	Email  *string
	Image  *string
	Role   string
}

// SessionCookie signs and verifies the JWT session cookie.
// TODO: implement HS256 with the config secret.
func SignSession(u UserIdentity, secret string) (string, error) { return "", nil }
func VerifySession(token, secret string) (*UserIdentity, error)  { return nil, nil }

// OidcStatus returns whether the Authelia provider is enabled.
// TODO: reuse the placeholder check from internal/oidc.
func OidcStatusEnabled(cfg *config.Config) OidcStatus { return OidcStatus{} }

// OidcStatus mirrors lib/oidc-config.ts. Enabled only when all three env values
// are real (non-placeholder). Never exposes the secret.
type OidcStatus struct {
	Enabled         bool
	IssuerSet       bool
	ClientIDSet     bool
	ClientSecretSet bool
	Issuer          *string
}

// Callback path for Authelia: /api/auth/callback/authelia.
const CallbackPath = "/api/auth/callback/authelia"