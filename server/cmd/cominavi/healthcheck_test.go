package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHealthcheck(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	if err := healthcheck(context.Background(), addr); err != nil {
		t.Fatalf("healthy server: %v", err)
	}
	status = http.StatusInternalServerError
	if err := healthcheck(context.Background(), addr); err == nil {
		t.Fatal("500 counted as healthy")
	}
	srv.Close()
	if err := healthcheck(context.Background(), addr); err == nil {
		t.Fatal("stopped server counted as healthy")
	}
}

// A scoped IPv6 listen address must reach the dialer: before the URL was
// built with url.URL, "%eth0" failed to parse and every probe failed.
func TestHealthcheckScopedIPv6(t *testing.T) {
	err := healthcheck(context.Background(), "[fe80::1%eth0]:1")
	var ue *url.Error
	if !errors.As(err, &ue) || ue.Op == "parse" {
		t.Fatalf("want a dial error, got %v", err)
	}
}

func TestProbeAddr(t *testing.T) {
	for in, want := range map[string]string{
		":25600":          "127.0.0.1:25600",
		"0.0.0.0:25600":   "127.0.0.1:25600",
		"[::]:25600":      "127.0.0.1:25600",
		"192.0.2.7:8080":  "192.0.2.7:8080",
		"localhost:25600": "localhost:25600",
	} {
		if got := probeAddr(in); got != want {
			t.Errorf("probeAddr(%q) = %q, want %q", in, got, want)
		}
	}
}
