package auth

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// SessionStrategy mirrors the original NextAuth setup: a stateless signed JWT
// cookie. The jwt callback stores token.id = user.id and token.provider; the
// session callback copies token.id to session.user.id (porting guide 3.1).
const (
	sessionCookieName = "computer_session"
	// time.Duration is nanoseconds, so multiply by time.Second explicitly.
	sessionMaxAge = 30 * 24 * 60 * 60 * time.Second // 30 days

	// Token types bind each token to its channel: the trusted Next server
	// signs short-lived "assertion" tokens for the Bearer header; Go signs
	// 30-day "session" tokens for its own cookie. Neither is accepted on the
	// other channel, so a long-lived cookie JWT can never be replayed as a
	// Bearer credential (and vice versa).
	TokenTypeSession   = "session"
	TokenTypeAssertion = "assertion"
	// maxAssertionLifetime is the server-side cap for Bearer assertions; the
	// Next server signs 60-second tokens.
	maxAssertionLifetime = 120 * time.Second
)

// SessionClaims is the JWT payload.
type SessionClaims struct {
	UserID string `json:"uid"`
	Role   string `json:"role"`
	Type   string `json:"typ,omitempty"`
	jwt.RegisteredClaims
}

// SignSession issues a signed JWT cookie value for the user.
func SignSession(u SessionUser, secret string) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("auth: empty session secret")
	}
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, SessionClaims{
		UserID: u.ID,
		Role:   u.Role,
		Type:   TokenTypeSession,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(sessionMaxAge)),
			Issuer:    "computer",
		},
	})
	return tok.SignedString([]byte(secret))
}

// VerifySession decodes a JWT cookie value into the session user. It checks
// signature, algorithm, issuer and expiry — but NOT the channel binding; use
// ReadSession for request handling.
func VerifySession(token, secret string) (*SessionUser, error) {
	claims, err := verifyClaims(token, secret)
	if err != nil {
		return nil, err
	}
	return &SessionUser{ID: claims.UserID, Role: claims.Role}, nil
}

func verifyClaims(token, secret string) (*SessionClaims, error) {
	if secret == "" {
		return nil, fmt.Errorf("auth: empty session secret")
	}
	tok, err := jwt.ParseWithClaims(token, &SessionClaims{}, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("computer"), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*SessionClaims)
	if !ok || !tok.Valid || claims.UserID == "" {
		return nil, errors.New("auth: invalid token")
	}
	return claims, nil
}

// ReadSession pulls the session credential from the request and verifies it,
// enforcing the channel binding: Bearer requires a short-lived assertion; the
// cookie requires a session token (tokens issued before typ existed, i.e. no
// typ claim, remain valid on the cookie channel only).
func ReadSession(r *http.Request, secret string) (*SessionUser, error) {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		claims, err := verifyClaims(strings.TrimPrefix(h, "Bearer "), secret)
		if err != nil {
			return nil, err
		}
		if claims.Type != TokenTypeAssertion {
			return nil, errors.New("auth: bearer token is not an assertion")
		}
		if claims.IssuedAt == nil || claims.ExpiresAt == nil || claims.ExpiresAt.Sub(claims.IssuedAt.Time) > maxAssertionLifetime {
			return nil, errors.New("auth: assertion lifetime too long")
		}
		return &SessionUser{ID: claims.UserID, Role: claims.Role}, nil
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, err
	}
	claims, err := verifyClaims(c.Value, secret)
	if err != nil {
		return nil, err
	}
	if claims.Type != "" && claims.Type != TokenTypeSession {
		return nil, errors.New("auth: cookie token has wrong type")
	}
	return &SessionUser{ID: claims.UserID, Role: claims.Role}, nil
}

// WriteSessionCookie sets the signed session cookie on the response.
func WriteSessionCookie(w http.ResponseWriter, u SessionUser, secret string) error {
	tok, err := SignSession(u, secret)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    tok,
		Path:     "/",
		MaxAge:   int(sessionMaxAge.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure(),
	})
	return nil
}

// ClearSessionCookie expires the session cookie.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure(),
	})
}

func isSecure() bool {
	return os.Getenv("COOKIE_SECURE") == "1"
}

// ErrUnauthorized is returned when there is no valid session.
var ErrUnauthorized = errors.New("auth: unauthorized")
