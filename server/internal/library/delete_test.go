package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tabunugoku/mars/server/internal/db"
	"github.com/tabunugoku/mars/server/internal/task"
)

const deleteTestTime = "2026-01-01T00:00:00Z"

func openDeleteTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if _, err := conn.Exec(`INSERT INTO users (id, name, email, password_hash, role, created_at) VALUES ('u1', 'reader', 'reader@example.com', 'x', 'admin', ?)`, deleteTestTime); err != nil {
		t.Fatal(err)
	}
	return conn
}

// seedDeleteLibrary inserts a library (deleting as given) with series series,
// each holding booksEach books of two pages, with their search rows and one
// reading progress row per book.
func seedDeleteLibrary(t *testing.T, conn *sql.DB, id string, deleting, series, booksEach int) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO libraries (id, name, root, kind, scan_interval, watch, created_at, deleting) VALUES (?, ?, ?, 'manga', 'off', 0, ?, ?)`,
		id, id, t.TempDir(), deleteTestTime, deleting)
	for s := range series {
		sid := fmt.Sprintf("%s-s%d", id, s)
		exec(`INSERT INTO series (id, library_id, dir, title, title_sort, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			sid, id, sid, sid, sid, deleteTestTime, deleteTestTime)
		exec(`INSERT INTO search_rows (kind, ref, library_id, text) VALUES ('series', ?, ?, ?)`, sid, id, sid)
		for b := range booksEach {
			bid := fmt.Sprintf("%s-b%d-%d", id, s, b)
			exec(`INSERT INTO books (id, series_id, library_id, path, format, size_bytes, mtime, title, number, number_sort, created_at, updated_at)
				VALUES (?, ?, ?, ?, 'cbz', 1, ?, ?, 1, 1, ?, ?)`, bid, sid, id, bid+".cbz", deleteTestTime, bid, deleteTestTime, deleteTestTime)
			exec(`INSERT INTO pages (book_id, number, entry, media_type, width, height) VALUES (?, 1, '1.png', 'image/png', 1, 1), (?, 2, '2.png', 'image/png', 1, 1)`, bid, bid)
			exec(`INSERT INTO search_rows (kind, ref, library_id, text) VALUES ('book', ?, ?, ?)`, bid, id, bid)
			exec(`INSERT INTO read_progress (user_id, book_id, page, completed, read_at) VALUES ('u1', ?, 1, 0, ?)`, bid, deleteTestTime)
		}
	}
}

