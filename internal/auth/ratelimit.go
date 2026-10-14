// Abuse protection for the unauthenticated auth endpoints: a fixed-window
// counter per IP plus consecutive-failure lockout per (IP, email) on login.
//
// Review SEC-1: the login endpoint and its failure lockout apply to EVERY
// peer, including loopback. In same-host non-Docker deployments the Next
// server reaches Go over localhost, and exempting that path removed all
// throttling from real browser logins (unlimited online password guessing).
// Signup keeps the loopback exemption: it exists for same-host consumers of
// the 127.0.0.1 binding, and a forwarded identity never earns it (SEC-2).
//
// Review AUTH-RATE: keying strictly on the direct peer made every browser
// behind the Next proxy share one bucket (in Compose, Go's peer is always
// the web container). The trusted Next server therefore forwards the browser
// IP it observed, signed with HMAC-SHA256(secret, ip|unix_ts) under the
// shared auth secret (web/lib/api.ts, web/auth.ts), with a 60s replay
// window. Go only accepts the forwarded identity when the signature and
// timestamp verify, so hostile clients cannot forge it; unsigned or invalid
// values fall back to the direct peer. Next forwards only X-Real-IP (never
// X-Forwarded-For) and only when TRUSTED_PROXY=1 pins the proxy that
// overwrites that header — the shipped nginx template does.

package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"github.com/andrew/go-computer/internal/config"
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

const (
	loginIPLimit      = 20          // logins per IP per window
	loginIPWindow     = time.Minute //
	signupIPLimit     = 10          // signups per IP per window
	signupIPWindow    = time.Hour   //
	failureThreshold  = 5           // consecutive failures before lockout
	failureLockout    = 15 * time.Minute
	failureWindow     = time.Hour // failures older than this are forgotten

	// limiterMaxEntries is the soft cap per store. Review SEC-03: the failure
	// tracker previously never evicted sub-threshold entries (zero until), so
	// cycling unique emails grew the map without bound. Every mutator now
	// sweeps once the cap is exceeded.
	limiterMaxEntries = 4096
)

type counter struct {
	count int
	reset time.Time
	until time.Time // lockout expiry (failure tracker only)
}

type limiterStore struct {
	mu sync.Mutex
	m  map[string]*counter
}

func newLimiterStore() *limiterStore { return &limiterStore{m: map[string]*counter{}} }

// sweep bounds memory (review SEC-03). Callers hold s.mu. Pass 1 drops
// entries whose window — and lockout, for the failure tracker — have expired.
// If a burst of still-live keys (e.g. unique-email login failures inside the
// failure window) keeps the store oversized, pass 2 evicts the oldest-tracked
// non-locked entries until the cap holds again; active lockouts are left
// alone so an eviction can never shorten a lockout.
func (s *limiterStore) sweep(now time.Time) {
	if len(s.m) <= limiterMaxEntries {
		return
	}
	for k, c := range s.m {
		if now.After(c.reset) && (c.until.IsZero() || now.After(c.until)) {
			delete(s.m, k)
		}
	}
	if len(s.m) <= limiterMaxEntries {
		return
	}
	keys := make([]string, 0, len(s.m))
	for k, c := range s.m {
		if c.until.IsZero() || now.After(c.until) {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return s.m[keys[i]].reset.Before(s.m[keys[j]].reset) })
	for _, k := range keys[:min(len(keys), len(s.m)-limiterMaxEntries)] {
		delete(s.m, k)
	}
}

// allow reports whether key is within limit for the window starting at first
// use. Expiry returns the remaining time until the window resets so callers
// can send an accurate Retry-After header (review ERR-3).
func (s *limiterStore) allow(key string, limit int, window time.Duration, now time.Time) (bool, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(now)
	c := s.m[key]
	if c == nil || now.After(c.reset) {
		s.m[key] = &counter{count: 1, reset: now.Add(window)}
		return true, 0
	}
	c.count++
	retry := c.reset.Sub(now)
	if retry < 0 {
		retry = 0
	}
	return c.count <= limit, retry.Truncate(time.Second) + time.Second
}

func (s *limiterStore) locked(key string, now time.Time) (time.Duration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(now)
	c := s.m[key]
	if c == nil || c.until.IsZero() {
		return 0, false
	}
	if now.After(c.until) {
		delete(s.m, key)
		return 0, false
	}
	// Consistent clock (review SEC-03): derive the retry delay from the
	// supplied now rather than mixing in time.Until.
	return c.until.Sub(now).Truncate(time.Second) + time.Second, true
}

