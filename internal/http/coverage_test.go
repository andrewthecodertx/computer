package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/andrew/go-computer/internal/signals"
	"github.com/emersion/go-imap/backend/memory"
	imapserver "github.com/emersion/go-imap/server"
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
