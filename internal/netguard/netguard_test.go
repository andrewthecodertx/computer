package netguard

import (
	"context"
	"net"
	"testing"
)

func TestIsPublic(t *testing.T) {
	public := []string{"8.8.8.8", "1.1.1.1", "2001:4860:4860::8888"}
	blocked := []string{
		"127.0.0.1", "::1", // loopback
		"10.1.2.3", "172.16.0.1", "192.168.1.1", "fc00::1", "fd12::1", // RFC1918 + ULA
		"169.254.169.254", "fe80::1", // link-local / cloud metadata
		"100.64.0.1", "100.127.255.254", // RFC 6598 CGNAT
		"198.18.0.1", "198.19.255.255", // benchmark
		"192.0.0.1", // IETF protocol assignments
		"0.0.0.0", "224.0.0.1", // unspecified / multicast
		"::ffff:10.0.0.1", // IPv4-mapped private
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
