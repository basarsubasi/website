package httpadapter

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFlyClientAddressTrustBoundary(t *testing.T) {
	for _, test := range []struct {
		name    string
		enabled bool
		remote  string
		headers []string
		want    string
	}{
		{"disabled", false, "172.19.1.1:80", []string{"192.0.2.1"}, "172.19.1.1"},
		{"proxy", true, "172.19.1.1:80", []string{"192.0.2.1"}, "192.0.2.1"},
		{"ipv6 proxy", true, "[fdaa::1]:80", []string{"2001:db8::2"}, "2001:db8::2"},
		{"mapped ipv4", true, "172.19.1.1:80", []string{"::ffff:192.0.2.1"}, "192.0.2.1"},
		{"public peer", true, "192.0.2.9:80", []string{"192.0.2.1"}, "192.0.2.9"},
		{"missing", true, "172.19.1.1:80", nil, "172.19.1.1"},
		{"invalid", true, "172.19.1.1:80", []string{"spoofed"}, "172.19.1.1"},
		{"list", true, "172.19.1.1:80", []string{"192.0.2.1, 192.0.2.2"}, "172.19.1.1"},
		{"duplicate", true, "172.19.1.1:80", []string{"192.0.2.1", "192.0.2.2"}, "172.19.1.1"},
		{"zone", true, "172.19.1.1:80", []string{"fe80::1%eth0"}, "172.19.1.1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &Server{cfg: Config{TrustFlyProxy: test.enabled}}
			r := httptest.NewRequest("POST", "/api/auth/login", nil)
			r.RemoteAddr = test.remote
			r.Header["Fly-Client-Ip"] = test.headers
			r.Header.Set("X-Forwarded-For", "203.0.113.99")
			if got := s.clientAddress(r); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestFlyClientsHaveSeparateLoginBudgets(t *testing.T) {
	s := &Server{cfg: Config{Password: testPassword, TrustFlyProxy: true}, sessions: make(map[[32]byte]time.Time)}
	for i := 0; i <= loginClientLimit; i++ {
		r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader("{"))
		r.RemoteAddr = "172.19.1.1:80"
		r.Header.Set("Fly-Client-IP", "192.0.2.1")
		w := httptest.NewRecorder()
		s.login(w, r)
		want := 400
		if i == loginClientLimit {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("attempt %d: got %d, want %d", i, w.Code, want)
		}
	}
	r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"`+testPassword+`"}`))
	r.RemoteAddr = "172.19.1.1:80"
	r.Header.Set("Fly-Client-IP", "192.0.2.2")
	w := httptest.NewRecorder()
	s.login(w, r)
	if w.Code != 200 {
		t.Fatalf("second Fly client blocked: %d", w.Code)
	}
}
