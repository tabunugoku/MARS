// Package config reads the process configuration from the environment.
// Settings an admin changes at runtime live in the database instead.
package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	// DataDir holds the database, thumbnails and the page cache.
	DataDir string
	// Addr is the listen address, e.g. "127.0.0.1:25600".
	Addr string
	// Dev serves no embedded UI (the Vite dev server proxies /api instead).
	Dev bool
}

const defaultPort = 25600

// Load reads COMINAVI_DATA, COMINAVI_HOST, COMINAVI_PORT and COMINAVI_DEV.
func Load() (Config, error) {
	c := Config{DataDir: os.Getenv("COMINAVI_DATA"), Dev: os.Getenv("COMINAVI_DEV") == "1"}
	if c.DataDir == "" {
		c.DataDir = "data"
	}
	abs, err := filepath.Abs(c.DataDir)
	if err != nil {
		return c, err
	}
	c.DataDir = abs

	port := defaultPort
	if p := os.Getenv("COMINAVI_PORT"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return c, fmt.Errorf("COMINAVI_PORT %q is not a port number", p)
		}
		port = n
	}
	// Until sign-in exists (milestone M2) the API is open to anyone who can
	// reach it, so listen on loopback unless told otherwise.
	host := os.Getenv("COMINAVI_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	c.Addr = net.JoinHostPort(host, strconv.Itoa(port))
	return c, nil
}

func (c Config) DBPath() string       { return filepath.Join(c.DataDir, "cominavi.db") }
func (c Config) ThumbnailDir() string { return filepath.Join(c.DataDir, "thumbnails") }
func (c Config) PageCacheDir() string { return filepath.Join(c.DataDir, "cache", "pages") }
