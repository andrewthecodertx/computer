package http

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/andrew/go-computer/internal/scripts"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strconv"
	"testing"
	"time"
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
	c.request("POST", "/api/signup", body, 201)
	c.request("POST", "/api/auth/login", body, 200)
	// Review CR-03: the signup body no longer carries the id; identity comes
	// from the authenticated session.
	return c, meID(c)
}

func TestAdminEnrollmentRequiresTrustedIdentity(t *testing.T) {
	srv, d := testServerDB(t)
	admin, _ := accountClient(t, srv.URL, "admin@computer.test")
	user, uid := accountClient(t, srv.URL, "unverified@computer.test")
	t.Setenv("ADMIN_EMAILS", "reserved@computer.test,unverified@computer.test")
	// Reserved addresses answer exactly like existing accounts (no 403 oracle).
	// Review SEC-4/CR-03: operator-reserved addresses return the SAME uniform
	// 201 {"ok":true} body as every other accepted registration — no userId
	// field, no 409.
	reserved := user.request("POST", "/api/signup", map[string]any{"email": "reserved@computer.test", "password": "test-password"}, 201)
	if len(reserved) != 1 || reserved["ok"] != true {
		t.Fatalf("operator-reserved address body is distinguishable: %v", reserved)
	}
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
	// Review PRIV-04: public links carry no owner attribution at all; the
	// README promises owner identity stays private.
	for _, key := range []string{"contact", "contactId", "sharedWith", "owner", "ownerId", "imapQuery", "imapWatchEnabled", "alertAt", "alertSent", "kanbanColumnId", "pageLinks"} {
		if _, exists := v[key]; exists {
			t.Errorf("public response includes %s", key)
		}
	}
	viewer.request("GET", "/api/bookmarks/"+id, nil, 404)
	owner.request("POST", "/api/bookmarks/"+id+"/share", map[string]any{"userIds": []string{vid}}, 200)
	// Review PRIV-02: an authenticated shared reader gets the narrowed
	// projection — no contact record, no recipient list, no owner operational
	// metadata — while the owner keeps the full view.
	shared := viewer.request("GET", "/api/bookmarks/"+id, nil, 200)
	for _, key := range []string{"contact", "contactId", "sharedWith", "imapQuery", "imapWatchEnabled", "alertAt", "alertSent", "kanbanColumnId"} {
		if _, exists := shared[key]; exists {
			t.Errorf("shared-reader response includes %s", key)
		}
	}
	if shared["url"] != "https://127.0.0.1/test" || shared["ownerId"] != uid {
		t.Error("shared-reader response lost basic bookmark fields")
	}
	if _, exists := shared["tags"]; !exists {
		t.Error("shared-reader response lost tags")
	}
	full := owner.request("GET", "/api/bookmarks/"+id, nil, 200)
	if c, _ := full["contact"].(map[string]any); c == nil || c["email"] != "private@computer.test" {
		t.Error("owner projection lost the contact record")
	}
	if s, ok := full["sharedWith"].([]any); !ok || len(s) != 1 {
		t.Error("owner projection lost the recipient list")
	}
	if _, exists := full["imapQuery"]; !exists {
		t.Error("owner projection lost mailbox-watch settings")
	}
	// The list endpoint must apply the same permission-specific projections.
	res, err := viewer.c.Get(srv.URL + "/api/bookmarks?shared=true")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var list []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0]["id"] != id {
		t.Fatalf("shared list returned %d entries", len(list))
	}
	for _, key := range []string{"contact", "sharedWith", "imapQuery", "alertAt"} {
		if _, exists := list[0][key]; exists {
			t.Errorf("shared list response includes %s", key)
		}
	}
	putBookmark(owner, id, map[string]any{"isPublic": false}, 200)
	viewer.request("GET", "/api/public/bookmarks/"+id, nil, 404)
}

