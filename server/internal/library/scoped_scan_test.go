package library

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/tabunugoku/mars/server/internal/db"
	"github.com/tabunugoku/mars/server/internal/task"
)

// scopeLog records the requests of a watcher's onQuiet.
type scopeLog struct {
	mu    sync.Mutex
	calls []scopeCall
}

type scopeCall struct {
	scope []string
	full  bool
}

func (l *scopeLog) add(scope []string, full bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, scopeCall{scope, full})
}

func (l *scopeLog) get() []scopeCall {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.calls)
}

// wait waits until n requests came in (or 5 s pass) and fails when they did not.
func (l *scopeLog) wait(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for len(l.get()) < n && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := len(l.get()); got < n {
		t.Fatalf("%d scan requests, want %d", got, n)
	}
}

// A change in one top-level folder, or to a root-level file, asks for a scan
// of just that entry.
func TestWatchChangeScopesScanToItsEntry(t *testing.T) {
	old := watchDelay
	watchDelay = 200 * time.Millisecond
	defer func() { watchDelay = old }()

	root := t.TempDir()
	testMkdir(t, root, "A")
	testMkdir(t, root, "B")
	var log scopeLog
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, done, err := watchTree(ctx, root, nil, log.add, func(err error) { t.Errorf("watcher failed: %v", err) })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { stop(); <-done }()

	testWriteFile(t, root, "A/x.cbz")
	log.wait(t, 1)
	time.Sleep(4 * watchDelay)
	if got := log.get(); len(got) != 1 || !slices.Equal(got[0].scope, []string{"A"}) || got[0].full {
		t.Fatalf("change in A: %+v, want scope [A]", got)
	}

	testWriteFile(t, root, "B/y.cbz")
	testWriteFile(t, root, "loose.cbz")
	log.wait(t, 2)
	time.Sleep(4 * watchDelay)
	got := log.get()
	if len(got) != 2 || !slices.Equal(got[1].scope, []string{"B", "loose.cbz"}) || got[1].full {
		t.Fatalf("changes in B and a root file: %+v, want scope [B loose.cbz]", got)
	}
}

// More than maxScopeEntries changed folders ask for a full scan.
func TestWatchManyFoldersScanFully(t *testing.T) {
	old := watchDelay
	watchDelay = 500 * time.Millisecond
	defer func() { watchDelay = old }()

	root := t.TempDir()
	n := maxScopeEntries + 1
	for i := 0; i < n; i++ {
		testMkdir(t, root, fmt.Sprintf("F%02d", i))
	}
	var log scopeLog
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, done, err := watchTree(ctx, root, nil, log.add, func(err error) { t.Errorf("watcher failed: %v", err) })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { stop(); <-done }()

	for i := 0; i < n; i++ {
		testWriteFile(t, root, fmt.Sprintf("F%02d/x.cbz", i))
	}
	log.wait(t, 1)
	time.Sleep(2 * watchDelay)
	if got := log.get(); len(got) != 1 || !got[0].full || got[0].scope != nil {
		t.Fatalf("%d changed folders: %+v, want one full scan", n, got)
	}
}

func TestDirtyScopeTake(t *testing.T) {
	var d dirtyScope
	d.add("b")
	d.add("a")
	d.add("a")
	if scope, full := d.take(); !slices.Equal(scope, []string{"a", "b"}) || full {
		t.Fatalf("take = %v, %v; want [a b], false", scope, full)
	}
	if scope, full := d.take(); len(scope) != 0 || full {
		t.Fatalf("second take = %v, %v; want empty", scope, full)
	}

	for i := 0; i <= maxScopeEntries; i++ {
		d.add(fmt.Sprint("e", i))
	}
	if scope, full := d.take(); scope != nil || !full {
		t.Fatalf("over the limit: %v, %v; want nil, true", scope, full)
	}

	d.add("x")
	d.markFull()
	d.add("y") // ignored once full
	if scope, full := d.take(); scope != nil || !full {
		t.Fatalf("marked full: %v, %v; want nil, true", scope, full)
	}
}

// Requests for one library merge into one pending set until a scan takes it.
func TestWatchPendingMerges(t *testing.T) {
	s := &Service{}
	s.addWatchDirty("L", []string{"B", "A"}, false)
	s.addWatchDirty("L", []string{"A", "C"}, false)
	if scope, full := s.takeWatchDirty("L"); !slices.Equal(scope, []string{"A", "B", "C"}) || full {
		t.Fatalf("merged take = %v, %v", scope, full)
	}
	if scope, full := s.takeWatchDirty("L"); scope != nil || !full {
		t.Fatalf("empty pending = %v, %v; want a full scan", scope, full)
	}

	s.addWatchDirty("L", []string{"A"}, false)
	s.addWatchDirty("L", nil, true)
	if scope, full := s.takeWatchDirty("L"); scope != nil || !full {
		t.Fatalf("full mark = %v, %v", scope, full)
	}
}

// A scan task started for a watcher scans only the pending folders: a book
// added in the changed folder appears, and books in other folders, deleted
// from disk without an event, are not touched because the scan never looked
// there.
func TestWatchScopedScanLeavesOtherFoldersAlone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer conn.Close()
	defer cancel()

	root := t.TempDir()
	for _, d := range []string{"A", "B", "C"} {
		testMkdir(t, root, d)
		testWriteFile(t, root, d+"/"+d+"1.cbz")
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO libraries (id, name, root, kind, watch, created_at) VALUES ('lib1', 'L', ?, 'comics', 1, '2026-01-01T00:00:00Z')`,
		root); err != nil {
		t.Fatal(err)
	}
	q := task.New(conn, 1)
	if err := q.Run(ctx); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: conn, Queue: q, ThumbDir: t.TempDir()}
	noProgress := func(float64) {}

	if err := s.scanLibrary(ctx, "lib1", nil, false, noProgress); err != nil {
		t.Fatal(err)
	}

	// Changes on disk: a new book in A; B's and C's books vanish with no event.
	testWriteFile(t, root, "A/A2.cbz")
	for _, p := range []string{"B/B1.cbz", "C/C1.cbz"} {
		if err := os.Remove(filepath.Join(root, p)); err != nil {
			t.Fatal(err)
		}
	}
	s.addWatchDirty("lib1", []string{"A"}, false)
	tk, err := s.queueWatchTask(ctx, "lib1")
	if err != nil {
		t.Fatal(err)
	}
	if tk.LabelCode != "task.scan_partial" {
		t.Fatalf("label code %q, want task.scan_partial", tk.LabelCode)
	}
	waitTaskFinished(t, conn, tk.ID)

	for path, want := range map[string]int{"A/A2.cbz": 1, "A/A1.cbz": 1, "B/B1.cbz": 1, "C/C1.cbz": 1} {
		if n := liveBooks(t, conn, path); n != want {
			t.Fatalf("%s: %d live rows, want %d", path, n, want)
		}
	}
}

// waitTaskFinished waits until the task leaves the queue (or 10 s pass).
func waitTaskFinished(t *testing.T, conn *sql.DB, id string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var state, errText string
		if err := conn.QueryRow(`SELECT state, error FROM tasks WHERE id = ?`, id).Scan(&state, &errText); err != nil {
			t.Fatal(err)
		}
		switch state {
		case "done":
			return
		case "failed", "cancelled":
			t.Fatalf("task %s ended %s: %s", id, state, errText)
		}
		if time.Now().After(deadline) {
			t.Fatalf("task %s still %s", id, state)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func liveBooks(t *testing.T, conn *sql.DB, path string) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM books WHERE path = ? AND deleted_at IS NULL`, path).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
