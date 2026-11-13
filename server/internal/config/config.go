// Package config reads the process configuration from the environment.
// Settings an admin changes at runtime live in the database instead.
package config

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	// DataDir holds the database, thumbnails and the page cache.
	DataDir string
	// Addr is the listen address, e.g. ":25600" (all interfaces) or "127.0.0.1:25600".
	Addr string
	// Dev serves no embedded UI (the Vite dev server proxies /api instead).
	Dev bool
	// TrustedProxies are reverse proxies (MARS_TRUSTED_PROXIES, comma-
	// separated IPs or CIDRs) whose X-Forwarded-For names the real client.
	TrustedProxies []netip.Prefix
	// PortSet reports whether MARS_PORT chose the port. When false the
	// port comes from the stored server setting (see AddrWithPort).
	PortSet bool

	host string
}

// AddrWithPort returns the listen address on the configured host with port.
func (c Config) AddrWithPort(port int) string {
	return net.JoinHostPort(c.host, strconv.Itoa(port))
}

const defaultPort = 25600

// Load reads MARS_DATA, MARS_HOST, MARS_PORT, MARS_DEV and MARS_TRUSTED_PROXIES.
func Load() (Config, error) {
	c := Config{DataDir: os.Getenv("MARS_DATA"), Dev: os.Getenv("MARS_DEV") == "1"}
	if c.DataDir == "" {
		c.DataDir = "data"
	}
	abs, err := filepath.Abs(c.DataDir)
	if err != nil {
		return c, err
	}
	c.DataDir = abs

	port := defaultPort
	if p := os.Getenv("MARS_PORT"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return c, fmt.Errorf("MARS_PORT %q is not a port number", p)
		}
		port = n
		c.PortSet = true
	}
	// Listen on all interfaces unless MARS_HOST narrows it; every API
	// route except sign-in, setup and the login screen needs a session.
	c.host = os.Getenv("MARS_HOST") // "" means all interfaces; sign-in protects the API
	c.Addr = c.AddrWithPort(port)
	for _, f := range strings.Split(os.Getenv("MARS_TRUSTED_PROXIES"), ",") {
		if f = strings.TrimSpace(f); f == "" {
			continue
		}
		p, err := netip.ParsePrefix(f)
		if err != nil {
			a, aerr := netip.ParseAddr(f)
			if aerr != nil {
				return c, fmt.Errorf("MARS_TRUSTED_PROXIES: %q is not an IP or CIDR", f)
			}
			p = netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen())
		}
		c.TrustedProxies = append(c.TrustedProxies, p.Masked())
	}
	return c, nil
}

func (c Config) DBPath() string       { return filepath.Join(c.DataDir, "mars.db") }
func (c Config) ThumbnailDir() string { return filepath.Join(c.DataDir, "thumbnails") }
func (c Config) PageCacheDir() string { return filepath.Join(c.DataDir, "cache", "pages") }
func (c Config) PageThumbDir() string { return filepath.Join(c.DataDir, "cache", "page-thumbs") }

// ListenAddrFile holds the address the running server listens on. The server
// writes it once it is listening, so the healthcheck can find a port that
// only the stored settings know.
func (c Config) ListenAddrFile() string { return filepath.Join(c.DataDir, "listen-addr") }
