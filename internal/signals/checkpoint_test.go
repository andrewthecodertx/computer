package signals

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
	"github.com/emersion/go-imap/backend/memory"
	imapserver "github.com/emersion/go-imap/server"
)

// validityBackend wraps the in-memory backend so tests can force the
// UIDVALIDITY the server reports (the stock memory backend hardcodes 1) and
// capture the SEARCH criteria every pass issues (review CR-15/CR-19
// verification).
type validityBackend struct {
	backend.Backend
	validity uint32
	mu       sync.Mutex
	criteria []*imap.SearchCriteria
}

func (b *validityBackend) Login(connInfo *imap.ConnInfo, username, password string) (backend.User, error) {
	u, e := b.Backend.Login(connInfo, username, password)
	if e != nil {
		return nil, e
	}
	return &validityUser{User: u, be: b}, nil
}

func (b *validityBackend) captured() []*imap.SearchCriteria {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]*imap.SearchCriteria(nil), b.criteria...)
}

type validityUser struct {
	backend.User
	be *validityBackend
}

func (u *validityUser) GetMailbox(name string) (backend.Mailbox, error) {
	m, e := u.User.GetMailbox(name)
	if e != nil {
		return nil, e
	}
	return &validityMailbox{Mailbox: m, be: u.be}, nil
}

type validityMailbox struct {
	backend.Mailbox
	be *validityBackend
}

func (m *validityMailbox) Status(items []imap.StatusItem) (*imap.MailboxStatus, error) {
	st, e := m.Mailbox.Status(items)
	if e == nil && m.be.validity != 0 {
		st.UidValidity = m.be.validity
	}
	return st, e
}

func (m *validityMailbox) SearchMessages(uid bool, c *imap.SearchCriteria) ([]uint32, error) {
	if uid {
		m.be.mu.Lock()
		m.be.criteria = append(m.be.criteria, c)
		m.be.mu.Unlock()
	}
	return m.Mailbox.SearchMessages(uid, c)
}

// newTestMailbox stands up the in-memory IMAP fixture the integration suite
// uses, reseeded with n messages in INBOX (the backend ships one). The server
// enforces real protocol semantics, so SearchConnFrom exercises
// UidSearch/UidFetch against genuine UIDVALIDITY/UIDNEXT responses.
func newTestMailbox(t *testing.T, n int) (Mailbox, func()) {
	m, _, cleanup := newTestMailboxValidity(t, n, 0)
	return m, cleanup
}

