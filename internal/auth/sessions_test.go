package auth

import (
	"github.com/golang-jwt/jwt/v5"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionLifetimeAndVerification(t *testing.T) {
	token, e := SignSession(SessionUser{ID: "alice"}, "test-secret")
	if e != nil {
		t.Fatal(e)
	}
	s, e := VerifySession(token, "test-secret")
	if e != nil || s.ID != "alice" {
		t.Fatalf("session: %v %v", s, e)
	}
	var claims SessionClaims
	_, _, e = jwt.NewParser().ParseUnverified(token, &claims)
	if e != nil {
		t.Fatal(e)
	}
	if d := claims.ExpiresAt.Sub(claims.IssuedAt.Time); d != 30*24*time.Hour {
		t.Fatalf("token lifetime: %v", d)
	}
	if _, e = VerifySession(token, "wrong-secret"); e == nil {
		t.Fatal("accepted wrong secret")
	}
	expired := jwt.NewWithClaims(jwt.SigningMethodHS256, SessionClaims{UserID: "alice", RegisteredClaims: jwt.RegisteredClaims{Issuer: "computer", ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour))}})
	v, _ := expired.SignedString([]byte("test-secret"))
	if _, e = VerifySession(v, "test-secret"); e == nil {
		t.Fatal("accepted expired token")
	}
	token256 := jwt.NewWithClaims(jwt.SigningMethodHS384, SessionClaims{UserID: "alice", RegisteredClaims: jwt.RegisteredClaims{Issuer: "computer", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}})
	v, _ = token256.SignedString([]byte("test-secret"))
	if _, e = VerifySession(v, "test-secret"); e == nil {
		t.Fatal("accepted unexpected algorithm")
	}
}

func signTestToken(t *testing.T, typ string, ttl time.Duration, ver int) string {
	t.Helper()
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, SessionClaims{
		UserID: "alice", Type: typ, Ver: ver,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "computer",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	})
	v, e := tok.SignedString([]byte("test-secret"))
	if e != nil {
		t.Fatal(e)
	}
	return v
}

// The Bearer channel accepts only short-lived assertions carrying a token
// version (review SEC-02); the cookie channel accepts only session tokens (or
// legacy tokens issued before typ existed). A 30-day cookie JWT must never
// work as a Bearer credential.
func TestTokenChannelBinding(t *testing.T) {
	assertion := signTestToken(t, TokenTypeAssertion, 60*time.Second, 3)
	versionlessAssertion := signTestToken(t, TokenTypeAssertion, 60*time.Second, 0)
	longAssertion := signTestToken(t, TokenTypeAssertion, time.Hour, 3)
	session, e := SignSession(SessionUser{ID: "alice"}, "test-secret")
	if e != nil {
		t.Fatal(e)
	}
	legacy := signTestToken(t, "", time.Hour, 0)

	bearer := func(token string) (*SessionUser, error) {
		r := httptest.NewRequest("GET", "/api/me", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		return ReadSession(r, "test-secret")
	}
	cookie := func(token string) (*SessionUser, error) {
		r := httptest.NewRequest("GET", "/api/me", nil)
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
		return ReadSession(r, "test-secret")
	}

	if s, e := bearer(assertion); e != nil || s.ID != "alice" || s.Ver != 3 {
		t.Fatalf("valid assertion rejected or lost its version: %v %v", s, e)
	}
	if _, e := bearer(versionlessAssertion); e == nil {
		t.Fatal("assertion without token version accepted (revocation bypass)")
	}
	if _, e := bearer(session); e == nil {
		t.Fatal("30-day session token accepted as Bearer assertion")
	}
	if _, e := bearer(longAssertion); e == nil {
		t.Fatal("over-long assertion accepted (exp-iat cap not enforced)")
	}
	if _, e := bearer(legacy); e == nil {
		t.Fatal("typ-less token accepted as Bearer assertion")
	}
	if s, e := cookie(session); e != nil || s.ID != "alice" {
		t.Fatalf("valid session cookie rejected: %v %v", s, e)
	}
	if s, e := cookie(legacy); e != nil || s.ID != "alice" {
		t.Fatalf("legacy typ-less cookie rejected: %v %v", s, e)
	}
	if _, e := cookie(assertion); e == nil {
		t.Fatal("assertion accepted as cookie session")
	}
}
