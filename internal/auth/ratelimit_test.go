package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLimiterStoreWindow(t *testing.T) {
	s := newLimiterStore()
	now := time.Now()
	for i := 0; i < 5; i++ {
		if !s.allow("ip", 5, time.Minute, now.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("request %d within limit rejected", i+1)
		}
	}
	if s.allow("ip", 5, time.Minute, now.Add(10*time.Second)) {
		t.Fatal("request over limit accepted")
	}
	if !s.allow("ip", 5, time.Minute, now.Add(61*time.Second)) {
		t.Fatal("request after window reset rejected")
	}
	if !s.allow("other", 5, time.Minute, now) {
		t.Fatal("separate key shares the limit")
	}
}

func TestFailureLockout(t *testing.T) {
	s := newLimiterStore()
	now := time.Now()
	for i := 0; i < failureThreshold-1; i++ {
		s.noteFailure("ip|user", now.Add(time.Duration(i)*time.Second))
	}
	if _, locked := s.locked("ip|user", now.Add(time.Minute)); locked {
		t.Fatal("locked below threshold")
	}
	s.noteFailure("ip|user", now.Add(time.Minute))
	retry, locked := s.locked("ip|user", now.Add(2*time.Minute))
	if !locked || retry <= 0 {
		t.Fatalf("not locked after %d failures: %v %v", failureThreshold, retry, locked)
	}
	s.noteSuccess("ip|user")
	if _, locked := s.locked("ip|user", now.Add(2*time.Minute)); locked {
		t.Fatal("successful login did not clear the lockout")
	}
	if _, locked := s.locked("ip|user", now.Add(failureLockout+time.Hour)); locked {
		t.Fatal("lockout did not expire")
	}
}

func dummyHandler(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }

func TestRateLimitMiddleware(t *testing.T) {
	resetRateLimits()
	t.Cleanup(resetRateLimits)

	// Loopback peers are exempt (host-local direct API consumers, tests).
	loop := httptest.NewRequest("POST", "/api/auth/login", nil) // RemoteAddr 192.0.2.1
	loop.RemoteAddr = "127.0.0.1:54321"
	for i := 0; i < loginIPLimit*2; i++ {
		w := httptest.NewRecorder()
		RateLimitLogin(dummyHandler)(w, loop)
		if w.Code != 204 {
			t.Fatalf("loopback request %d limited with %d", i+1, w.Code)
		}
	}

	// Non-loopback peers get 429 after the window limit, with Retry-After.
	remote := httptest.NewRequest("POST", "/api/auth/login", nil)
	remote.RemoteAddr = "203.0.113.7:1234"
	var last *httptest.ResponseRecorder
	for i := 0; i < loginIPLimit+1; i++ {
		last = httptest.NewRecorder()
		RateLimitLogin(dummyHandler)(last, remote)
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after limit, got %d", last.Code)
	}
	if last.Header().Get("Retry-After") == "" {
		t.Fatal("429 without Retry-After header")
	}

	// Signup limiter counts independently.
	resetRateLimits()
	var signupLast *httptest.ResponseRecorder
	for i := 0; i < signupIPLimit+1; i++ {
		signupLast = httptest.NewRecorder()
		RateLimitSignup(dummyHandler)(signupLast, remote)
	}
	if signupLast.Code != http.StatusTooManyRequests {
		t.Fatalf("signup: expected 429 after limit, got %d", signupLast.Code)
	}
	w := httptest.NewRecorder()
	RateLimitLogin(dummyHandler)(w, remote)
	if w.Code != 204 {
		t.Fatalf("signup exhaustion must not limit login: %d", w.Code)
	}
}
