package httpadapter

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// Fly Proxy supplies Fly-Client-IP. Enable this only when public traffic goes
// through Fly Proxy and other workloads on the private network are trusted.
func (s *Server) clientAddress(r *http.Request) string {
	if !s.cfg.TrustFlyProxy {
		return clientAddress(r.RemoteAddr, r.Header.Get("X-Forwarded-For"), s.cfg.TrustedProxies)
	}
	peer := clientAddress(r.RemoteAddr, "", nil)
	address, err := netip.ParseAddr(peer)
	if err != nil || !address.IsPrivate() {
		return peer
	}
	// Reject duplicate, malformed, or missing headers instead of falling back to
	// a potentially client-supplied forwarding chain.
	values := r.Header.Values("Fly-Client-IP")
	if len(values) != 1 {
		return peer
	}
	client, err := netip.ParseAddr(strings.TrimSpace(values[0]))
	if err != nil || client.Zone() != "" {
		return peer
	}
	return client.Unmap().String()
}

// clientAddress walks the forwarding chain only through explicitly trusted
// proxies, starting at the transport peer. The first untrusted hop is the client.
func clientAddress(remote, forwarded string, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	peer = peer.Unmap()
	isTrusted := func(ip netip.Addr) bool {
		for _, prefix := range trusted {
			if prefix.Contains(ip) {
				return true
			}
		}
		return false
	}
	if !isTrusted(peer) || forwarded == "" {
		return peer.String()
	}
	hops := strings.Split(forwarded, ",")
	if len(hops) > 16 {
		return peer.String()
	}
	original := peer
	for i := len(hops) - 1; i >= 0 && isTrusted(peer); i-- {
		ip, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return original.String()
		}
		peer = ip.Unmap()
	}
	return peer.String()
}
