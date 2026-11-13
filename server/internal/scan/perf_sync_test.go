//go:build perf

package scan

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tabunugoku/mars/server/internal/db"
)

// TestPerfSync runs a first Sync of PERF_LIB into a fresh database with a
// write-lock probe, logging every wait over 100ms.
func TestPerfSync(t *testing.T) {
	root := os.Getenv("PERF_LIB")
	if root == "" {
		t.Skip("PERF_LIB not set")
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "t.db")
	d, err := db.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.Exec(`INSERT INTO libraries (id, name, root, kind, created_at) VALUES ('L', 'L', ?, 'manga', '')`, root)
	found, err := Walk(ctx, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	probe, _ := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=busy_timeout(3600000)")
	defer probe.Close()
	done := make(chan struct{})
	go func() {
		conn, _ := probe.Conn(ctx)
		defer conn.Close()
		for {
			select {
			case <-done:
				return
			default:
			}
			t0 := time.Now()
			conn.ExecContext(ctx, "BEGIN IMMEDIATE")
			conn.ExecContext(ctx, "ROLLBACK")
			if el := time.Since(t0); el > 100*time.Millisecond {
				t.Logf("probe waited %s until %s", el.Round(time.Millisecond), time.Now().Format("15:04:05.000"))
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	for i := range 2 {
		t0 := time.Now()
		st, err := Sync(ctx, d, "L", "manga", found, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("sync %d: %s %+v", i, time.Since(t0).Round(time.Millisecond), st)
	}
	close(done)
}
