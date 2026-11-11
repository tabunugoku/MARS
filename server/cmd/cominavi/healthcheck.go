package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// healthcheck asks a running server for /api/v1/healthz and fails unless it
// answers 200. It is the image's health probe ("cominavi healthcheck"): the
// image has no shell or curl, so the binary checks itself. It opens neither
// the data directory nor the database.
func healthcheck(ctx context.Context, addr string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+probeAddr(addr)+"/api/v1/healthz", nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
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
