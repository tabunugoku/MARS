package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/db"
)

func testWriteFile(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, rel), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testMkdir(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
		t.Fatal(err)
	}
}

// waitScans waits until n reaches want (or 5 s pass).
func waitScans(n *atomic.Int32, want int32) {
	deadline := time.Now().Add(5 * time.Second)
	for n.Load() < want && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWatchTreeScans(t *testing.T) {
	old := watchDelay
	watchDelay = 200 * time.Millisecond
	defer func() { watchDelay = old }()

	root := t.TempDir()
	testMkdir(t, root, "a")
	var scans atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, done, err := watchTree(ctx, root, []string{"skipme"},
		func() { scans.Add(1) },
		func(err error) { t.Errorf("watcher failed: %v", err) })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { stop(); <-done }()

	// Excluded and system folders trigger nothing.
	testMkdir(t, root, "@eaDir")
	testWriteFile(t, root, "@eaDir/x.cbz")
	testMkdir(t, root, "skipme")
	testWriteFile(t, root, "skipme/y.cbz")
	time.Sleep(4 * watchDelay)
	if n := scans.Load(); n != 0 {
		t.Fatalf("excluded folders triggered %d scans, want 0", n)
	}

	// A nested new folder gets watched: a book in it triggers exactly one scan.
	testMkdir(t, root, "a/new/deep")
	time.Sleep(4 * watchDelay) // folder events settle (and their scan fires)
	scans.Store(0)
	testWriteFile(t, root, "a/new/deep/z.cbz")
	waitScans(&scans, 1)
	time.Sleep(4 * watchDelay)
	if n := scans.Load(); n != 1 {
		t.Fatalf("one nested book: %d scans, want 1", n)
	}

	// A file that is not a book does not count.
	testWriteFile(t, root, "a/new/deep/cover.jpg")
	time.Sleep(4 * watchDelay)
	if n := scans.Load(); n != 1 {
		t.Fatalf("cover.jpg triggered a scan: %d, want 1", n)
	}

	// A burst of books within the delay becomes one more scan.
	for i := 0; i < 20; i++ {
		testWriteFile(t, root, fmt.Sprintf("a/new/deep/b%02d.cbz", i))
	}
	waitScans(&scans, 2)
	time.Sleep(4 * watchDelay)
	if n := scans.Load(); n != 2 {
		t.Fatalf("burst: %d scans, want 2", n)
	}
}

func TestWatchTreeMissingRoot(t *testing.T) {
	ctx := context.Background()
	cancel, done, err := watchTree(ctx, filepath.Join(t.TempDir(), "gone"), nil,
		func() {}, func(error) {})
	if err == nil {
		t.Fatal("watching a missing folder succeeded")
	}
	if cancel != nil || done != nil {
		t.Fatal("a failed start returned a handle")
	}
	if msg := watchErrMessage(err); msg != watchMsgGeneric {
		t.Errorf("message = %q, want the generic one", msg)
	}
}

func TestWatchErrMessageLimit(t *testing.T) {
	if msg := watchErrMessage(syscall.ENOSPC); msg != watchMsgLimit {
		t.Errorf("ENOSPC message = %q, want the limit message", msg)
	}
}

func TestWatchErrorEmptyWhenNotWatching(t *testing.T) {
	var s Service
	if msg := s.WatchError("no-such-library"); msg != "" {
		t.Errorf("WatchError = %q, want empty", msg)
	}
}

func TestWatchFailedStartBackoff(t *testing.T) {
	oldFirst, oldMax := watchRetryFirst, watchRetryMax
	watchRetryFirst, watchRetryMax = 300*time.Millisecond, 2*time.Second
	defer func() { watchRetryFirst, watchRetryMax = oldFirst, oldMax }()

	ctx := context.Background()
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	missing := filepath.Join(t.TempDir(), "gone")
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO libraries (id, name, root, kind, watch, created_at) VALUES ('lib1', 'L', ?, 'manga', 1, '2026-01-01T00:00:00Z')`,
		missing); err != nil {
		t.Fatal(err)
	}

	var starts atomic.Int32
	watchStartHook = func(id string) {
		if id == "lib1" {
			starts.Add(1)
		}
	}
	defer func() { watchStartHook = nil }()

	s := &Service{DB: conn}
	ws := s.watches()
	check := func(step string, want int32) {
		t.Helper()
		if n := starts.Load(); n != want {
			t.Fatalf("%s: %d start attempts, want %d", step, n, want)
		}
	}

	s.reconcileWatches(ctx, ws)
	check("first pass", 1)
	if msg := s.WatchError("lib1"); msg != watchMsgGeneric {
		t.Fatalf("WatchError = %q, want the generic message", msg)
	}

	// Within the back-off the start is not retried, and the message stays.
	s.reconcileWatches(ctx, ws)
	check("within back-off", 1)
	if msg := s.WatchError("lib1"); msg != watchMsgGeneric {
		t.Fatalf("WatchError during back-off = %q, want the generic message", msg)
	}

	// After the back-off it is retried.
	time.Sleep(watchRetryFirst + 100*time.Millisecond)
	s.reconcileWatches(ctx, ws)
	check("after back-off", 2)

	// A config change retries at once, even inside the (now doubled) back-off.
	if _, err := conn.ExecContext(ctx, `UPDATE libraries SET exclude = '["x"]' WHERE id = 'lib1'`); err != nil {
		t.Fatal(err)
	}
	s.reconcileWatches(ctx, ws)
	check("after config change", 3)
}