func countRows(t *testing.T, conn *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// TestDeleteLibraryInBatches removes a library with 5 series and 15 books in
// batches of 4. Between the batches another writer gets the database (the
// hook writes, which fails while a batch transaction is still open), and the
// rows of the other library, and the collection it shares, are left alone.
func TestDeleteLibraryInBatches(t *testing.T) {
	ctx := context.Background()
	conn := openDeleteTestDB(t)
	seedDeleteLibrary(t, conn, "gone", 1, 5, 3)
	seedDeleteLibrary(t, conn, "kept", 0, 2, 2)
	for _, q := range []string{
		`INSERT INTO collections (id, name, created_at) VALUES ('only-gone', 'only gone', ?), ('mixed', 'mixed', ?), ('kept-only', 'kept only', ?)`,
	} {
		if _, err := conn.Exec(q, deleteTestTime, deleteTestTime, deleteTestTime); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{
		`INSERT INTO collection_series (collection_id, series_id, position) VALUES ('only-gone', 'gone-s0', 0)`,
		`INSERT INTO collection_series (collection_id, series_id, position) VALUES ('mixed', 'gone-s1', 0), ('mixed', 'kept-s0', 1)`,
		`INSERT INTO collection_series (collection_id, series_id, position) VALUES ('kept-only', 'kept-s1', 0)`,
	} {
		if _, err := conn.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	s := &Service{DB: conn, deleteBatch: 4}
	hooks := 0
	s.deleteHook = func() {
		hooks++
		if _, err := conn.Exec(`UPDATE libraries SET name = name WHERE id = 'kept'`); err != nil {
			t.Errorf("another write between batches failed: %v", err)
		}
	}
	var last float64
	if err := s.deleteLibrary(ctx, "gone", func(p float64) { last = p }); err != nil {
		t.Fatal(err)
	}
	// Books: 15 in batches of 4 is 4 transactions; series: 5 in batches of 4 is 2.
	if hooks != 6 {
		t.Errorf("batches committed = %d, want 6 (one per batch)", hooks)
	}
	if last != 1 {
		t.Errorf("final progress = %v, want 1", last)
	}

	for _, c := range []struct {
		what  string
		query string
		want  int
		args  []any
	}{
		{"library row", `SELECT count(*) FROM libraries WHERE id = 'gone'`, 0, nil},
		{"series", `SELECT count(*) FROM series WHERE library_id = 'gone'`, 0, nil},
		{"books", `SELECT count(*) FROM books WHERE library_id = 'gone'`, 0, nil},
		{"search rows of the library", `SELECT count(*) FROM search_rows WHERE library_id = 'gone'`, 0, nil},
		{"pages of kept books", `SELECT count(*) FROM pages`, 8, nil},
		{"progress of kept books", `SELECT count(*) FROM read_progress`, 4, nil},
		{"search rows of kept rows", `SELECT count(*) FROM search_rows`, 6, nil},
		{"libraries", `SELECT count(*) FROM libraries`, 1, nil},
		{"collections", `SELECT count(*) FROM collections`, 2, nil},
		{"collection entries", `SELECT count(*) FROM collection_series`, 2, nil},
	} {
		if got := countRows(t, conn, c.query, c.args...); got != c.want {
			t.Errorf("%s = %d, want %d", c.what, got, c.want)
		}
	}
	if n := countRows(t, conn, `SELECT count(*) FROM collections WHERE id = 'only-gone'`); n != 0 {
		t.Error("the collection that held only deleted series is still there")
	}
}

// With deleteEmptyCollections off (stored under the server settings), the
// collection that lost its only series stays.
func TestDeleteLibraryKeepsCollectionsWhenSettingIsOff(t *testing.T) {
	ctx := context.Background()
	conn := openDeleteTestDB(t)
	seedDeleteLibrary(t, conn, "gone", 1, 1, 1)
	if _, err := conn.Exec(`INSERT INTO settings (key, value) VALUES ('server', '{"deleteEmptyCollections": false}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO collections (id, name, created_at) VALUES ('c', 'c', ?)`, deleteTestTime); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO collection_series (collection_id, series_id, position) VALUES ('c', 'gone-s0', 0)`); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: conn}
	if err := s.deleteLibrary(ctx, "gone", func(float64) {}); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, conn, `SELECT count(*) FROM collections WHERE id = 'c'`); n != 1 {
		t.Errorf("collection kept with the setting off: %d rows, want 1", n)
	}
}

// A library left with its deleting flag is removed once Schedule starts, and
// until then it is neither scanned nor listed for scans. A deleting library
// that is due for a scan is not queued for one either.
func TestDeletingLibraryIsResumedAtStartupAndNotScanned(t *testing.T) {
	ctx := context.Background()
	conn := openDeleteTestDB(t)
	seedDeleteLibrary(t, conn, "gone", 1, 2, 2)
	seedDeleteLibrary(t, conn, "kept", 0, 1, 1)
	// Due for a daily scan (never scanned) if it were not being deleted.
	if _, err := conn.Exec(`UPDATE libraries SET scan_interval = 'daily' WHERE id = 'gone'`); err != nil {
		t.Fatal(err)
	}

	q := task.New(conn, 2)
	s := &Service{DB: conn, Queue: q, ThumbDir: t.TempDir()}
	if _, err := s.QueueScan(ctx, "gone"); !errors.Is(err, ErrNotFound) {
		t.Errorf("QueueScan on a deleting library: %v, want ErrNotFound", err)
	}
	if _, err := s.QueueDeepScan(ctx, "gone"); !errors.Is(err, ErrNotFound) {
		t.Errorf("QueueDeepScan on a deleting library: %v, want ErrNotFound", err)
	}
	if err := q.Run(ctx); err != nil {
		t.Fatal(err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go s.Schedule(runCtx)

	waitUntil(t, "the deleting library to be removed", func() bool {
		return countRows(t, conn, `SELECT count(*) FROM libraries WHERE id = 'gone'`) == 0
	})
	cancel()
	if n := countRows(t, conn, `SELECT count(*) FROM books WHERE library_id = 'gone'`); n != 0 {
		t.Errorf("books of the deleted library left: %d", n)
	}
	if n := countRows(t, conn, `SELECT count(*) FROM books WHERE library_id = 'kept'`); n != 1 {
		t.Errorf("books of the other library = %d, want 1", n)
	}
	tasks, err := q.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range tasks {
		if tk.Kind == "scan" && tk.LibraryID == "gone" {
			t.Error("a deleting library was queued for a scan at startup")
		}
	}
}

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Two libraries deleted at once take turns: while one is part way through its
// books, the other has none removed. With one-book batches and a pause after
// each, two deletes running together would both be part way through at once.
func TestDeletesRunOneAtATime(t *testing.T) {
	ctx := context.Background()
	conn := openDeleteTestDB(t)
	seedDeleteLibrary(t, conn, "a", 1, 2, 2)
	seedDeleteLibrary(t, conn, "b", 1, 2, 2)
	var overlap atomic.Bool
	s := &Service{DB: conn, deleteBatch: 1}
	s.deleteHook = func() {
		// Libraries that still have some, but not all, of their four books.
		var n int
		if err := conn.QueryRow(`SELECT count(*) FROM (SELECT library_id FROM books GROUP BY library_id HAVING count(*) < 4)`).Scan(&n); err != nil {
			t.Errorf("count: %v", err)
			return
		}
		if n > 1 {
			overlap.Store(true)
		}
		time.Sleep(2 * time.Millisecond)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, id := range []string{"a", "b"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = s.deleteLibrary(ctx, id, func(float64) {})
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if overlap.Load() {
		t.Error("two library deletes ran at once")
	}
	if n := countRows(t, conn, `SELECT count(*) FROM libraries`); n != 0 {
		t.Errorf("libraries left: %d", n)
	}
}

// The delete task's row outlives its library: it ends as done, detached from
// the library, so the task list still shows the delete.
func TestDeleteKeepsItsTaskRow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := openDeleteTestDB(t)
	seedDeleteLibrary(t, conn, "gone", 1, 1, 1)
	q := task.New(conn, 2)
	if err := q.Run(ctx); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: conn, Queue: q}
	tk, err := s.QueueDelete(ctx, "gone")
	if err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the delete task to end", func() bool {
		return countRows(t, conn, `SELECT count(*) FROM tasks WHERE id = ? AND state = 'done'`, tk.ID) == 1
	})
	if n := countRows(t, conn, `SELECT count(*) FROM libraries WHERE id = 'gone'`); n != 0 {
		t.Errorf("library left: %d rows", n)
	}
	if n := countRows(t, conn, `SELECT count(*) FROM tasks WHERE id = ? AND kind = 'delete' AND library_id IS NULL`, tk.ID); n != 1 {
		t.Errorf("delete task row detached from its library: %d rows, want 1", n)
	}
}

// A library left marked deleting with no task row for it (its queue request
// never got through) is queued again by the next tick.
func TestScheduleRequeuesOrphanDelete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := openDeleteTestDB(t)
	seedDeleteLibrary(t, conn, "gone", 1, 1, 1)
	q := task.New(conn, 2)
	if err := q.Run(ctx); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: conn, Queue: q}
	s.queueOrphanDeletes(ctx) // one tick of Schedule
	waitUntil(t, "the orphan delete to remove the library", func() bool {
		return countRows(t, conn, `SELECT count(*) FROM libraries WHERE id = 'gone'`) == 0
	})
}

// A delete the admin cancelled keeps its task row, so a tick does not queue the
// library again: it stays marked deleting until the next server start.
func TestCancelledDeleteIsNotRequeuedByTick(t *testing.T) {
	ctx := context.Background()
	conn := openDeleteTestDB(t)
	seedDeleteLibrary(t, conn, "gone", 1, 1, 1)
	// No Run: the task stays queued until it is cancelled.
	q := task.New(conn, 2)
	s := &Service{DB: conn, Queue: q}
	tk, err := s.QueueDelete(ctx, "gone")
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Cancel(ctx, tk.ID); err != nil {
		t.Fatal(err)
	}
	s.queueOrphanDeletes(ctx)
	if n := countRows(t, conn, `SELECT count(*) FROM tasks WHERE kind = 'delete'`); n != 1 {
		t.Errorf("delete task rows after a tick = %d, want 1 (the cancelled one)", n)
	}
	if n := countRows(t, conn, `SELECT count(*) FROM tasks WHERE id = ? AND state = 'cancelled'`, tk.ID); n != 1 {
		t.Errorf("the cancelled delete task is not cancelled")
	}
	if n := countRows(t, conn, `SELECT count(*) FROM libraries WHERE id = 'gone'`); n != 1 {
		t.Errorf("the library of a cancelled delete was removed or re-queued: %d rows", n)
	}
}

// Libraries marked deleting and queued one after another share one delete task:
// while the removal runs, no two delete tasks are queued or running at once, and
// all three libraries go. Each delete batch is slowed so the task outlasts the
// three requests in most runs; the task count is not asserted, since a request
// can come after the task has ended.
func TestDeletesShareOneTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := openDeleteTestDB(t)
	for _, id := range []string{"a", "b", "c"} {
		seedDeleteLibrary(t, conn, id, 1, 2, 3)
	}
	q := task.New(conn, 2)
	if err := q.Run(ctx); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: conn, Queue: q, deleteBatch: 1}
	s.deleteHook = func() { time.Sleep(2 * time.Millisecond) }
	for _, id := range []string{"a", "b", "c"} {
		if _, err := s.QueueDelete(ctx, id); err != nil && !errors.Is(err, ErrNotFound) {
			t.Fatalf("QueueDelete(%s): %v", id, err)
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	for countRows(t, conn, `SELECT count(*) FROM libraries`) > 0 {
		if n := countRows(t, conn, `SELECT count(*) FROM tasks WHERE kind = 'delete' AND state IN ('queued','running')`); n > 1 {
			t.Fatalf("%d delete tasks queued or running at once, want at most 1", n)
		}
		if time.Now().After(deadline) {
			t.Fatal("libraries still there after 10 seconds")
		}
		time.Sleep(time.Millisecond)
	}
	t.Logf("delete task rows for three libraries: %d", countRows(t, conn, `SELECT count(*) FROM tasks WHERE kind = 'delete'`))
}

// The rows that a delete and its cascades look up by library, series or book
// have full indexes: a partial index cannot serve them, and a scan of the
// whole table per batch made the delete slow enough to hold the write lock.
func TestCascadeUsesIndexes(t *testing.T) {
	conn := openDeleteTestDB(t)
	for _, c := range []struct{ query, index string }{
		{`SELECT 1 FROM books WHERE series_id = ?`, "books_series_all"},
		{`SELECT 1 FROM books WHERE library_id = ?`, "books_library_all"},
		{`SELECT 1 FROM series WHERE library_id = ?`, "series_library_all"},
		{`SELECT 1 FROM progress_writers WHERE book_id = ?`, "progress_writers_book"},
		{`SELECT 1 FROM user_libraries WHERE library_id = ?`, "user_libraries_library"},
		{`SELECT 1 FROM tasks WHERE library_id = ?`, "tasks_library"},
	} {
		if plan := queryPlan(t, conn, c.query); !strings.Contains(plan, c.index) {
			t.Errorf("%s: plan does not use %s: %s", c.query, c.index, plan)
		}
	}
}

// queryPlan returns the EXPLAIN QUERY PLAN lines of a query with one parameter.
func queryPlan(t *testing.T, conn *sql.DB, query string) string {
	t.Helper()
	rows, err := conn.Query("EXPLAIN QUERY PLAN "+query, "x")
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(lines, "; ")
}
