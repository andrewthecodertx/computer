package http

import (
	"bytes"
	"encoding/json"
	"github.com/andrew/go-computer/internal/auth"
	"github.com/andrew/go-computer/internal/config"
	"github.com/andrew/go-computer/internal/db"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

// Integration tests create and drop only their own random PostgreSQL schema.
// They never truncate application tables or delete pre-existing accounts.
func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv, _ := testServerDB(t)
	return srv
}
func testServerDB(t *testing.T) (*httptest.Server, *db.DB) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	// The limiter stores are process-global and login has no loopback
	// exemption (review SEC-1); give every test a clean slate.
	auth.ResetRateLimits()
	t.Cleanup(auth.ResetRateLimits)
	t.Setenv("AUTH_SECRET", "integration-test-secret")
	t.Setenv("NEXTAUTH_SECRET", "integration-test-secret")
	t.Setenv("ADMIN_EMAILS", "")
	// CardDAV/IMAP fakes listen on loopback; netguard blocks private ranges by
	// default (production compose never sets this).
	t.Setenv("INTEGRATIONS_ALLOW_PRIVATE", "1")
	// The IMAP fixture is a plaintext loopback server; production refuses
	// non-TLS mailboxes (review NET-02) and this flag is the test-only escape
	// hatch (see signals.AllowInsecure).
	t.Setenv("INTEGRATIONS_ALLOW_INSECURE_IMAP", "1")
	admin, e := db.New(&config.Config{DatabaseURL: dsn})
	if e != nil {
		t.Fatal(e)
	}
	schema := "test_" + db.NewID()
	if _, e = admin.Exec(`CREATE SCHEMA ` + db.Quote(schema)); e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	d, e := db.New(&config.Config{DatabaseURL: u.String()})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { d.Close(); admin.Exec(`DROP SCHEMA ` + db.Quote(schema) + ` CASCADE`); admin.Close() })
	raw, e := os.ReadFile("../../db/schema.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = d.Exec(string(raw)); e != nil {
		t.Fatal(e)
	}
	srv := httptest.NewServer(NewServer(config.Load(), d).Handler)
	t.Cleanup(srv.Close)
	return srv, d
}

type apiClient struct {
	t    *testing.T
	c    *http.Client
	base string
}

func (a apiClient) request(method, path string, body any, status int) map[string]any {
	a.t.Helper()
	var b io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		b = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, a.base+path, b)
	req.Header.Set("Content-Type", "application/json")
	res, e := a.c.Do(req)
	if e != nil {
		a.t.Fatal(e)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != status {
		a.t.Fatalf("%s %s: wanted %d, got %d: %s", method, path, status, res.StatusCode, raw)
	}
	var v map[string]any
	json.Unmarshal(raw, &v)
	return v
}

