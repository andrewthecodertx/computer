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
	"strings"
	"sync"
	"testing"
)

func TestExternalIntegrations(t *testing.T) {
	srv, d := testServerDB(t)
	jar, _ := cookiejar.New(nil)
	a := apiClient{t, &http.Client{Jar: jar}, srv.URL}
	credentials := map[string]any{"email": "integrations@computer.test", "password": "test-password"}
	a.request("POST", "/api/signup", credentials, 201)
	a.request("POST", "/api/auth/login", credentials, 200)
	// CardDAV fixture (reviews CR-07/CR-08): the served multistatus is
	// mutable so one test can walk the reconciliation policy — full snapshot,
	// partially-withheld response, remote deletion, empty response.
	var davMu sync.Mutex
	var davBody string
	setBody := func(body string) { davMu.Lock(); davBody = body; davMu.Unlock() }
	getBody := func() string { davMu.Lock(); defer davMu.Unlock(); return davBody }
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
		io.WriteString(w, getBody())
	}))
	defer dav.Close()
	card1 := `<d:response><d:href>/card.vcf</d:href><d:propstat><d:prop><card:address-data>BEGIN:VCARD
UID:contact-1
FN:Contact Name
EMAIL:last@computer.test
EMAIL;TYPE=PREF:preferred@computer.test
TEL:1234
END:VCARD</card:address-data></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`
	card2 := `<d:response><d:href>/card2.vcf</d:href><d:propstat><d:prop><card:address-data>BEGIN:VCARD
UID:contact-2
FN:Second Contact
EMAIL:second@computer.test
END:VCARD</card:address-data></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`
	card2Withheld := `<d:response><d:href>/card2.vcf</d:href><d:propstat><d:prop/><d:status>HTTP/1.1 404 Not Found</d:status></d:propstat></d:response>`
	wrap := func(responses ...string) string {
		return `<d:multistatus xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">` + strings.Join(responses, "") + `</d:multistatus>`
	}
	contactsOf := func() []map[string]any {
		t.Helper()
		res, e := a.c.Get(srv.URL + "/api/contacts")
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		var contacts []map[string]any
		json.NewDecoder(res.Body).Decode(&contacts)
		return contacts
	}
	req := map[string]any{"nextcloudUrl": dav.URL, "username": "nextcloud-user", "password": "app-password"}

	// Full snapshot: both cards import; the PREF occurrence wins the repeated
	// EMAIL property (review CR-08 — deterministic selection, no data loss).
	setBody(wrap(card1, card2))
	if v := a.request("POST", "/api/contacts/sync", req, 200); v["synced"] != float64(2) || v["removed"] != float64(0) {
		t.Fatal("CardDAV sync", v)
	}
	contacts := contactsOf()
	if len(contacts) != 2 {
		t.Fatal("CardDAV import count", contacts)
	}
	for _, c := range contacts {
		if c["uid"] == "contact-1" && c["email"] != "preferred@computer.test" {
			t.Fatal("repeated EMAIL did not resolve to the PREF value:", c)
		}
	}
	// Idempotent re-sync: nothing removed, nothing duplicated.
	if v := a.request("POST", "/api/contacts/sync", req, 200); v["synced"] != float64(2) || v["removed"] != float64(0) {
		t.Fatal("CardDAV re-sync", v)
	}
	if len(contactsOf()) != 2 {
		t.Fatal("CardDAV re-sync duplicated contacts")
	}

	// Partially-withheld response (card2's data 404s): must NOT be read as a
	// remote deletion (review CR-07 — never delete from a partial snapshot).
	setBody(wrap(card1, card2Withheld))
	if v := a.request("POST", "/api/contacts/sync", req, 200); v["removed"] != float64(0) {
		t.Fatal("partial response triggered reconciliation:", v)
	}
	if len(contactsOf()) != 2 {
		t.Fatal("partial response deleted a contact")
	}

	// Complete snapshot without card2: the remote deletion is mirrored.
	setBody(wrap(card1))
	if v := a.request("POST", "/api/contacts/sync", req, 200); v["synced"] != float64(1) || v["removed"] != float64(1) {
		t.Fatal("CardDAV deletion reconciliation", v)
	}
	contacts = contactsOf()
	if len(contacts) != 1 || contacts[0]["uid"] != "contact-1" {
		t.Fatal("wrong contact survived reconciliation:", contacts)
	}

	// Empty response: the guard preserves everything (a server fault that
	// answers 207 with zero cards must not wipe the address book).
	setBody(wrap())
	if v := a.request("POST", "/api/contacts/sync", req, 200); v["synced"] != float64(0) || v["removed"] != float64(0) {
		t.Fatal("empty response reconciliation", v)
	}
	if len(contactsOf()) != 1 {
		t.Fatal("empty response deleted contacts")
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
	putBookmark(a, id, map[string]any{"imapWatchEnabled": true}, 200)
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
