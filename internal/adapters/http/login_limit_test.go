package httpadapter

import (
	"fmt"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestLoginFloodDoesNotBlockAnotherClient(t *testing.T) {
	c := setup(t)
	for i := 0; i < 100; i++ {
		r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader("{"))
		r.RemoteAddr = "192.0.2.1:1234"
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i))
		w := httptest.NewRecorder()
		c.handler.ServeHTTP(w, r)
		want := 400
		if i >= loginClientLimit {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("attempt %d: got %d want %d", i, w.Code, want)
		}
	}
	r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader("{\"password\":\""+testPassword+"\"}"))
	r.RemoteAddr = "192.0.2.2:4321"
	w := httptest.NewRecorder()
	c.handler.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("legitimate login blocked: %d", w.Code)
	}
}

func TestLoginLimitsExpireAndBoundDistributedTraffic(t *testing.T) {
	var l loginLimiter
	now := time.Now()
	for i := 0; i < loginGlobalLimit; i++ {
		if !l.allow(fmt.Sprint(i), now) {
			t.Fatalf("early global rejection at %d", i)
		}
	}
	if l.allow("another", now) {
		t.Fatal("global budget not enforced")
	}
	if !l.allow("another", now.Add(time.Minute)) {
		t.Fatal("budget did not recover")
	}
	if len(l.clients) != 1 {
		t.Fatal("expired client identities retained")
	}
	var perClient loginLimiter
	for i := 0; i < loginClientLimit; i++ {
		perClient.allow("client", now)
	}
	if perClient.allow("client", now) {
		t.Fatal("client budget not enforced")
	}
	if !perClient.allow("client", now.Add(time.Minute)) {
		t.Fatal("client budget did not recover")
	}
}

func TestClientAddressTrustBoundary(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")}
	for _, test := range []struct{ remote, forwarded, want string }{
		{"192.0.2.1:80", "198.51.100.2", "192.0.2.1"},
		{"10.0.0.1:80", "198.51.100.2", "198.51.100.2"},
		{"10.0.0.1:80", "spoofed, 192.0.2.1, 10.0.0.2", "192.0.2.1"},
		{"10.0.0.1:80", "bad", "10.0.0.1"},
		{"[::ffff:192.0.2.1]:80", "", "192.0.2.1"},
	} {
		if got := clientAddress(test.remote, test.forwarded, trusted); got != test.want {
			t.Fatalf("%+v: got %s", test, got)
		}
	}
}

func TestTrustedProxyClientsHaveSeparateLoginBudgets(t *testing.T) {
	s := &Server{cfg: Config{Password: testPassword, TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.1/32")}}, sessions: make(map[[32]byte]time.Time)}
	for i := 0; i < loginClientLimit; i++ {
		r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader("{"))
		r.RemoteAddr = "10.0.0.1:80"
		r.Header.Set("X-Forwarded-For", "192.0.2.1")
		s.login(httptest.NewRecorder(), r)
	}
	r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader("{\"password\":\""+testPassword+"\"}"))
	r.RemoteAddr = "10.0.0.1:80"
	r.Header.Set("X-Forwarded-For", "192.0.2.2")
	w := httptest.NewRecorder()
	s.login(w, r)
	if w.Code != 200 {
		t.Fatalf("second proxy client blocked: %d", w.Code)
	}
}
