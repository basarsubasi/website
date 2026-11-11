package httpadapter

import (
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"
)

const loginClientLimit = 20
const loginGlobalLimit = 200
const loginClientCapacity = 4096

type loginWindow struct {
	start time.Time
	count int
}
type loginLimiter struct {
	mu      sync.Mutex
	clients map[string]loginWindow
	global  loginWindow
}

func (l *loginLimiter) allow(remote string, now time.Time) bool {
	// The caller supplies an identity resolved through the trusted proxy chain.
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.clients == nil {
		l.clients = make(map[string]loginWindow)
	}
	if now.Sub(l.global.start) >= time.Minute {
		l.global = loginWindow{start: now}
		for key, window := range l.clients {
			if now.Sub(window.start) >= time.Minute {
				delete(l.clients, key)
			}
		}
	}
	client, exists := l.clients[remote]
	if now.Sub(client.start) >= time.Minute {
		client = loginWindow{start: now}
	}
	// Rejected traffic from one client never consumes the global allowance.
	if client.count >= loginClientLimit || l.global.count >= loginGlobalLimit ||
		(!exists && len(l.clients) >= loginClientCapacity) {
		return false
	}
	client.count++
	l.clients[remote] = client
	l.global.count++
	return true
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
