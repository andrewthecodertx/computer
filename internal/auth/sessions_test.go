package auth

import (
	"github.com/golang-jwt/jwt/v5"
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
