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
// benchmark space (198.18.0.0/15) and IETF protocol assignments (192.0.0.0/24).
var extraBlocked = func() []net.IPNet {
	nets := make([]net.IPNet, 0, 3)
	for _, cidr := range []string{"100.64.0.0/10", "198.18.0.0/15", "192.0.0.0/24"} {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic("netguard: bad CIDR " + cidr)
		}
		nets = append(nets, *n)
	}
	return nets
}()

// IsPublic reports whether ip is a publicly routable unicast address.
func IsPublic(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, n := range extraBlocked {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

var errBlocked = errors.New("netguard: private destination blocked")

// DialContext resolves the host, rejects non-public destinations, then dials
// the checked IP. Check and dial use the same lookup result, so there is no
// DNS-rebinding window between them.
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
	return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}
