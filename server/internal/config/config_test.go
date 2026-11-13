package config

import "testing"

func TestPortSet(t *testing.T) {
	t.Setenv("MARS_DATA", t.TempDir())
	t.Setenv("MARS_HOST", "127.0.0.1")

	t.Setenv("MARS_PORT", "")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.PortSet {
		t.Error("PortSet = true with MARS_PORT unset")
	}
	if c.Addr != "127.0.0.1:25600" {
		t.Errorf("Addr = %q, want 127.0.0.1:25600", c.Addr)
	}
	if got := c.AddrWithPort(8080); got != "127.0.0.1:8080" {
		t.Errorf("AddrWithPort(8080) = %q, want 127.0.0.1:8080", got)
	}

	t.Setenv("MARS_PORT", "9000")
	c, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.PortSet || c.Addr != "127.0.0.1:9000" {
		t.Errorf("PortSet = %v, Addr = %q; want true, 127.0.0.1:9000", c.PortSet, c.Addr)
	}
}
