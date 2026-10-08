package http

import (
	"bytes"
	"context"
	"github.com/andrew/go-computer/internal/scripts"
	"net/http"
	"net/http/cookiejar"
	"testing"
)

func TestConfiguredAdminCannotBeBypassedDuringBootstrap(t *testing.T) {
	srv, d := testServerDB(t)
	t.Setenv("ADMIN_EMAILS", "operator@computer.test")
	visitor, _ := accountClient(t, srv.URL, "visitor@computer.test")
	if visitor.request("GET", "/api/me", nil, 200)["isAdmin"] != false {
		t.Fatal("public signup bypassed reserved administrator bootstrap")
	}
	visitor.request("GET", "/api/admin/users", nil, 403)
	t.Setenv("SEED_EMAIL", "operator@computer.test")
	t.Setenv("SEED_PASSWORD", "operator-test-password")
	if err := scripts.Seed(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	visitor.request("POST", "/api/auth/login", map[string]any{"email": "operator@computer.test", "password": "operator-test-password"}, 200)
	if visitor.request("GET", "/api/me", nil, 200)["isAdmin"] != true {
		t.Fatal("operator-provisioned account is not admin")
	}
	t.Setenv("SEED_PASSWORD", "different-password")
	if err := scripts.Seed(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	visitor.request("POST", "/api/auth/login", map[string]any{"email": "operator@computer.test", "password": "operator-test-password"}, 200)
}

func accountClient(t *testing.T, base, email string) (apiClient, string) {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := apiClient{t, &http.Client{Jar: jar}, base}
	body := map[string]any{"email": email, "password": "test-long-password"}
	id := c.request("POST", "/api/signup", body, 201)["userId"].(string)
	c.request("POST", "/api/auth/login", body, 200)
	return c, id
}

func TestAdminEnrollmentRequiresTrustedIdentity(t *testing.T) {
	srv, d := testServerDB(t)
	admin, _ := accountClient(t, srv.URL, "admin@computer.test")
	user, uid := accountClient(t, srv.URL, "unverified@computer.test")
	t.Setenv("ADMIN_EMAILS", "reserved@computer.test,unverified@computer.test")
	user.request("POST", "/api/signup", map[string]any{"email": "reserved@computer.test", "password": "test-password"}, 403)
	if user.request("GET", "/api/me", nil, 200)["isAdmin"] != false {
		t.Fatal("unverified address promoted to admin")
	}
	user.request("GET", "/api/admin/users", nil, 403)
	if _, err := d.Exec(`UPDATE "User" SET "emailVerified"=now() WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	if user.request("GET", "/api/me", nil, 200)["isAdmin"] != true {
		t.Fatal("verified allowlisted address was not promoted")
	}
	admin.request("GET", "/api/admin/users", nil, 200)
}

func TestPublicBookmarksDoNotExposePrivateMetadata(t *testing.T) {
	srv, d := testServerDB(t)
	owner, uid := accountClient(t, srv.URL, "owner@computer.test")
	viewer, vid := accountClient(t, srv.URL, "viewer@computer.test")
	if _, err := d.Exec(`INSERT INTO "Contact"(id,uid,"displayName",email,phone,"userId") VALUES('contact','vcard','Private','private@computer.test','555-private',$1)`, uid); err != nil {
		t.Fatal(err)
	}
	id := owner.request("POST", "/api/bookmarks", map[string]any{"url": "https://127.0.0.1/test", "isPublic": true, "contactId": "contact", "imapQuery": "private mailbox query", "alertAt": "2026-10-07"}, 201)["id"].(string)
	v := viewer.request("GET", "/api/public/bookmarks/"+id, nil, 200)
	for _, key := range []string{"contact", "contactId", "sharedWith", "ownerId", "imapQuery", "imapWatchEnabled", "alertAt", "kanbanColumnId", "pageLinks"} {
		if _, exists := v[key]; exists {
			t.Errorf("public response includes %s", key)
		}
	}
	if _, exists := v["owner"].(map[string]any)["email"]; exists {
		t.Fatal("owner email exposed")
	}
	viewer.request("GET", "/api/bookmarks/"+id, nil, 404)
	owner.request("POST", "/api/bookmarks/"+id+"/share", map[string]any{"userIds": []string{vid}}, 200)
	viewer.request("GET", "/api/bookmarks/"+id, nil, 200)
	owner.request("PUT", "/api/bookmarks/"+id, map[string]any{"isPublic": false}, 200)
	viewer.request("GET", "/api/public/bookmarks/"+id, nil, 404)
}

func TestDirectLoginRejectsBrowserForgery(t *testing.T) {
	srv := testServer(t)
	for _, tc := range []struct {
		contentType, origin string
		status              int
	}{
		{"text/plain", "https://attacker.invalid", 415},
		{"application/json", "https://attacker.invalid", 403},
	} {
		req, _ := http.NewRequest("POST", srv.URL+"/api/auth/login", bytes.NewBufferString(`{"email":"attacker@computer.test","password":"password"}`))
		req.Header.Set("Content-Type", tc.contentType)
		req.Header.Set("Origin", tc.origin)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != tc.status || len(res.Cookies()) != 0 {
			t.Fatalf("forged login: status=%d cookies=%v", res.StatusCode, res.Cookies())
		}
	}
}
