package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/andrew/go-computer/internal/auth"
	"github.com/andrew/go-computer/internal/signals"
	"github.com/emersion/go-imap/backend/memory"
	imapserver "github.com/emersion/go-imap/server"
	"github.com/golang-jwt/jwt/v5"
)

// Covers routes that previously had no coverage at any level (CODE_REVIEW
// TEST-1) plus signout revocation (AUTH-5) and the background scheduler
// (TEST-2).

func (a apiClient) getJSON(path string, status int) []map[string]any {
	a.t.Helper()
	res, e := a.c.Get(a.base + path)
	if e != nil {
		a.t.Fatal(e)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != status {
		a.t.Fatalf("GET %s: wanted %d, got %d: %s", path, status, res.StatusCode, raw)
	}
	var v []map[string]any
	if e := json.Unmarshal(raw, &v); e != nil {
		a.t.Fatalf("GET %s: not a JSON array: %v: %s", path, e, raw)
	}
	return v
}

func (a apiClient) getText(path string, status int) string {
	a.t.Helper()
	res, e := a.c.Get(a.base + path)
	if e != nil {
		a.t.Fatal(e)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != status {
		a.t.Fatalf("GET %s: wanted %d, got %d: %s", path, status, res.StatusCode, raw)
	}
	return string(raw)
}

func newAPIUser(t *testing.T, srv *httptest.Server, email string) apiClient {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := apiClient{t, &http.Client{Jar: jar}, srv.URL}
	creds := map[string]any{"email": email, "password": "coverage-test-password"}
	c.request("POST", "/api/signup", creds, 201)
	c.request("POST", "/api/auth/login", creds, 200)
	return c
}

func TestPreviouslyUncoveredRoutes(t *testing.T) {
	srv := testServer(t)
	alice := newAPIUser(t, srv, "coverage-alice@computer.test")
	newAPIUser(t, srv, "coverage-searchable@computer.test")

	// GET /api/preview — handler plumbing (auth + query + parse).
	html := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<html><head><title>Preview Target</title><meta name="description" content="Described"></head><body>x</body></html>`)
	}))
	defer html.Close()
	anon, _ := cookiejar.New(nil)
	anonymous := apiClient{t, &http.Client{Jar: anon}, srv.URL}
	anonymous.request("GET", "/api/preview?url="+html.URL, nil, 401)
	if v := alice.request("GET", "/api/preview?url="+html.URL, nil, 200); v["title"] != "Preview Target" || v["description"] != "Described" {
		t.Fatal("preview handler:", v)
	}

	// GET /api/bookmarks/{id}/markdown — content, not just the link element.
	b := alice.request("POST", "/api/bookmarks", map[string]any{"url": "https://example.org/coverage", "title": "Coverage Bookmark", "notes": "# Coverage notes"}, 201)
	id := b["id"].(string)
	md := alice.getText("/api/bookmarks/"+id+"/markdown", 200)
	if !strings.Contains(md, "https://example.org/coverage") || !strings.Contains(md, "Coverage notes") {
		t.Fatal("bookmark markdown:", md)
	}

	// GET /api/users/search — name substring, exact email, masked output,
	// no email-substring enumeration, 2-rune guard.
	users := alice.getJSON("/api/users/search?q=coverage-sea", 200)
	if len(users) != 1 || users[0]["email"] != "c***@computer.test" {
		t.Fatal("users/search name match:", users)
	}
	if users := alice.getJSON("/api/users/search?q=coverage-searchable@computer.test", 200); len(users) != 1 {
		t.Fatal("users/search exact email:", users)
	}
	// "able@computer.test" is a substring of the target's email but not a
	// full email and not part of any name: must not match.
	if users := alice.getJSON("/api/users/search?q=able%40computer.test", 200); len(users) != 0 {
		t.Fatal("users/search must not substring-match emails:", users)
	}
	if users := alice.getJSON("/api/users/search?q=c", 200); len(users) != 0 {
		t.Fatal("users/search short-query guard:", users)
	}
	// Review SEC-3: LIKE wildcards are escaped, so "%%"/"_%" cannot enumerate
	// the whole user base. "%%" matches a literal percent-pair (2 runes, so
	// it passes the minimum-length guard); a name literally containing "%%"
	// still matches, but nothing else does.
	if users := alice.getJSON("/api/users/search?q=%25%25", 200); len(users) != 0 {
		t.Fatalf("users/search wildcard enumeration: %v", users)
	}

	// PATCH /api/kanban/columns/{id} and PUT /api/kanban/columns happy paths.
	col := alice.request("POST", "/api/kanban/columns", map[string]any{"label": "Coverage"}, 201)["id"].(string)
	alice.request("PATCH", "/api/kanban/columns/"+col, map[string]any{"label": "Renamed", "color": "#00ff00"}, 200)
	cols := alice.getJSON("/api/kanban/columns", 200)
	var found bool
	order := make([]string, 0, len(cols))
	for _, c := range cols {
		order = append(order, c["id"].(string))
		if c["id"] == col {
			found = c["label"] == "Renamed" && c["color"] == "#00ff00"
		}
	}
	if !found {
		t.Fatal("column patch not applied:", cols)
	}
	reversed := make([]string, 0, len(order))
	for i := len(order) - 1; i >= 0; i-- {
		reversed = append(reversed, order[i])
	}
	alice.request("PUT", "/api/kanban/columns", map[string]any{"order": reversed}, 200)
	after := alice.getJSON("/api/kanban/columns", 200)
	if after[0]["id"] != reversed[0] || after[len(after)-1]["id"] != reversed[len(reversed)-1] {
		t.Fatal("column reorder not applied")
	}

	// PUT /api/pages reorder.
	p1 := alice.request("POST", "/api/pages", map[string]any{"title": "P1"}, 201)["id"].(string)
	p2 := alice.request("POST", "/api/pages", map[string]any{"title": "P2"}, 201)["id"].(string)
	p3 := alice.request("POST", "/api/pages", map[string]any{"title": "P3"}, 201)["id"].(string)
	alice.request("PUT", "/api/pages", map[string]any{"order": []string{p3, p1, p2}}, 200)
	pages := alice.getJSON("/api/pages", 200)
	if pages[0]["id"] != p3 || pages[1]["id"] != p1 || pages[2]["id"] != p2 {
		t.Fatal("pages reorder not applied:", pages)
	}

	// DELETE /api/tags/{id} owner success + BookmarkTag cascade.
	tag := alice.request("POST", "/api/tags", map[string]any{"name": "DeleteMe"}, 201)["id"].(string)
	alice.request("PUT", "/api/bookmarks/"+id, map[string]any{"tagIds": []string{tag}}, 200)
	alice.request("DELETE", "/api/tags/"+tag, nil, 200)
	if v := alice.request("GET", "/api/bookmarks/"+id, nil, 200); len(v["tags"].([]any)) != 0 {
		t.Fatal("tag delete did not cascade bookmark_tags")
	}
	for _, tg := range alice.getJSON("/api/tags", 200) {
		if tg["id"] == tag {
			t.Fatal("deleted tag still listed")
		}
	}

	// DELETE /api/signal-sources/{id} owner success.
	source := alice.request("POST", "/api/bookmarks/"+id+"/signals", map[string]any{"type": "email", "config": map[string]any{"subject": "x"}}, 201)["id"].(string)
	alice.request("DELETE", "/api/signal-sources/"+source, nil, 200)
	if v := alice.request("GET", "/api/bookmarks/"+id+"/signals", nil, 200); len(v["sources"].([]any)) != 0 {
		t.Fatal("signal source still present after delete")
	}
}

// Signout must revoke every issued cookie JWT, not just clear this browser's
// cookie: a token copied before signout stops working.
func TestSignoutRevokesIssuedTokens(t *testing.T) {
	srv := testServer(t)
	u := newAPIUser(t, srv, "revoke@computer.test")

	creds, _ := json.Marshal(map[string]any{"email": "revoke@computer.test", "password": "coverage-test-password"})
	req, _ := http.NewRequest("POST", srv.URL+"/api/auth/login", bytes.NewReader(creds))
	req.Header.Set("Content-Type", "application/json")
	res, e := u.c.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	var stale string
	for _, c := range res.Cookies() {
		if c.Name == "computer_session" {
			stale = c.Value
		}
	}
	if stale == "" {
		t.Fatal("login did not set a session cookie")
	}

	u.request("POST", "/api/auth/signout", map[string]any{}, 200)

	// Replay the pre-signout token directly (no jar): must be rejected.
	replay := func(path string) int {
		rq, _ := http.NewRequest("GET", srv.URL+path, nil)
		rq.AddCookie(&http.Cookie{Name: "computer_session", Value: stale})
		rs, e := u.c.Do(rq)
		if e != nil {
			t.Fatal(e)
		}
		defer rs.Body.Close()
		io.Copy(io.Discard, rs.Body)
		return rs.StatusCode
	}
	if code := replay("/api/me"); code != 401 {
		t.Fatalf("stale cookie accepted on /api/me: %d", code)
	}
	if code := replay("/api/bookmarks"); code != 401 {
		t.Fatalf("stale cookie accepted on data route: %d", code)
	}
	rq, _ := http.NewRequest("GET", srv.URL+"/api/auth/session", nil)
	rq.AddCookie(&http.Cookie{Name: "computer_session", Value: stale})
	rs, e := u.c.Do(rq)
	if e != nil {
		t.Fatal(e)
	}
	defer rs.Body.Close()
	raw, _ := io.ReadAll(rs.Body)
	if !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		t.Fatalf("stale cookie still resolves a session: %s", raw)
	}
}

// One scheduler tick creates signals and marks sources checked (TEST-2).
func TestSignalSchedulerRunOnce(t *testing.T) {
	srv, d := testServerDB(t)
	alice := newAPIUser(t, srv, "scheduler@computer.test")

	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	mail := imapserver.New(memory.New())
	mail.AllowInsecureAuth = true
	go mail.Serve(listener)
	defer mail.Close()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	pn, _ := strconv.Atoi(port)

	alice.request("POST", "/api/settings/imap", map[string]any{"host": host, "port": pn, "tls": false, "username": "username", "password": "password", "folder": "INBOX"}, 200)
	id := alice.request("POST", "/api/bookmarks", map[string]any{"url": "https://example.org/scheduler", "imapQuery": "little"}, 201)["id"].(string)
	alice.request("POST", "/api/bookmarks/"+id+"/signals", map[string]any{"type": "email", "config": map[string]any{"from": "contact@example.org", "subject": "little"}}, 201)

	signals.RunOnce(context.Background(), d)

	v := alice.request("GET", "/api/bookmarks/"+id+"/signals", nil, 200)
	if len(v["signals"].([]any)) == 0 {
		t.Fatal("scheduler tick created no signals")
	}
	sources := v["sources"].([]any)
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}
	src := sources[0].(map[string]any)
	if src["lastStatus"] != "Checked" || src["lastCheckedAt"] == nil {
		t.Fatal("scheduler did not mark source checked:", src)
	}
	// A second tick must not duplicate signals (dedup under the scheduler path).
	before := len(v["signals"].([]any))
	signals.RunOnce(context.Background(), d)
	v = alice.request("GET", "/api/bookmarks/"+id+"/signals", nil, 200)
	if len(v["signals"].([]any)) != before {
		t.Fatal("scheduler tick duplicated signals")
	}
}

// Review CR-16: mailbox failures are scheduler failures. A pass whose mailbox
// cannot even load must record the per-source status, answer manual checks
// with the structured outcome (failed count, not a blanket 500), and drive
// the per-user backoff so the next ticks are skipped.
func TestSchedulerBacksOffAfterMailboxFailure(t *testing.T) {
	srv, d := testServerDB(t)
	alice := newAPIUser(t, srv, "backoff@computer.test")
	id := alice.request("POST", "/api/bookmarks", map[string]any{"url": "https://example.org/backoff"}, 201)["id"].(string)
	alice.request("POST", "/api/bookmarks/"+id+"/signals", map[string]any{"type": "email", "config": map[string]any{"subject": "anything"}}, 201)
	// Deliberately no saved IMAP configuration: every pass fails in LoadMailbox.

	v := alice.request("POST", "/api/bookmarks/"+id+"/signals", map[string]any{"action": "check"}, 200)
	if v["checked"] != float64(1) || v["failed"] != float64(1) {
		t.Fatal("manual check did not report the structured failure outcome:", v)
	}
	src := func() map[string]any {
		got := alice.request("GET", "/api/bookmarks/"+id+"/signals", nil, 200)
		return got["sources"].([]any)[0].(map[string]any)
	}
	if status, _ := src()["lastStatus"].(string); !strings.HasPrefix(status, "Error:") {
		t.Fatal("per-source failure status not recorded:", status)
	}

	s := signals.NewScheduler()
	ctx := context.Background()
	s.RunOnce(ctx, d) // tick 1: runs and fails → failures=1
	s.RunOnce(ctx, d) // tick 2: 2%2==0 → runs and fails → failures=2
	after := src()["lastCheckedAt"]
	s.RunOnce(ctx, d) // tick 3: 3%4!=0 → backoff must skip it
	if got := src()["lastCheckedAt"]; got != after {
		t.Fatalf("backoff did not skip tick 3: lastCheckedAt changed %v → %v", after, got)
	}
}

// Page saves enforce optimistic concurrency (review DATA-01/DATA-05): a stale
// base version is rejected with 409 and a missing version with 400, both
// leaving the row untouched; content saves bump the revision, and metadata
// patches neither require nor bump it. The list projection omits content
// bodies (review PERF-01).
func TestPageVersionAndListProjection(t *testing.T) {
	srv := testServer(t)
	alice := newAPIUser(t, srv, "pagever@computer.test")
	created := alice.request("POST", "/api/pages", map[string]any{"title": "Doc", "content": "first"}, 201)
	id := created["id"].(string)
	if created["version"].(float64) != 1 {
		t.Fatalf("new page did not start at version 1: %v", created["version"])
	}
	alice.request("PATCH", "/api/pages/"+id, map[string]any{"content": "stale tab", "version": 99}, 409)
	if v := alice.request("GET", "/api/pages/"+id, nil, 200); v["content"] != "first" {
		t.Fatal("rejected stale write still modified content")
	}
	saved := alice.request("PATCH", "/api/pages/"+id, map[string]any{"content": "second", "version": 1}, 200)
	if saved["version"].(float64) != 2 {
		t.Fatalf("content save did not bump version: %v", saved["version"])
	}
	// Review DATA-05: omitting the version cannot bypass concurrency — the
	// write is rejected and the row is untouched. Metadata patches stay
	// version-free (they do not bump the revision either).
	alice.request("PATCH", "/api/pages/"+id, map[string]any{"content": "unversioned clobber"}, 400)
	if v := alice.request("GET", "/api/pages/"+id, nil, 200); v["content"] != "second" || v["version"].(float64) != 2 {
		t.Fatalf("version-less content write mutated the page: %v", v)
	}
	pin := alice.request("PATCH", "/api/pages/"+id, map[string]any{"pinned": true}, 200)
	if pin["version"].(float64) != 2 {
		t.Fatalf("metadata patch changed the revision: %v", pin["version"])
	}
	alice.request("PATCH", "/api/pages/"+id, map[string]any{"content": "third", "version": 1}, 409)
	alice.request("PATCH", "/api/pages/"+id, map[string]any{"content": "x", "version": -1}, 400)
	found := false
	for _, p := range alice.getJSON("/api/pages", 200) {
		if p["id"] != id {
			continue
		}
		found = true
		if _, ok := p["content"]; ok {
			t.Fatal("list projection still includes content")
		}
		if p["title"] != "Doc" || p["_count"] == nil {
			t.Fatal("list projection lost metadata:", p)
		}
	}
	if !found {
		t.Fatal("page missing from list")
	}
}

// Review CR-18: a content-save acknowledgement must describe the revision
// THIS write produced. Concurrent writers racing on the same base: exactly
// one wins and its response reports base+1 (the snapshot is read inside the
// write transaction), every loser gets 409 — no acknowledgement can carry a
// competing writer's committed revision.
func TestConcurrentPageSaveAckReportsOwnWrite(t *testing.T) {
	srv := testServer(t)
	alice := newAPIUser(t, srv, "pageack@computer.test")
	id := alice.request("POST", "/api/pages", map[string]any{"title": "Ack", "content": "v1"}, 201)["id"].(string)

	const writers = 8
	type outcome struct {
		status  int
		version float64
	}
	results := make(chan outcome, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			body, _ := json.Marshal(map[string]any{"content": fmt.Sprintf("writer-%d", n), "version": 1})
			rq, _ := http.NewRequest("PATCH", srv.URL+"/api/pages/"+id, bytes.NewReader(body))
			rq.Header.Set("Content-Type", "application/json")
			rs, e := alice.c.Do(rq)
			if e != nil {
				results <- outcome{status: -1}
				return
			}
			defer rs.Body.Close()
			raw, _ := io.ReadAll(rs.Body)
			out := outcome{status: rs.StatusCode}
			var v map[string]any
			if json.Unmarshal(raw, &v) == nil {
				out.version, _ = v["version"].(float64)
			}
			results <- out
		}(i)
	}
	wg.Wait()
	close(results)
	wins := 0
	for o := range results {
		switch o.status {
		case 200:
			wins++
			if o.version != 2 {
				t.Fatalf("winning acknowledgement reported version %v, want exactly base+1 = 2", o.version)
			}
		case 409:
		default:
			t.Fatalf("concurrent writer got unexpected status %d", o.status)
		}
	}
	if wins != 1 {
		t.Fatalf("expected exactly one winning writer, got %d", wins)
	}
}

// Assertions ride the tokenVersion (review SEC-02): "sign out everywhere"
// must revoke NextAuth-backed Bearer assertions, not just Go cookie sessions.
func TestAssertionRevocationFollowsTokenVersion(t *testing.T) {
	srv, d := testServerDB(t)
	u := newAPIUser(t, srv, "assert-revoke@computer.test")
	uid := u.request("GET", "/api/me", nil, 200)["user"].(map[string]any)["id"].(string)
	var ver int
	if e := d.QueryRow(`SELECT "tokenVersion" FROM "User" WHERE id=$1`, uid).Scan(&ver); e != nil {
		t.Fatal(e)
	}
	sign := func(v int) string {
		now := time.Now()
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, auth.SessionClaims{
			UserID: uid, Type: auth.TokenTypeAssertion, Ver: v,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    "computer",
				IssuedAt:  jwt.NewNumericDate(now),
				ExpiresAt: jwt.NewNumericDate(now.Add(60 * time.Second)),
			},
		})
		s, e := tok.SignedString([]byte("integration-test-secret"))
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	bearer := func(token string) int {
		rq, _ := http.NewRequest("GET", srv.URL+"/api/me", nil)
		rq.Header.Set("Authorization", "Bearer "+token)
		rs, e := u.c.Do(rq)
		if e != nil {
			t.Fatal(e)
		}
		defer rs.Body.Close()
		io.Copy(io.Discard, rs.Body)
		return rs.StatusCode
	}
	if code := bearer(sign(ver)); code != 200 {
		t.Fatalf("current-version assertion rejected: %d", code)
	}
	if code := bearer(sign(0)); code != 401 {
		t.Fatalf("version-less assertion accepted: %d", code)
	}
	u.request("POST", "/api/auth/signout", map[string]any{}, 200)
	if code := bearer(sign(ver)); code != 401 {
		t.Fatalf("assertion minted before the revocation bump still accepted: %d", code)
	}
	var bumped int
	if e := d.QueryRow(`SELECT "tokenVersion" FROM "User" WHERE id=$1`, uid).Scan(&bumped); e != nil {
		t.Fatal(e)
	}
	if code := bearer(sign(bumped)); code != 200 {
		t.Fatalf("assertion with the post-bump version rejected: %d", code)
	}
}