// meID resolves the authenticated client's user id. Signup responses no
// longer carry it (review CR-03: identical bodies for every accepted
// registration, so no unauthenticated existence oracle); identity comes from
// the session after login.
func meID(c apiClient) string {
	c.t.Helper()
	return c.request("GET", "/api/me", nil, 200)["user"].(map[string]any)["id"].(string)
}
func TestApplicationFlows(t *testing.T) {
	srv := testServer(t)
	newClient := func() apiClient { jar, _ := cookiejar.New(nil); return apiClient{t, &http.Client{Jar: jar}, srv.URL} }
	alice, bob, anonymous := newClient(), newClient(), newClient()
	post := func(email string) map[string]any {
		return map[string]any{"email": email, "password": "a-long-test-password"}
	}
	alice.request("POST", "/api/signup", post("alice@computer.test"), 201)
	bob.request("POST", "/api/signup", post("bob@computer.test"), 201)
	// Review SEC-4/CR-03: a duplicate signup returns the SAME 201 {"ok":true}
	// body as a fresh creation (no 409, no distinguishing field).
	dup := alice.request("POST", "/api/signup", post("alice@computer.test"), 201)
	if len(dup) != 1 || dup["ok"] != true {
		t.Fatalf("duplicate signup body differs from a fresh creation: %v", dup)
	}
	alice.request("POST", "/api/auth/login", post("alice@computer.test"), 200)
	bob.request("POST", "/api/auth/login", post("bob@computer.test"), 200)
	au := meID(alice)
	bu := meID(bob)
	if alice.request("GET", "/api/me", nil, 200)["isAdmin"] != true {
		t.Fatal("first signup was not admin")
	}
	anonymous.request("GET", "/api/bookmarks", nil, 401)
	bob.request("GET", "/api/admin/users", nil, 403)
	tag := alice.request("POST", "/api/tags", map[string]any{"name": "Research"}, 201)["id"].(string)
	b := alice.request("POST", "/api/bookmarks", map[string]any{"url": "https://127.0.0.1/link", "title": "Original", "notes": "# Notes", "tagIds": []string{tag}, "dueDate": "2026-10-07", "alertAt": "2000-01-01"}, 201)
	id := b["id"].(string)
	bob.request("GET", "/api/bookmarks/"+id, nil, 404)
	bob.request("PUT", "/api/bookmarks/"+id, map[string]any{"notes": "attack"}, 404)
	alice.request("PUT", "/api/bookmarks/"+id, map[string]any{"dueDate": nil, "isPublic": false}, 200)
	if v := alice.request("GET", "/api/bookmarks/"+id, nil, 200); v["title"] != "Original" || v["dueDate"] != nil {
		t.Fatal("partial/null update broken")
	}
	alice.request("PUT", "/api/bookmarks/"+id, map[string]any{"tagIds": []string{"missing"}}, 400)
	v := alice.request("GET", "/api/bookmarks/"+id, nil, 200)
	if len(v["tags"].([]any)) != 1 {
		t.Fatal("failed update lost tags")
	}
	alice.request("POST", "/api/bookmarks/"+id+"/share", map[string]any{"userIds": []string{bu}, "isPublic": true}, 200)
	bob.request("GET", "/api/bookmarks/"+id, nil, 200)
	anonymous.request("GET", "/api/public/bookmarks/"+id, nil, 200)
	bob.request("DELETE", "/api/bookmarks/"+id, nil, 404)
	alice.request("PUT", "/api/tags/"+tag, map[string]any{"shareUserIds": []string{bu}}, 200)
	bob.request("GET", "/api/bookmarks?tagId="+tag, nil, 200)
	page := bob.request("POST", "/api/pages", map[string]any{"title": "Workspace", "content": "Markdown text"}, 201)["id"].(string)
	bob.request("PATCH", "/api/pages/"+page, map[string]any{"addBookmarkId": id}, 200)
	bob.request("PATCH", "/api/pages/"+page, map[string]any{"addBookmarkId": id}, 200)
	v = bob.request("GET", "/api/pages/"+page, nil, 200)
	if len(v["bookmarks"].([]any)) != 1 {
		t.Fatal("duplicate page link")
	}
	bob.request("GET", "/api/pages/"+page+"/markdown", nil, 200)
	alice.request("GET", "/api/kanban/columns", nil, 200)
	col := alice.request("POST", "/api/kanban/columns", map[string]any{"label": "Custom"}, 201)["id"].(string)
	alice.request("PUT", "/api/bookmarks/"+id, map[string]any{"kanbanColumnId": col}, 200)
	v = alice.request("DELETE", "/api/kanban/columns/"+col, nil, 200)
	if v["moved"] != float64(1) {
		t.Fatal("deleted column did not move card")
	}
	alice.request("GET", "/api/bookmarks/"+id, nil, 200)
	alice.request("PUT", "/api/kanban/columns", map[string]any{"order": []string{"not-mine"}}, 400)
	alice.request("GET", "/api/alerts/check", nil, 200)
	alice.request("DELETE", "/api/alerts/"+id, nil, 200)
	v = alice.request("GET", "/api/bookmarks/"+id, nil, 200)
	if v["alertSent"] != true {
		t.Fatal("alert dismissal")
	}
	alice.request("PUT", "/api/bookmarks/"+id, map[string]any{"alertAt": "2000-01-02"}, 200)
	source := alice.request("POST", "/api/bookmarks/"+id+"/signals", map[string]any{"type": "email", "config": map[string]any{"from": "sender@computer.test"}}, 201)["id"].(string)
	alice.request("POST", "/api/bookmarks/"+id+"/signals", map[string]any{"action": "check"}, 200)
	alice.request("PATCH", "/api/signal-sources/"+source, map[string]any{"enabled": false}, 200)
	bob.request("DELETE", "/api/signal-sources/"+source, nil, 404)
	alice.request("POST", "/api/settings/imap", map[string]any{"host": "imap.invalid", "username": "alice", "password": "encrypted-password"}, 200)
	v = alice.request("GET", "/api/settings/imap", nil, 200)
	if v["password"] != "••••••••" {
		t.Fatal("mailbox password exposed")
	}
	alice.request("POST", "/api/settings/imap", map[string]any{"host": "imap.invalid", "username": "alice", "password": ""}, 200)
	alice.request("POST", "/api/shared-dates", map[string]any{"date": "2026-10-07", "recipientIds": []string{bu}}, 201)
	bob.request("GET", "/api/shared-dates", nil, 200)
	alice.request("POST", "/api/admin/view-as", map[string]any{"userId": bu}, 200)
	v = alice.request("GET", "/api/me", nil, 200)
	if v["effectiveUser"].(map[string]any)["id"] != bu || v["user"].(map[string]any)["id"] != au {
		t.Fatal("view-as identity")
	}
	alice.request("GET", "/api/admin/users", nil, 200)
	alice.request("GET", "/api/pages/"+page, nil, 200)
	alice.request("POST", "/api/admin/view-as", map[string]any{"userId": nil}, 200)
	alice.request("PATCH", "/api/admin/users", map[string]any{"userId": au, "role": "USER"}, 400)
	alice.request("GET", "/api/auth/oidc/status", nil, 200)
	alice.request("GET", "/api/auth/callback/authelia", nil, 501)
	bob.request("DELETE", "/api/pages/"+page, nil, 200)
	alice.request("DELETE", "/api/bookmarks/"+id, nil, 200)
	anonymous.request("GET", "/api/public/bookmarks/"+id, nil, 404)
	alice.request("POST", "/api/auth/signout", map[string]any{}, 200)
	alice.request("GET", "/api/me", nil, 401)
}
