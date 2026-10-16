// Package netguard blocks server-initiated connections to non-public
// destinations. It backs URL previews, CardDAV sync and IMAP connections so a
// user-supplied host can never point the server at another user's internal
// service, the Docker database, or cloud metadata endpoints.
package netguard

import (
	"context"
	"errors"
	"net"
	"os"
	"time"
)

// AllowPrivate reports whether range checks are disabled via
// INTEGRATIONS_ALLOW_PRIVATE=1. Tests set it for loopback fake servers;
// operators can set it for deliberate LAN deployments (home Nextcloud or
// mail on private addresses). It is off in the production Compose stack.
func AllowPrivate() bool { return os.Getenv("INTEGRATIONS_ALLOW_PRIVATE") == "1" }

// extraBlocked covers ranges net.IP's helpers miss: RFC 6598 CGNAT
// (100.64.0.0/10 — used by Tailscale, GKE internal LBs, some Docker setups),
// benchmark space (198.18.0.0/15), IETF protocol assignments (192.0.0.0/24),
// IPv4 documentation space (192.0.2.0/24), the limited broadcast address and
// the IPv6 documentation prefix (2001:db8::/32).
var extraBlocked = func() []net.IPNet {
	nets := make([]net.IPNet, 0, 6)
	for _, cidr := range []string{"100.64.0.0/10", "198.18.0.0/15", "192.0.0.0/24", "192.0.2.0/24", "255.255.255.255/32", "2001:db8::/32"} {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic("netguard: bad CIDR " + cidr)
		}
		nets = append(nets, *n)
	}
	return nets
}()

// nat64Prefix is the RFC 6052 well-known NAT64 prefix. Addresses inside it
// embed an IPv4 destination in their low 32 bits, so the guard must judge
// the EMBEDDED address — 64:ff9b::7f00:1 is loopback wearing an IPv6 coat.
var nat64Prefix = func() net.IPNet {
	_, n, err := net.ParseCIDR("64:ff9b::/96")
	if err != nil {
		panic("netguard: bad CIDR 64:ff9b::/96")
	}
	return *n
}()

// Destination policy (review follow-up, stated explicitly): allowed targets
// are globally routable unicast addresses only. Blocked: private (RFC 1918,
// ULA fc00::/7), loopback, link-local, CGNAT, benchmark, IETF protocol and
// documentation ranges, unspecified/broadcast/multicast, IPv4-mapped
// addresses whose embedded IPv4 is non-public, and NAT64 addresses whose
// embedded IPv4 is non-public. INTEGRATIONS_ALLOW_PRIVATE=1 (fixtures and
// deliberate LAN deployments) is the only relaxation.
func IsPublic(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if nat64Prefix.Contains(ip) {
		if v6 := ip.To16(); v6 != nil {
			return IsPublic(net.IPv4(v6[12], v6[13], v6[14], v6[15]))
		}
	}
	for _, n := range extraBlocked {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

var errBlocked = errors.New("netguard: private destination blocked")

// dialMaxAttempts bounds the fallback across already-validated addresses
// (review follow-up): the first A/AAAA record being unreachable does not make
// the host unreachable, but we never chase an unbounded list — and there is
// deliberately no second DNS lookup anywhere in this path.
const dialMaxAttempts = 3

// DialContext resolves the host, rejects non-public destinations, then dials
// the checked IPs. Check and dial use the same lookup result, so there is no
// DNS-rebinding window between them. Every resolved address must pass the
// range check (a host that mixes public and private records is refused
// outright), and the dial then falls back among the validated addresses
// instead of failing on the first unreachable one.
func DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, e := net.SplitHostPort(address)
	if e != nil {
		return nil, e
	}
	ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
	if e != nil {
		return nil, e
	}
	if len(ips) == 0 {
		return nil, errors.New("netguard: unresolved address")
	}
	if !AllowPrivate() {
		for _, a := range ips {
			if !IsPublic(a.IP) {
				return nil, errBlocked
			}
		}
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var lastErr error
	for i, a := range ips {
		if i >= dialMaxAttempts {
			break
		}
		var conn net.Conn
		conn, lastErr = dialer.DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
		if lastErr == nil {
			return conn, nil
		}
	}
	return nil, lastErr
}