func TestAdminBootstrapIsOneShot(t *testing.T) {
	srv, d := testServerDB(t)
	// Fresh installation: the first signup completes bootstrap as admin.
	admin, adminID := accountClient(t, srv.URL, "first@computer.test")
	if admin.request("GET", "/api/me", nil, 200)["isAdmin"] != true {
		t.Fatal("first signup on a fresh installation was not admin")
	}
	// Review AUTH-06 acceptance: demoting the last administrator must not
	// re-open administrator assignment through signup.
	if _, err := d.Exec(`UPDATE "User" SET role='USER' WHERE id=$1`, adminID); err != nil {
		t.Fatal(err)
	}
	latecomer, _ := accountClient(t, srv.URL, "late@computer.test")
	if latecomer.request("GET", "/api/me", nil, 200)["isAdmin"] != false {
		t.Fatal("signup re-opened admin bootstrap after the last admin was demoted")
	}
	// Removing every administrator entirely does not re-open it either.
	if _, err := d.Exec(`DELETE FROM "User" WHERE id=$1`, adminID); err != nil {
		t.Fatal(err)
	}
	successor, _ := accountClient(t, srv.URL, "successor@computer.test")
	if successor.request("GET", "/api/me", nil, 200)["isAdmin"] != false {
		t.Fatal("signup re-opened admin bootstrap after the last admin was removed")
	}
	successor.request("GET", "/api/admin/users", nil, 403)
}

// Review SEC-5: with ALLOW_BOOTSTRAP_SIGNUP=0 a fresh installation must not
// hand administrator to the first signup — provisioning stays operator-driven.
func TestBootstrapSignupCanBeDisabled(t *testing.T) {
	srv := testServer(t)
	t.Setenv("ALLOW_BOOTSTRAP_SIGNUP", "0")
	visitor, _ := accountClient(t, srv.URL, "unclaimed@computer.test")
	if visitor.request("GET", "/api/me", nil, 200)["isAdmin"] != false {
		t.Fatal("first signup became admin despite ALLOW_BOOTSTRAP_SIGNUP=0")
	}
	visitor.request("GET", "/api/admin/users", nil, 403)
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

// Review SEC-1/SEC-2: the failure lockout keys on the proxy-attested browser
// IP. Two different signed identities get independent lockout buckets; a
// replayed (stale-timestamp) signature is not honored.
func TestLoginLockoutHonorsSignedForwardedIdentity(t *testing.T) {
	srv := testServer(t)
	secret := os.Getenv("AUTH_SECRET")
	sign := func(ip string, ts int64) (string, string, string) {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(ip + "|" + strconv.FormatInt(ts, 10)))
		return ip, strconv.FormatInt(ts, 10), hex.EncodeToString(mac.Sum(nil))
	}
	post := func(email, password, ip string, ts int64) int {
		body, _ := json.Marshal(map[string]any{"email": email, "password": password})
		req, _ := http.NewRequest("POST", srv.URL+"/api/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if ip != "" {
			hip, hts, sig := sign(ip, ts)
			req.Header.Set("X-Computer-Client-Ip", hip)
			req.Header.Set("X-Computer-Client-Ts", hts)
			req.Header.Set("X-Computer-Client-Sig", sig)
		}
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		res.Body.Close()
		return res.StatusCode
	}
	now := time.Now().Unix()
	// Browser A: five failures lock out its (IP, email) bucket.
	for i := 0; i < 5; i++ { // failureThreshold
		if code := post("victim-a@computer.test", "wrong-password", "198.51.100.1", now); code != 401 {
			t.Fatalf("browser A attempt %d: %d", i+1, code)
		}
	}
	if code := post("victim-a@computer.test", "wrong-password", "198.51.100.1", now); code != 429 {
		t.Fatalf("browser A not locked out: %d", code)
	}
	// Browser B behind the same proxy peer keeps its own buckets.
	if code := post("victim-a@computer.test", "wrong-password", "198.51.100.2", now); code != 401 {
		t.Fatalf("browser B locked out by browser A's failures: %d", code)
	}
	// A stale signature (outside the ~60s window) falls back to the peer, so
	// it cannot smuggle a fresh identity: these attempts count against the
	// peer bucket and stay 401.
	if code := post("victim-b@computer.test", "wrong-password", "198.51.100.9", now-3600); code != 401 {
		t.Fatalf("stale signature changed behavior: %d", code)
	}
}
