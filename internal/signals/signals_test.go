package signals

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Review NET-02: mailbox credentials must never travel over a plaintext
// socket unless the explicit development fixture flag is set.
func TestPlaintextTransportRejected(t *testing.T) {
	t.Setenv("INTEGRATIONS_ALLOW_INSECURE_IMAP", "")
	if e := checkTransport(Mailbox{TLS: false}); e == nil {
		t.Fatal("plaintext mailbox accepted without the development flag")
	}
	if e := checkTransport(Mailbox{TLS: true}); e != nil {
		t.Fatalf("TLS mailbox rejected: %v", e)
	}
	// The guard runs before any dial, so Connect fails fast without network.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, e := Connect(ctx, Mailbox{Host: "127.0.0.1", Port: 1, TLS: false})
	if e == nil || !strings.Contains(e.Error(), "TLS is required") {
		t.Fatalf("plaintext Connect was not refused up front: %v", e)
	}
	t.Setenv("INTEGRATIONS_ALLOW_INSECURE_IMAP", "1")
	if e := checkTransport(Mailbox{TLS: false}); e != nil {
		t.Fatalf("development fixture flag did not permit plaintext: %v", e)
	}
}