// newTestMailboxValidity is newTestMailbox with a forced UIDVALIDITY (0 keeps
// the backend default) and access to the criteria capture.
func newTestMailboxValidity(t *testing.T, n int, validity uint32) (Mailbox, *validityBackend, func()) {
	t.Helper()
	mb := memory.New()
	// Reach the fixture's own user/mailbox through a refused login, which is
	// how the backend exposes them.
	_, err := mb.Login(nil, "username", "bad")
	if err == nil {
		t.Fatal("fixture login unexpectedly succeeded")
	}
	user, err := mb.Login(nil, "username", "password")
	if err != nil {
		t.Fatal(err)
	}
	mbox, err := user.GetMailbox("INBOX")
	if err != nil {
		t.Fatal(err)
	}
	memMbox := mbox.(*memory.Mailbox)
	memMbox.Messages = memMbox.Messages[:0]
	for i := 0; i < n; i++ {
		body := "From: sender@computer.test\r\n" +
			"Subject: subject " + fmt.Sprint(i) + "\r\n" +
			"Message-Id: <msg-" + fmt.Sprint(i) + "@computer.test>\r\n" +
			"\r\n" + "body"
		memMbox.Messages = append(memMbox.Messages, &memory.Message{
			Uid:  uint32(i + 1),
			Date: time.Now(),
			Size: uint32(len(body)),
			Body: []byte(body),
		})
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	be := &validityBackend{Backend: mb, validity: validity}
	mail := imapserver.New(be)
	mail.AllowInsecureAuth = true
	go mail.Serve(listener)
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	var p int
	fmt.Sscanf(port, "%d", &p)
	m := Mailbox{Host: host, Username: "username", Password: "password", Port: p, TLS: false, Folder: "INBOX"}
	t.Setenv("INTEGRATIONS_ALLOW_INSECURE_IMAP", "1")
	return m, be, func() { mail.Close() }
}

// TestTruncatedPassKeepsRemainingMatches pins review COR-1: when the
// per-pass cap truncates the matched UID list, the checkpoint must advance
// only to the last fetched UID — never to the mailbox high-water mark — or
// matches past the cap are dropped forever.
func TestTruncatedPassKeepsRemainingMatches(t *testing.T) {
	// Connect dials loopback through netguard; the fixture flag unblocks it
	// (same escape hatch the integration harness uses).
	t.Setenv("INTEGRATIONS_ALLOW_PRIVATE", "1")
	cap := searchBatch * maxIngestBatches
	m, cleanup := newTestMailbox(t, cap+7)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, e := Connect(ctx, m)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Logout()

	events, next, e := SearchConnFrom(c, m, "", "subject", Checkpoint{})
	if e != nil {
		t.Fatal(e)
	}
	if len(events) != cap {
		t.Fatalf("fetched %d events, want %d (per-pass cap)", len(events), cap)
	}
	if next.LastUID != uint32(cap) {
		t.Fatalf("checkpoint advanced to %d, want %d (high-water shortcut must stay off after truncation)", next.LastUID, cap)
	}
	// A follow-up pass from the new checkpoint picks up every remaining match.
	rest, final, e := SearchConnFrom(c, m, "", "subject", next)
	if e != nil {
		t.Fatal(e)
	}
	if len(rest) != 7 {
		t.Fatalf("second pass fetched %d events, want 7", len(rest))
	}
	if final.LastUID != uint32(cap+7) {
		t.Fatalf("final checkpoint %d, want %d", final.LastUID, cap+7)
	}
}

func TestCheckpointHighWaterShortcutAppliesWhenComplete(t *testing.T) {
	t.Setenv("INTEGRATIONS_ALLOW_PRIVATE", "1")
	// When the SEARCH considered every message (no truncation), the
	// checkpoint may advance to the mailbox high-water mark.
	m, cleanup := newTestMailbox(t, searchBatch+1)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, e := Connect(ctx, m)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Logout()

	events, next, e := SearchConnFrom(c, m, "", "subject", Checkpoint{})
	if e != nil {
		t.Fatal(e)
	}
	if len(events) != searchBatch+1 {
		t.Fatalf("fetched %d events, want %d", len(events), searchBatch+1)
	}
	if next.LastUID != uint32(searchBatch+1) {
		t.Fatalf("checkpoint %d, want %d", next.LastUID, searchBatch+1)
	}
}

// TestUIDValidityResetReingests pins review CR-15 with a fixture-backed
// ingestion pass: when UIDVALIDITY changes, the new UID space is scanned from
// zero — even when every new UID sits BELOW the old checkpoint's LastUID.
// The old behavior filtered against the stale high-water mark and skipped the
// whole new space while advancing the checkpoint past it.
func TestUIDValidityResetReingests(t *testing.T) {
	t.Setenv("INTEGRATIONS_ALLOW_PRIVATE", "1")
	// New UID space: UIDs 1..20 under UIDVALIDITY 8.
	m, _, cleanup := newTestMailboxValidity(t, 20, 8)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, e := Connect(ctx, m)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Logout()

	// A checkpoint from the PREVIOUS UID space whose LastUID (100) is above
	// every UID that now exists.
	stale := Checkpoint{UIDValidity: 7, LastUID: 100}
	events, next, e := SearchConnFrom(c, m, "", "subject", stale)
	if e != nil {
		t.Fatal(e)
	}
	if len(events) != 20 {
		t.Fatalf("reset pass ingested %d events, want all 20 of the new UID space", len(events))
	}
	if next.UIDValidity != 8 || next.LastUID != 20 {
		t.Fatalf("checkpoint after reset = %+v, want {UIDValidity:8 LastUID:20}", next)
	}
	// A subsequent pass from the reset checkpoint finds nothing new and does
	// not regress the checkpoint.
	again, final, e := SearchConnFrom(c, m, "", "subject", next)
	if e != nil {
		t.Fatal(e)
	}
	if len(again) != 0 {
		t.Fatalf("follow-up pass ingested %d events, want 0", len(again))
	}
	if final.UIDValidity != 8 || final.LastUID != 20 {
		t.Fatalf("follow-up checkpoint = %+v, want {UIDValidity:8 LastUID:20}", final)
	}
}

// TestIncrementalPassConstrainsUIDSearch pins review CR-19: once a source is
// caught up, the server-side SEARCH carries a UID window derived from the
// normalized checkpoint instead of re-matching the whole history. The reset
// pass (CR-15) must NOT constrain — the whole new UID space is fair game.
func TestIncrementalPassConstrainsUIDSearch(t *testing.T) {
	t.Setenv("INTEGRATIONS_ALLOW_PRIVATE", "1")
	m, be, cleanup := newTestMailboxValidity(t, 10, 5)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, e := Connect(ctx, m)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Logout()

	// Pass 1: mismatched UIDVALIDITY forces a reset; no UID constraint.
	if _, next, e := SearchConnFrom(c, m, "", "subject", Checkpoint{UIDValidity: 4, LastUID: 99}); e != nil {
		t.Fatal(e)
	} else if next.LastUID != 10 {
		t.Fatalf("reset pass checkpoint %d, want 10", next.LastUID)
	}
	// Pass 2: caught up at UID 4 of validity 5 — only UIDs 5..10 ingest.
	events, next2, e := SearchConnFrom(c, m, "", "subject", Checkpoint{UIDValidity: 5, LastUID: 4})
	if e != nil {
		t.Fatal(e)
	}
	if len(events) != 6 {
		t.Fatalf("incremental pass ingested %d events, want 6", len(events))
	}
	if next2.LastUID != 10 {
		t.Fatalf("incremental checkpoint %d, want 10", next2.LastUID)
	}

	got := be.captured()
	if len(got) != 2 {
		t.Fatalf("captured %d UID searches, want 2", len(got))
	}
	if got[0].Uid != nil {
		t.Fatalf("reset pass constrained the SEARCH to %v; it must rescan the whole UID space", got[0].Uid)
	}
	if got[1].Uid == nil {
		t.Fatal("incremental pass did not constrain the SEARCH by UID window")
	}
	if !got[1].Uid.Contains(5) || got[1].Uid.Contains(4) {
		t.Fatalf("incremental SEARCH UID window %v does not start at 5", got[1].Uid)
	}
}
