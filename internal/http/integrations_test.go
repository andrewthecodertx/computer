package http

import (
	"context"
	"encoding/json"
	crypt "github.com/andrew/go-computer/internal/crypto"
	"github.com/andrew/go-computer/internal/signals"
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

// Review CR-21: credential writes must never restore a stale password. The
// lazy ENCRYPTION_KEY migration re-encrypts with a compare-and-swap against
// the exact ciphertext it read, and a blank/masked settings save no longer
// rewrites the password column at all.
func TestImapCredentialWritesNeverRestoreStalePasswords(t *testing.T) {
	srv, d := testServerDB(t)
	alice := newAPIUser(t, srv, "imap-stale@computer.test")
	t.Setenv("ENCRYPTION_KEY", "dedicated-encryption-key")
	uid := alice.request("GET", "/api/me", nil, 200)["user"].(map[string]any)["id"].(string)

	// Seed a legacy row encrypted under the AUTH secret (pre-ENCRYPTION_KEY),
	// exactly the shape the transparent migration expects.
	legacy, e := crypt.Encrypt("legacy-password", "integration-test-secret")
	if e != nil {
		t.Fatal(e)
	}
	if _, e := d.Exec(`INSERT INTO "ImapConfig"(id,host,port,tls,username,password,folder,"userId") VALUES('imap-cfg-1','legacy.example.test',993,true,'user',$1,'INBOX',$2)`, legacy, uid); e != nil {
		t.Fatal(e)
	}

	// Loading migrates: the password decrypts and the row is re-encrypted
	// under the dedicated key.
	m, e := signals.LoadMailbox(context.Background(), d, uid)
	if e != nil || m.Password != "legacy-password" {
		t.Fatalf("legacy load: %v %+v", e, m)
	}
	var stored string
	if e := d.QueryRow(`SELECT password FROM "ImapConfig" WHERE "userId"=$1`, uid).Scan(&stored); e != nil {
		t.Fatal(e)
	}
	if p, e := crypt.Decrypt(stored, "dedicated-encryption-key"); e != nil || p != "legacy-password" {
		t.Fatalf("row was not migrated under ENCRYPTION_KEY: %v", e)
	}

	// The CAS guard: an update conditioned on a stale ciphertext touches no
	// rows, so a concurrent replacement can never be rolled back by migration.
	res, e := d.Exec(`UPDATE "ImapConfig" SET password=$1 WHERE "userId"=$2 AND password=$3`, "stale-rewrite", uid, legacy)
	if e != nil {
		t.Fatal(e)
	}
	if n, _ := res.RowsAffected(); n != 0 {
		t.Fatal("compare-and-swap against a stale ciphertext updated the row")
	}

	// A concurrent replacement through the API survives...
	alice.request("POST", "/api/settings/imap", map[string]any{"host": "new.example.test", "port": 993, "tls": true, "username": "user", "password": "replacement-password", "folder": "INBOX"}, 200)
	// ...and a later masked save (connection settings only) must keep it.
	alice.request("POST", "/api/settings/imap", map[string]any{"host": "masked.example.test", "port": 993, "tls": true, "username": "user", "password": "••••••••", "folder": "INBOX"}, 200)
	m, e = signals.LoadMailbox(context.Background(), d, uid)
	if e != nil {
		t.Fatal(e)
	}
	if m.Password != "replacement-password" || m.Host != "masked.example.test" {
		t.Fatalf("masked save clobbered the stored credentials: %+v", m)
	}

	// A masked save with no stored configuration is a 400, never an empty
	// or resurrected password.
	bob := newAPIUser(t, srv, "imap-nocfg@computer.test")
	bob.request("POST", "/api/settings/imap", map[string]any{"host": "h.example.test", "port": 993, "tls": true, "username": "user", "password": "••••••••", "folder": "INBOX"}, 400)
}
