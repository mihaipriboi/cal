package httpapi

// Outbound HTTP for user-supplied URLs (link previews, feeds, webhooks).
// The address check runs in the dialer, on the IP actually being connected
// to, so it also covers every redirect hop and DNS answers that change
// between checkURL's lookup and the connection.

import (
	"errors"
	"net"
	"net/http"
	"syscall"
	"time"
)

var errPrivateAddress = errors.New("private address")

// blockedNets are ranges a user-supplied URL may not reach: private,
// loopback, link-local, shared (CGNAT), multicast and reserved space, plus
// IPv6 prefixes that embed or translate to IPv4 addresses.
var blockedNets = mustParseCIDRs(
	"0.0.0.0/8",      // "this network"
	"10.0.0.0/8",     // private
	"100.64.0.0/10",  // shared address space (CGNAT)
	"127.0.0.0/8",    // loopback
	"169.254.0.0/16", // link-local, incl. cloud metadata
	"172.16.0.0/12",  // private
	"192.0.0.0/24",   // IETF protocol assignments
	"192.168.0.0/16", // private
	"198.18.0.0/15",  // benchmarking
	"224.0.0.0/4",    // multicast
	"240.0.0.0/4",    // reserved, incl. broadcast
	"::/96",          // unspecified, loopback, IPv4-compatible
	"64:ff9b::/96",   // NAT64
	"64:ff9b:1::/48", // local-use NAT64
	"100::/64",       // discard-only
	"2001::/32",      // Teredo
	"2002::/16",      // 6to4
	"fc00::/7",       // unique local
	"fe80::/10",      // link-local
	"fec0::/10",      // site-local (deprecated)
	"ff00::/8",       // multicast
)

func mustParseCIDRs(cidrs ...string) []*net.IPNet {
	nets := make([]*net.IPNet, len(cidrs))
	for i, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		nets[i] = n
	}
	return nets
}

// isBlockedIP reports whether ip is in blockedNets. IPv4-mapped IPv6
// addresses (::ffff:a.b.c.d) are checked as the IPv4 address they carry.
func isBlockedIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	for _, n := range blockedNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// newOutboundClient builds a client that refuses to connect to any IP for
// which blocked returns true (nil = no address restriction), follows at most
// 3 redirects and only to http(s).
func newOutboundClient(timeout time.Duration, blocked func(net.IP) bool) *http.Client {
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if blocked != nil {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			// ParseIP rejects zoned addresses (fe80::1%eth0): refused too.
			if ip := net.ParseIP(host); ip == nil || blocked(ip) {
				return errPrivateAddress
			}
			return nil
		}
		// Through a proxy the dialed address is the proxy's, not the target's.
		tr.Proxy = nil
	}
	tr.DialContext = dialer.DialContext
	return &http.Client{Timeout: timeout, Transport: tr, CheckRedirect: checkRedirect}
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > 3 {
		return errors.New("too many redirects")
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return errors.New("scheme not allowed")
	}
	return nil
}
