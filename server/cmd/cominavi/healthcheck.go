package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// healthcheck asks a running server for /api/v1/healthz and fails unless it
// answers 200. It is the image's health probe ("cominavi healthcheck"): the
// image has no shell or curl, so the binary checks itself. It opens neither
// the data directory nor the database.
func healthcheck(ctx context.Context, addr string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	// url.URL escapes an IPv6 zone ("fe80::1%eth0" becomes "%25eth0"), which
	// a concatenated string would leave for the URL parser to reject.
	u := url.URL{Scheme: "http", Host: probeAddr(addr), Path: "/api/v1/healthz"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	// The probe is local: never route it through HTTP_PROXY.
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz answered %s", res.Status)
	}
	return nil
}

// probeAddr turns the listen address into one to dial: a server listening
// on all interfaces ("" / 0.0.0.0 / ::) is reached on loopback.
func probeAddr(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return listen
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
