package httpadapter

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	disabledCase                   = "disabled"
	proxyAddr                      = "172.19.1.1:80"
	firstClientIP                  = "192.0.2.1"
	proxyIP                        = "172.19.1.1"
	proxyCase                      = "proxy"
	ipv6ProxyCase                  = "ipv6 proxy"
	ipv6ProxyAddr                  = "[fdaa::1]:80"
	ipv6ClientIP                   = "2001:db8::2"
	mappedIPv4Case                 = "mapped ipv4"
	mappedClientIP                 = "::ffff:192.0.2.1"
	publicPeerCase                 = "public peer"
	proxyTestPublicPeerAddr        = "192.0.2.9:80"
	proxyTestPublicPeerIP          = "192.0.2.9"
	missingHeaderCase              = "missing"
	invalidHeaderCase              = "invalid"
	spoofedHeader                  = "spoofed"
	listHeaderCase                 = "list"
	clientIPList                   = "192.0.2.1, 192.0.2.2"
	duplicateHeaderCase            = "duplicate"
	proxyTestSecondClientIP        = "192.0.2.2"
	zoneCase                       = "zone"
	zonedAddress                   = "fe80::1%eth0"
	proxyTestPostMethod            = "POST"
	proxyTestLoginPath             = "/api/auth/login"
	canonicalFlyHeader             = "Fly-Client-Ip"
	proxyTestForwardedForHeader    = "X-Forwarded-For"
	spoofedClientIP                = "203.0.113.99"
	proxyTestAddressMismatchFormat = "got %q, want %q"

	proxyTestMalformedJSON             = "{"
	flyHeader                          = "Fly-Client-IP"
	proxyTestLoginStatusFormat         = "attempt %d: got %d, want %d"
	proxyTestPasswordJSONPrefix        = `{"password":"`
	proxyTestPasswordJSONSuffix        = `"}`
	proxyTestSecondClientFailureFormat = "second Fly client blocked: %d"
)

func TestFlyClientAddressTrustBoundary(t *testing.T) {
	for _, test := range []struct {
		name    string
		enabled bool
		remote  string
		headers []string
		want    string
	}{
		{disabledCase, false, proxyAddr, []string{firstClientIP}, proxyIP},
		{proxyCase, true, proxyAddr, []string{firstClientIP}, firstClientIP},
		{ipv6ProxyCase, true, ipv6ProxyAddr, []string{ipv6ClientIP}, ipv6ClientIP},
		{mappedIPv4Case, true, proxyAddr, []string{mappedClientIP}, firstClientIP},
		{publicPeerCase, true, proxyTestPublicPeerAddr, []string{firstClientIP}, proxyTestPublicPeerIP},
		{missingHeaderCase, true, proxyAddr, nil, proxyIP},
		{invalidHeaderCase, true, proxyAddr, []string{spoofedHeader}, proxyIP},
		{listHeaderCase, true, proxyAddr, []string{clientIPList}, proxyIP},
		{duplicateHeaderCase, true, proxyAddr, []string{firstClientIP, proxyTestSecondClientIP}, proxyIP},
		{zoneCase, true, proxyAddr, []string{zonedAddress}, proxyIP},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &Server{cfg: Config{TrustFlyProxy: test.enabled}}
			r := httptest.NewRequest(proxyTestPostMethod, proxyTestLoginPath, nil)
			r.RemoteAddr = test.remote
			r.Header[canonicalFlyHeader] = test.headers
			r.Header.Set(proxyTestForwardedForHeader, spoofedClientIP)
			if got := s.clientAddress(r); got != test.want {
				t.Fatalf(proxyTestAddressMismatchFormat, got, test.want)
			}
		})
	}
}

func TestFlyClientsHaveSeparateLoginBudgets(t *testing.T) {
	s := &Server{cfg: Config{Password: testPassword, TrustFlyProxy: true}, sessions: make(map[[32]byte]time.Time)}
	for i := 0; i <= loginClientLimit; i++ {
		r := httptest.NewRequest(proxyTestPostMethod, proxyTestLoginPath, strings.NewReader(proxyTestMalformedJSON))
		r.RemoteAddr = proxyAddr
		r.Header.Set(flyHeader, firstClientIP)
		w := httptest.NewRecorder()
		s.login(w, r)
		want := 400
		if i == loginClientLimit {
			want = 429
		}
		if w.Code != want {
			t.Fatalf(proxyTestLoginStatusFormat, i, w.Code, want)
		}
	}
	r := httptest.NewRequest(proxyTestPostMethod, proxyTestLoginPath, strings.NewReader(proxyTestPasswordJSONPrefix+testPassword+proxyTestPasswordJSONSuffix))
	r.RemoteAddr = proxyAddr
	r.Header.Set(flyHeader, proxyTestSecondClientIP)
	w := httptest.NewRecorder()
	s.login(w, r)
	if w.Code != 200 {
		t.Fatalf(proxyTestSecondClientFailureFormat, w.Code)
	}
}
