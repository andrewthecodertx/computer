package netguard

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestIsPublic(t *testing.T) {
	public := []string{
		"8.8.8.8", "1.1.1.1", "2001:4860:4860::8888",
		// NAT64 wrapping a PUBLIC embedded IPv4 addresses a real public
		// destination on IPv6-only networks — allowed by policy.
		"64:ff9b::8.8.8.8",
	}
	blocked := []string{
		"127.0.0.1", "::1", // loopback
		"10.1.2.3", "172.16.0.1", "192.168.1.1", "fc00::1", "fd12::1", // RFC1918 + ULA
		"169.254.169.254", "fe80::1", // link-local / cloud metadata
		"100.64.0.1", "100.127.255.254", // RFC 6598 CGNAT
		"198.18.0.1", "198.19.255.255", // benchmark
		"192.0.0.1",                // IETF protocol assignments
		"192.0.2.1", "2001:db8::1", // documentation space (v4 + v6)
		"0.0.0.0", "::", "224.0.0.1", "ff02::1", "255.255.255.255", // unspecified / multicast / broadcast
		"::ffff:10.0.0.1", "::ffff:127.0.0.1", // IPv4-mapped private/loopback
		"64:ff9b::7f00:1", "64:ff9b::a00:1", "64:ff9b::a9fe:a9fe", // NAT64-embedded loopback / private / metadata
	}
	for _, s := range public {
		if !IsPublic(net.ParseIP(s)) {
			t.Errorf("IsPublic(%s) = false, want true", s)
		}
	}
	for _, s := range blocked {
		if IsPublic(net.ParseIP(s)) {
			t.Errorf("IsPublic(%s) = true, want false", s)
		}
	}
}

func TestAllowPrivateEnv(t *testing.T) {
	if AllowPrivate() {
		t.Fatal("AllowPrivate must default to false")
	}
	t.Setenv("INTEGRATIONS_ALLOW_PRIVATE", "1")
	if !AllowPrivate() {
		t.Fatal("AllowPrivate must honor INTEGRATIONS_ALLOW_PRIVATE=1")
	}
}

func TestDialContextBlocksLoopbackByDefault(t *testing.T) {
	if _, e := DialContext(context.Background(), "tcp", "127.0.0.1:5432"); e == nil {
		t.Fatal("loopback destination accepted")
	}
}

// The NAT64 well-known prefix must not smuggle a non-public IPv4 destination
// past the guard (review follow-up: comprehensive special-range coverage).
func TestDialContextBlocksNAT64WrappedLoopback(t *testing.T) {
	if _, e := DialContext(context.Background(), "tcp", "[64:ff9b::7f00:1]:80"); e == nil {
		t.Fatal("NAT64-wrapped loopback destination accepted")
	}
	if _, e := DialContext(context.Background(), "tcp", "[64:ff9b::a9fe:a9fe]:80"); e == nil {
		t.Fatal("NAT64-wrapped metadata destination accepted")
	}
}

// Bounded fallback among already-validated addresses (review follow-up): a
// host that resolves to several addresses connects even when the first
// record is unreachable. "localhost" resolves to ::1 and/or 127.0.0.1
// depending on the resolver; the listener binds only IPv4 loopback, so on
// dual-stack machines the ::1 attempt must fail over to 127.0.0.1.
func TestDialContextFallsBackAmongValidatedAddresses(t *testing.T) {
	t.Setenv("INTEGRATIONS_ALLOW_PRIVATE", "1")
	ln, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Skipf("no IPv4 loopback listener possible: %v", e)
	}
	defer ln.Close()
	go func() {
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			c.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, e := DialContext(ctx, "tcp", fmt.Sprintf("localhost:%d", ln.Addr().(*net.TCPAddr).Port))
	if e != nil {
		t.Fatalf("fallback dial to localhost failed: %v", e)
	}
	conn.Close()
}
