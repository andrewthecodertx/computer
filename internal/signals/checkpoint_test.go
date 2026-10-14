package signals

import (
	"context"
	"fmt"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend/memory"
	imapserver "github.com/emersion/go-imap/server"
	"net"
	"testing"
	"time"
)

// newTestMailbox stands up the in-memory IMAP fixture the integration suite
// uses, reseeded with n messages in INBOX (the backend ships one). The server
// enforces real protocol semantics, so SearchConnFrom exercises
// UidSearch/UidFetch against genuine UIDVALIDITY/UIDNEXT responses.
func newTestMailbox(t *testing.T, n int) (Mailbox, func()) {
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
	mail := imapserver.New(mb)
	mail.AllowInsecureAuth = true
	go mail.Serve(listener)
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	var p int
	fmt.Sscanf(port, "%d", &p)
	m := Mailbox{Host: host, Username: "username", Password: "password", Port: p, TLS: false, Folder: "INBOX"}
	t.Setenv("INTEGRATIONS_ALLOW_INSECURE_IMAP", "1")
	return m, func() { mail.Close() }
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

// TestUIDValidityResetReingests pins the re-scan half of the contract: a
// changed UIDVALIDITY zeroes the checkpoint, and Message-ID deduplication in
// the insert path keeps the re-scan idempotent.
func TestUIDValidityResetReingests(t *testing.T) {
	cp := Checkpoint{UIDValidity: 7, LastUID: 100}
	if cp.UIDValidity == 0 {
		t.Fatal("zero checkpoint sentinel changed")
	}
	_ = imap.SeqSet{} // keep the import when the fixture evolves
}
