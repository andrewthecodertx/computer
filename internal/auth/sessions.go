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
)

// SessionClaims is the JWT payload.
type SessionClaims struct {
	UserID string `json:"uid"`
	Role   string `json:"role"`
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
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(sessionMaxAge)),
			Issuer:    "computer",
		},
	})
	return tok.SignedString([]byte(secret))
}

// VerifySession decodes a JWT cookie value into the session user.
func VerifySession(token, secret string) (*SessionUser, error) {
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
	return &SessionUser{ID: claims.UserID, Role: claims.Role}, nil
}

// ReadSession pulls the session cookie from the request and verifies it.
func ReadSession(r *http.Request, secret string) (*SessionUser, error) {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return VerifySession(strings.TrimPrefix(h, "Bearer "), secret)
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, err
	}
	return VerifySession(c.Value, secret)
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
