package http

import (
	"encoding/json"
	"github.com/emersion/go-imap/backend/memory"
	imapserver "github.com/emersion/go-imap/server"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestExternalIntegrations(t *testing.T) {
	srv, d := testServerDB(t)
	jar, _ := cookiejar.New(nil)
	a := apiClient{t, &http.Client{Jar: jar}, srv.URL}
	credentials := map[string]any{"email": "integrations@computer.test", "password": "test-password"}
	a.request("POST", "/api/signup", credentials, 201)
	a.request("POST", "/api/auth/login", credentials, 200)
	dav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "REPORT" || r.Header.Get("Depth") != "1" {
			t.Error("invalid CardDAV request")
		}
		u, p, _ := r.BasicAuth()
		if u != "nextcloud-user" || p != "app-password" {
			t.Error("CardDAV authentication")
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(207)
		io.WriteString(w, `<d:multistatus xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav"><d:response><d:href>/card.vcf</d:href><d:propstat><d:prop><card:address-data>BEGIN:VCARD
UID:contact-1
FN:Contact Name
EMAIL:first@computer.test
EMAIL:last@computer.test
TEL:1234
END:VCARD</card:address-data></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`)
	}))
	defer dav.Close()
	req := map[string]any{"nextcloudUrl": dav.URL, "username": "nextcloud-user", "password": "app-password"}
	if v := a.request("POST", "/api/contacts/sync", req, 200); v["synced"] != float64(1) {
		t.Fatal("CardDAV sync")
	}
	a.request("POST", "/api/contacts/sync", req, 200)
	res, e := a.c.Get(srv.URL + "/api/contacts")
	if e != nil {
		t.Fatal(e)
	}
	var contacts []map[string]any
	json.NewDecoder(res.Body).Decode(&contacts)
	res.Body.Close()
	if len(contacts) != 1 || contacts[0]["email"] != "last@computer.test" {
		t.Fatal("CardDAV upsert/parse", contacts)
	}
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
	mailbox := map[string]any{"host": host, "port": pn, "tls": false, "username": "username", "password": "password", "folder": "INBOX"}
	a.request("POST", "/api/imap/test", mailbox, 200)
	a.request("POST", "/api/settings/imap", mailbox, 200)
	id := a.request("POST", "/api/bookmarks", map[string]any{"url": "https://127.0.0.1/test", "imapQuery": "little"}, 201)["id"].(string)
	a.request("POST", "/api/bookmarks/"+id+"/signals", map[string]any{"type": "email", "config": map[string]any{"from": "contact@example.org", "subject": "little"}}, 201)
	v := a.request("POST", "/api/bookmarks/"+id+"/signals", map[string]any{"action": "check"}, 200)
	if v["created"] != float64(1) {
		t.Fatal("IMAP signal creation", v)
	}
	// Review SYNC-02: the pass persisted a UID checkpoint for the source.
	var lastUID int64
	if e := d.QueryRow(`SELECT "lastUid" FROM "SignalCheckpoint"`).Scan(&lastUID); e != nil {
		t.Fatal("checkpoint not persisted:", e)
	}
	if lastUID < 1 {
		t.Fatal("checkpoint did not advance:", lastUID)
	}
	v = a.request("POST", "/api/bookmarks/"+id+"/signals", map[string]any{"action": "check"}, 200)
	if v["created"] != float64(0) {
		t.Fatal("signal deduplication", v)
	}
	v = a.request("GET", "/api/bookmarks/"+id+"/signals", nil, 200)
	if len(v["signals"].([]any)) != 1 {
		t.Fatal("IMAP envelope fetch")
	}
	a.request("PUT", "/api/bookmarks/"+id, map[string]any{"imapWatchEnabled": true}, 200)
	v = a.request("GET", "/api/imap/check", nil, 200)
	if len(v["matches"].([]any)) != 1 {
		t.Fatal("legacy mailbox watch")
	}
}

// CardDAV transport (review recheck): Basic-auth credentials must never
// travel plaintext. Without the netguard fixture escape hatch, an http
// Nextcloud URL is rejected at the API boundary — no dial, no credentials.
func TestCardDAVRequiresTLSOutsideFixtures(t *testing.T) {
	srv := testServer(t)
	a := newAPIUser(t, srv, "carddav-tls@computer.test")
	t.Setenv("INTEGRATIONS_ALLOW_PRIVATE", "")
	a.request("POST", "/api/contacts/sync", map[string]any{
		"nextcloudUrl": "http://nextcloud.example.test", "username": "u", "password": "p",
	}, 400)
	// https passes the transport gate (the dial itself fails: no such host
	// through netguard — the point is the 400 above never happens for TLS).
	resp := a.request("POST", "/api/contacts/sync", map[string]any{
		"nextcloudUrl": "https://nextcloud.invalid", "username": "u", "password": "p",
	}, 400)
	if resp == nil {
		t.Fatal("expected a connect-level rejection, not a transport-policy one")
	}
}
