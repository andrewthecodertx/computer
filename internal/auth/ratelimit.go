package auth

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Abuse protection for the unauthenticated auth endpoints: a fixed-window
// counter per IP plus consecutive-failure lockout per (IP, email) on login.
//
// Loopback clients are exempt by design: the Go API binds to 127.0.0.1 for
// direct host consumers and to the internal Docker network for the trusted
// Next proxy. All remote traffic — including anything arriving through the
// web app — reaches Go from a non-loopback address and is limited. An
// operator fronting :8080 with their own same-host proxy accepts the same
// trust they grant localhost today.

const (
	loginIPLimit      = 20             // logins per IP per window
	loginIPWindow     = time.Minute    //
	signupIPLimit     = 10             // signups per IP per window
	signupIPWindow    = time.Hour      //
	failureThreshold  = 5              // consecutive failures before lockout
	failureLockout    = 15 * time.Minute
	failureWindow     = time.Hour // failures older than this are forgotten
)

type counter struct {
	count  int
	reset  time.Time
	until  time.Time // lockout expiry (failure tracker only)
}

type limiterStore struct {
	mu sync.Mutex
	m  map[string]*counter
}

func newLimiterStore() *limiterStore { return &limiterStore{m: map[string]*counter{}} }

// allow reports whether key is within limit for the window starting at first
// use. Expired entries are swept opportunistically to bound memory.
func (s *limiterStore) allow(key string, limit int, window time.Duration, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.m) > 4096 {
		for k, c := range s.m {
			if now.After(c.reset) && c.until.IsZero() {
				delete(s.m, k)
			}
		}
	}
	c := s.m[key]
	if c == nil || now.After(c.reset) {
		s.m[key] = &counter{count: 1, reset: now.Add(window)}
		return true
	}
	c.count++
	return c.count <= limit
}

func (s *limiterStore) locked(key string, now time.Time) (time.Duration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.m[key]
	if c == nil || c.until.IsZero() {
		return 0, false
	}
	if now.After(c.until) {
		delete(s.m, key)
		return 0, false
	}
	return time.Until(c.until).Truncate(time.Second) + time.Second, true
}

func (s *limiterStore) noteFailure(key string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.m[key]
	if c == nil || now.After(c.reset) {
		c = &counter{reset: now.Add(failureWindow)}
		s.m[key] = c
	}
	c.count++
	if c.count >= failureThreshold {
		c.until = now.Add(failureLockout)
	}
}

func (s *limiterStore) noteSuccess(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
}

var (
	loginIPs      = newLimiterStore()
	signupIPs     = newLimiterStore()
	loginFailures = newLimiterStore()
)

// resetRateLimits clears all limiter state. Tests only.
func resetRateLimits() {
	for _, s := range []*limiterStore{loginIPs, signupIPs, loginFailures} {
		s.mu.Lock()
		s.m = map[string]*counter{}
		s.mu.Unlock()
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// isLoopback reports whether the direct peer is localhost. Deliberately based
// on RemoteAddr only — X-Forwarded-For is client-controlled and never trusted.
func isLoopback(r *http.Request) bool {
	ip := net.ParseIP(clientIP(r))
	return ip != nil && ip.IsLoopback()
}

func tooManyRequests(w http.ResponseWriter, retry time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())))
	failure(w, http.StatusTooManyRequests, "Too many attempts, slow down")
}

// RateLimitLogin caps login attempts per IP for non-loopback peers.
func RateLimitLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLoopback(r) && !loginIPs.allow(clientIP(r), loginIPLimit, loginIPWindow, time.Now()) {
			tooManyRequests(w, loginIPWindow)
			return
		}
		next(w, r)
	}
}

// RateLimitSignup caps account creation per IP for non-loopback peers.
func RateLimitSignup(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLoopback(r) && !signupIPs.allow(clientIP(r), signupIPLimit, signupIPWindow, time.Now()) {
			tooManyRequests(w, signupIPWindow)
			return
		}
		next(w, r)
	}
}
