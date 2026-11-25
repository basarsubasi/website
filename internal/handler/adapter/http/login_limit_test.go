package httpadapter

import (
	"fmt"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

const (
	loginLimitTestPostMethod         = "POST"
	loginLimitTestLoginPath          = "/api/auth/login"
	loginLimitTestMalformedJSON      = "{"
	firstClientAddr                  = "192.0.2.1:1234"
	loginLimitTestForwardedForHeader = "X-Forwarded-For"
	forwardedIPFormat                = "198.51.100.%d"
	loginLimitTestLoginStatusFormat  = "attempt %d: got %d want %d"
	loginLimitTestPasswordJSONPrefix = "{\"password\":\""
	loginLimitTestPasswordJSONSuffix = "\"}"
	secondClientAddr                 = "192.0.2.2:4321"
	legitimateLoginFormat            = "legitimate login blocked: %d"

	earlyGlobalRejectionFormat = "early global rejection at %d"
	otherClient                = "another"
	globalLimitFailure         = "global budget not enforced"
	recoveryFailure            = "budget did not recover"
	expiredClientFailure       = "expired client identities retained"
	clientName                 = "client"
	clientLimitFailure         = "client budget not enforced"
	clientRecoveryFailure      = "client budget did not recover"

	proxyNetwork                        = "10.0.0.0/24"
	loginLimitTestPublicPeerAddr        = "192.0.2.1:80"
	forwardedIP                         = "198.51.100.2"
	loginLimitTestPublicPeerIP          = "192.0.2.1"
	trustedPeerAddr                     = "10.0.0.1:80"
	forwardedChain                      = "spoofed, 192.0.2.1, 10.0.0.2"
	invalidIP                           = "bad"
	trustedPeerIP                       = "10.0.0.1"
	mappedPublicPeerAddr                = "[::ffff:192.0.2.1]:80"
	emptyValue                          = ""
	loginLimitTestAddressMismatchFormat = "%+v: got %s"

	trustedHostCIDR                         = "10.0.0.1/32"
	loginLimitTestSecondClientIP            = "192.0.2.2"
	loginLimitTestSecondClientFailureFormat = "second proxy client blocked: %d"
)

func TestLoginFloodDoesNotBlockAnotherClient(t *testing.T) {
	c := setup(t)
	for i := 0; i < 100; i++ {
		r := httptest.NewRequest(loginLimitTestPostMethod, loginLimitTestLoginPath, strings.NewReader(loginLimitTestMalformedJSON))
		r.RemoteAddr = firstClientAddr
		r.Header.Set(loginLimitTestForwardedForHeader, fmt.Sprintf(forwardedIPFormat, i))
		w := httptest.NewRecorder()
		c.handler.ServeHTTP(w, r)
		want := 400
		if i >= loginClientLimit {
			want = 429
		}
		if w.Code != want {
			t.Fatalf(loginLimitTestLoginStatusFormat, i, w.Code, want)
		}
	}
	r := httptest.NewRequest(loginLimitTestPostMethod, loginLimitTestLoginPath, strings.NewReader(loginLimitTestPasswordJSONPrefix+testPassword+loginLimitTestPasswordJSONSuffix))
	r.RemoteAddr = secondClientAddr
	w := httptest.NewRecorder()
	c.handler.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf(legitimateLoginFormat, w.Code)
	}
}

func TestLoginLimitsExpireAndBoundDistributedTraffic(t *testing.T) {
	var l loginLimiter
	now := time.Now()
	for i := 0; i < loginGlobalLimit; i++ {
		if !l.allow(fmt.Sprint(i), now) {
			t.Fatalf(earlyGlobalRejectionFormat, i)
		}
	}
	if l.allow(otherClient, now) {
		t.Fatal(globalLimitFailure)
	}
	if !l.allow(otherClient, now.Add(time.Minute)) {
		t.Fatal(recoveryFailure)
	}
	if len(l.clients) != 1 {
		t.Fatal(expiredClientFailure)
	}
	var perClient loginLimiter
	for i := 0; i < loginClientLimit; i++ {
		perClient.allow(clientName, now)
	}
	if perClient.allow(clientName, now) {
		t.Fatal(clientLimitFailure)
	}
	if !perClient.allow(clientName, now.Add(time.Minute)) {
		t.Fatal(clientRecoveryFailure)
	}
}

func TestClientAddressTrustBoundary(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix(proxyNetwork)}
	for _, test := range []struct{ remote, forwarded, want string }{
		{loginLimitTestPublicPeerAddr, forwardedIP, loginLimitTestPublicPeerIP},
		{trustedPeerAddr, forwardedIP, forwardedIP},
		{trustedPeerAddr, forwardedChain, loginLimitTestPublicPeerIP},
		{trustedPeerAddr, invalidIP, trustedPeerIP},
		{mappedPublicPeerAddr, emptyValue, loginLimitTestPublicPeerIP},
	} {
		if got := clientAddress(test.remote, test.forwarded, trusted); got != test.want {
			t.Fatalf(loginLimitTestAddressMismatchFormat, test, got)
		}
	}
}

func TestTrustedProxyClientsHaveSeparateLoginBudgets(t *testing.T) {
	s := &Server{cfg: Config{Password: testPassword, TrustedProxies: []netip.Prefix{netip.MustParsePrefix(trustedHostCIDR)}}, sessions: make(map[[32]byte]time.Time)}
	for i := 0; i < loginClientLimit; i++ {
		r := httptest.NewRequest(loginLimitTestPostMethod, loginLimitTestLoginPath, strings.NewReader(loginLimitTestMalformedJSON))
		r.RemoteAddr = trustedPeerAddr
		r.Header.Set(loginLimitTestForwardedForHeader, loginLimitTestPublicPeerIP)
		s.login(httptest.NewRecorder(), r)
	}
	r := httptest.NewRequest(loginLimitTestPostMethod, loginLimitTestLoginPath, strings.NewReader(loginLimitTestPasswordJSONPrefix+testPassword+loginLimitTestPasswordJSONSuffix))
	r.RemoteAddr = trustedPeerAddr
	r.Header.Set(loginLimitTestForwardedForHeader, loginLimitTestSecondClientIP)
	w := httptest.NewRecorder()
	s.login(w, r)
	if w.Code != 200 {
		t.Fatalf(loginLimitTestSecondClientFailureFormat, w.Code)
	}
}
