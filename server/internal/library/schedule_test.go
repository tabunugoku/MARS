package library

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/tabunugoku/mars/server/internal/db"
	"github.com/tabunugoku/mars/server/internal/task"
)

// TestScheduleStartupQueuesOnlyDueLibraries checks that starting Schedule
// does not scan every library. A library scanned within its interval is not
// queued, one whose interval has passed is, one never scanned is, and one with
// interval "off" is not.
func TestScheduleStartupQueuesOnlyDueLibraries(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ago := func(d time.Duration) string { return time.Now().UTC().Add(-d).Format(time.RFC3339Nano) }
	const created = "2026-01-01T00:00:00Z"
	libs := []struct {
		id, interval string
		last         any // nil = never scanned
	}{
		{"recent", "daily", ago(time.Hour)},       // scanned 1 h ago, daily: not due
		{"overdue", "hourly", ago(2 * time.Hour)}, // scanned 2 h ago, hourly: due
		{"fresh", "daily", nil},                   // never scanned: due
		{"manual", "off", nil},                    // interval off: not due
	}
	for _, l := range libs {
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO libraries (id, name, root, kind, scan_interval, watch, created_at, last_scan) VALUES (?, ?, ?, 'manga', ?, 0, ?, ?)`,
			l.id, l.id, t.TempDir(), l.interval, created, l.last); err != nil {
			t.Fatal(err)
		}
	}

	q := task.New(conn, 1) // not running: queued tasks stay in the table
	s := &Service{DB: conn, Queue: q, ThumbDir: t.TempDir()}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		s.Schedule(runCtx)
		close(done)
	}()

	queued := func() map[string]bool {
		tasks, err := q.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, tk := range tasks {
			if tk.Kind == "scan" {
				got[tk.LibraryID] = true
			}
		}
		return got
	}
	// The startup check runs before Schedule waits for its first tick. Wait
	// for the due libraries, then stop and wait for Schedule to return, so
	// the check has finished before the others are looked at.
	deadline := time.Now().Add(5 * time.Second)
	for {
		g := queued()
		if g["overdue"] && g["fresh"] {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("due libraries not queued at startup: %v", g)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done

	got := queued()
	if got["recent"] {
		t.Error("library scanned within its interval was queued at startup")
	}
	if got["manual"] {
		t.Error(`library with interval "off" was queued at startup`)
	}
	if !got["overdue"] || !got["fresh"] {
		t.Errorf("queued = %v, want overdue and fresh", got)
	}
}

// TestScheduleSkipsLibraryBeingScanned checks that a library whose scan is
// still running is not queued again by the scheduler, even though it has never
// finished a scan (the first scan of a new library takes longer than a tick).
func TestScheduleSkipsLibraryBeingScanned(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO libraries (id, name, root, kind, scan_interval, watch, created_at) VALUES ('new', 'new', ?, 'manga', 'daily', 0, '2026-01-01T00:00:00Z')`,
		t.TempDir()); err != nil {
		t.Fatal(err)
	}
	q := task.New(conn, 2)
	if err := q.Run(ctx); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: conn, Queue: q, ThumbDir: t.TempDir()}

	release := make(chan struct{})
	defer close(release)
	first, err := q.SubmitKey(ctx, "scan", "scan", task.Text{English: "first"}, "new", func(ctx context.Context, _ func(float64)) error {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		tasks, err := q.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) == 1 && tasks[0].ID == first.ID && tasks[0].State == "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("first scan not running: %+v", tasks)
		}
		time.Sleep(10 * time.Millisecond)
	}

	s.queueDue(ctx)

	tasks, err := q.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks after queueDue = %d, want only the running scan", len(tasks))
	}
}

// TestScheduleQueuesFullScanDuringWatcherScan checks that a watcher scan (key
// scan:watch, possibly only some folders) does not hold off a due full scan.
func TestScheduleQueuesFullScanDuringWatcherScan(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO libraries (id, name, root, kind, scan_interval, watch, created_at, last_scan) VALUES ('lib', 'lib', ?, 'manga', 'hourly', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		t.TempDir()); err != nil {
		t.Fatal(err)
	}
	q := task.New(conn, 2)
	if err := q.Run(ctx); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: conn, Queue: q, ThumbDir: t.TempDir()}

	release := make(chan struct{})
	defer close(release)
	watch, err := q.SubmitKey(ctx, "scan", "scan:watch", task.Text{English: "watch"}, "lib", func(ctx context.Context, _ func(float64)) error {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		tasks, err := q.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) == 1 && tasks[0].ID == watch.ID && tasks[0].State == "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("watcher scan not running: %+v", tasks)
		}
		time.Sleep(10 * time.Millisecond)
	}

	s.queueDue(ctx)

	tasks, err := q.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	full := 0
	for _, tk := range tasks {
		if tk.ID != watch.ID && tk.Kind == "scan" {
			full++
		}
	}
	if full != 1 {
		t.Fatalf("full scans queued during a watcher scan = %d, want 1 (tasks %+v)", full, tasks)
	}
}
