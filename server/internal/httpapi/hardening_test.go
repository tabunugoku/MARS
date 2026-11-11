package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestBodyCap(t *testing.T) {
	e := newEnv(t)
	big := `{"currentPassword":"` + strings.Repeat("a", 2<<20) + `","newPassword":"x"}`
	req := httptest.NewRequest("PUT", "/api/v1/me/password", bytes.NewReader([]byte(big)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+req.Host)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: e.admin.cookie})
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("2 MiB body: %d", rec.Code)
	}
}

// Wrong setup tokens must not lock out the operator's correct one.
func TestSetupNotLockedOut(t *testing.T) {
	e := newEnv(t)
	e.gate.token = "right"
	defer func() { e.gate.token = "" }()
	for range 10 {
		if rec := e.anon().do("POST", "/api/v1/setup", map[string]any{"token": "wrong", "name": "x", "email": "x@example.com", "password": "12345678"}); rec.Code != 403 {
			t.Fatalf("wrong token: %d", rec.Code)
		}
	}
	rec := e.anon().do("POST", "/api/v1/setup", map[string]any{"token": "right", "name": "二人目", "email": "x@example.com", "password": "12345678"})
	if rec.Code != 201 {
		t.Fatalf("right token after failures: %d %s", rec.Code, rec.Body)
	}
}

func TestClientIP(t *testing.T) {
	s := &server{Deps: Deps{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}}}
	for _, c := range []struct{ remote, xff, want string }{
		{"203.0.113.9:1234", "", "203.0.113.9"},
		{"203.0.113.9:1234", "1.2.3.4", "203.0.113.9"},             // untrusted peer: header ignored
		{"10.0.0.2:1234", "1.2.3.4", "1.2.3.4"},                    // via the proxy
		{"10.0.0.2:1234", "6.6.6.6, 1.2.3.4, 10.0.0.3", "1.2.3.4"}, // spoofed left entry ignored
		{"10.0.0.2:1234", "", "10.0.0.2"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := s.clientIP(r); got != c.want {
			t.Errorf("%s %q: %s, want %s", c.remote, c.xff, got, c.want)
		}
	}
}