func (s *limiterStore) noteFailure(key string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(now)
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

// ResetRateLimits clears all limiter state. Tests only: the stores are
// process-global, and integration harnesses reset them per test server so
// sequential tests do not inherit each other's buckets (the login endpoint
// has no loopback exemption since review SEC-1).
func ResetRateLimits() {
	for _, s := range []*limiterStore{loginIPs, signupIPs, loginFailures} {
		s.mu.Lock()
		s.m = map[string]*counter{}
		s.mu.Unlock()
	}
}

// resetRateLimits clears all limiter state. Tests only.
func resetRateLimits() { ResetRateLimits() }

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Headers carrying the proxy-attested browser IP (review AUTH-RATE). The
// signature is HMAC-SHA256(secret, ip|unix_ts) in hex, computed by the
// trusted Next server; anything else falls back to the direct peer address.
// The timestamp bounds replay of a captured header pair to ~60s (review
// SEC-2). Next only forwards an IP when TRUSTED_PROXY=1 pins the boundary
// that overwrites X-Real-IP (the shipped nginx template does).
const (
	headerClientIP  = "X-Computer-Client-Ip"
	headerClientTs  = "X-Computer-Client-Ts"
	headerClientSig = "X-Computer-Client-Sig"
	// forwardSkew bounds clock skew and replay of the signed identity.
	forwardSkew = 60 * time.Second
)

func SignClientIP(secret, ip string, ts int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ip + "|" + strconv.FormatInt(ts, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

func signedForwardIP(r *http.Request) (string, bool) {
	ip := r.Header.Get(headerClientIP)
	sig := r.Header.Get(headerClientSig)
	tsRaw := r.Header.Get(headerClientTs)
	if ip == "" || sig == "" || tsRaw == "" || net.ParseIP(ip) == nil {
		return "", false
	}
	ts, err := strconv.ParseInt(tsRaw, 10, 64)
	if err != nil {
		return "", false
	}
	now := time.Now().Unix()
	if ts > now+int64(forwardSkew/time.Second) || ts < now-int64(forwardSkew/time.Second) {
		return "", false
	}
	secret := config.Load().Secret()
	if secret == "" {
		return "", false
	}
	if !hmac.Equal([]byte(SignClientIP(secret, ip, ts)), []byte(sig)) {
		return "", false
	}
	return ip, true
}

// EffectiveClientIP is the identity rate limiting buckets on: the signed
// forwarded browser IP when the trusted proxy attested one, else the direct
// peer. Never trust arbitrary forwarded headers here.
func EffectiveClientIP(r *http.Request) string {
	if ip, ok := signedForwardIP(r); ok {
		return ip
	}
	return clientIP(r)
}

// isLoopback reports whether the DIRECT peer is localhost. A forwarded
// identity — even a signed loopback one — never earns the exemption (review
// SEC-2): the exemption exists for same-host consumers of the 127.0.0.1
// binding, not for anything arriving through a proxy. Plain X-Forwarded-For
// is client-controlled and never trusted.
func isLoopback(r *http.Request) bool {
	if _, forwarded := signedForwardIP(r); forwarded {
		return false
	}
	ip := net.ParseIP(clientIP(r))
	return ip != nil && ip.IsLoopback()
}

func tooManyRequests(w http.ResponseWriter, retry time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())))
	failure(w, http.StatusTooManyRequests, "Too many attempts, slow down")
}

// RateLimitLogin caps login attempts per effective client IP for every peer
// (review SEC-1: loopback is NOT exempt on the login endpoint).
func RateLimitLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Review ERR-3: Retry-After carries the remaining window time, not
		// the full window.
		if ok, retry := loginIPs.allow(EffectiveClientIP(r), loginIPLimit, loginIPWindow, time.Now()); !ok {
			tooManyRequests(w, retry)
			return
		}
		next(w, r)
	}
}

// RateLimitSignup caps account creation per effective client IP for
// non-loopback peers.
func RateLimitSignup(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLoopback(r) {
			if ok, retry := signupIPs.allow(EffectiveClientIP(r), signupIPLimit, signupIPWindow, time.Now()); !ok {
				tooManyRequests(w, retry)
				return
			}
		}
		next(w, r)
	}
}

// NewLimiterStore creates a new in-memory rate limiter store. Exported so
// callers (e.g. public endpoints) can have independent buckets.
func NewLimiterStore() *limiterStore { return newLimiterStore() }

// Limiter wraps a handler with a simple per-IP fixed-window rate limit.
// Uses the direct peer IP (no forwarded headers) and does not exempt
// loopback — public endpoints apply to every peer.
func Limiter(s *limiterStore, limit int, window time.Duration) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			if ok, retry := s.allow(ip, limit, window, time.Now()); !ok {
				tooManyRequests(w, retry)
				return
			}
			next(w, r)
		}
	}
}
