package library

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tabunugoku/mars/server/internal/db"
	"github.com/tabunugoku/mars/server/internal/task"
)

// TestDeleteLoad deletes six libraries at once, the way the handler and the
// queue do, while a short writer updates a user row every 100 ms. It fails if
// a write of that writer errors or waits a second or more. It is skipped
// unless MARS_DELETE_LOAD holds the total number of books to seed, e.g. 37000.
//
//	MARS_DELETE_LOAD=6000 go test ./internal/library -run TestDeleteLoad -v -timeout 30m
func TestDeleteLoad(t *testing.T) {
	raw := os.Getenv("MARS_DELETE_LOAD")
	if raw == "" {
		t.Skip("set MARS_DELETE_LOAD to the total number of books to seed")
	}
	total, err := strconv.Atoi(raw)
	if err != nil || total <= 0 {
		t.Skipf("MARS_DELETE_LOAD = %q: want a positive integer", raw)
	}
	const libraries, seriesBooks, pagesPerBook = 6, 20, 200
	booksEach := max(total/libraries, 1)

	ctx := context.Background()
	conn := openDeleteTestDB(t)
	start := time.Now()
	for i := range libraries {
		seedLoadLibrary(t, conn, fmt.Sprintf("lib%d", i), booksEach, seriesBooks, pagesPerBook)
	}
	t.Logf("seeded %d libraries, %d books, %d page rows in %s",
		libraries, libraries*booksEach, libraries*booksEach*pagesPerBook, time.Since(start).Round(time.Millisecond))

	q := task.New(conn, 2)
	if err := q.Run(ctx); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: conn, Queue: q}

	// The short writer runs throughout; its calls are timed one by one.
	writerCtx, stopWriter := context.WithCancel(ctx)
	defer stopWriter()
	var (
		wg       sync.WaitGroup
		writes   int
		failures int
		firstErr error
		maxWait  time.Duration
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-writerCtx.Done():
				return
			case <-tick.C:
			}
			begun := time.Now()
			_, err := conn.ExecContext(writerCtx, `UPDATE users SET last_active = ? WHERE id = 'u1'`, begun.UTC().Format(time.RFC3339Nano))
			took := time.Since(begun)
			if writerCtx.Err() != nil {
				return
			}
			writes++
			if err != nil {
				failures++
				if firstErr == nil {
					firstErr = err
				}
			}
			maxWait = max(maxWait, took)
		}
	}()

	runStart := time.Now()
	for i := range libraries {
		id := fmt.Sprintf("lib%d", i)
		if err := db.RetryBusy(ctx, func() error {
			_, err := conn.ExecContext(ctx, `UPDATE libraries SET deleting = 1 WHERE id = ? AND deleting = 0`, id)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if err := db.RetryBusy(ctx, func() error {
			_, err := s.QueueDelete(ctx, id)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("queued %d library deletes in %s", libraries, time.Since(runStart).Round(time.Millisecond))

	deadline := time.Now().Add(30 * time.Minute)
	for countRows(t, conn, `SELECT count(*) FROM libraries`) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("libraries still there after 30 minutes")
		}
		time.Sleep(500 * time.Millisecond)
	}
	elapsed := time.Since(runStart)
	stopWriter()
	wg.Wait()

	if n := countRows(t, conn, `SELECT count(*) FROM books`); n != 0 {
		t.Errorf("books left after the deletes: %d", n)
	}
	if n := countRows(t, conn, `SELECT count(*) FROM pages`); n != 0 {
		t.Errorf("page rows left after the deletes: %d", n)
	}
	t.Logf("deleted %d libraries in %s", libraries, elapsed.Round(time.Millisecond))
	t.Logf("short writer: %d writes, %d errors, max wait %s", writes, failures, maxWait.Round(time.Millisecond))
	if failures > 0 {
		t.Errorf("short writer errors: %d, first: %v", failures, firstErr)
	}
	if maxWait >= time.Second {
		t.Errorf("short writer max wait %s, want under 1s", maxWait)
	}
}

// seedLoadLibrary inserts a library not marked deleting, with books books in
// series of perSeries, each book with pages page rows and a search row, the
// series with one too. It uses set-based SQL so the seeding is quick.
func seedLoadLibrary(t *testing.T, conn *sql.DB, id string, books, perSeries, pages int) {
	t.Helper()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	series := (books + perSeries - 1) / perSeries
	exec(`INSERT INTO libraries (id, name, root, kind, scan_interval, watch, created_at, deleting) VALUES (?, ?, ?, 'manga', 'off', 0, ?, 0)`,
		id, id, t.TempDir(), deleteTestTime)
	exec(`WITH RECURSIVE seq(n) AS (SELECT 0 UNION ALL SELECT n + 1 FROM seq WHERE n < ?)
		INSERT INTO series (id, library_id, dir, title, title_sort, created_at, updated_at)
		SELECT ? || '-s' || n, ?, ? || '-s' || n, ? || '-s' || n, ? || '-s' || n, ?, ? FROM seq`,
		series-1, id, id, id, id, id, deleteTestTime, deleteTestTime)
	exec(`WITH RECURSIVE seq(n) AS (SELECT 0 UNION ALL SELECT n + 1 FROM seq WHERE n < ?)
		INSERT INTO books (id, series_id, library_id, path, format, size_bytes, mtime, title, number, number_sort, created_at, updated_at)
		SELECT ? || '-b' || n, ? || '-s' || (n / ?), ?, ? || '-b' || n || '.cbz', 'cbz', 1, ?, ? || '-b' || n, 1, 1, ?, ? FROM seq`,
		books-1, id, id, perSeries, id, id, deleteTestTime, id, deleteTestTime, deleteTestTime)
	exec(`WITH RECURSIVE bk(n) AS (SELECT 0 UNION ALL SELECT n + 1 FROM bk WHERE n < ?),
		pg(p) AS (SELECT 1 UNION ALL SELECT p + 1 FROM pg WHERE p < ?)
		INSERT INTO pages (book_id, number, entry, media_type, width, height)
		SELECT ? || '-b' || bk.n, pg.p, pg.p || '.png', 'image/png', 1, 1 FROM bk, pg`,
		books-1, pages, id)
	exec(`WITH RECURSIVE seq(n) AS (SELECT 0 UNION ALL SELECT n + 1 FROM seq WHERE n < ?)
		INSERT INTO search_rows (kind, ref, library_id, text)
		SELECT 'book', ? || '-b' || n, ?, ? || '-b' || n FROM seq`, books-1, id, id, id)
	exec(`WITH RECURSIVE seq(n) AS (SELECT 0 UNION ALL SELECT n + 1 FROM seq WHERE n < ?)
		INSERT INTO search_rows (kind, ref, library_id, text)
		SELECT 'series', ? || '-s' || n, ?, ? || '-s' || n FROM seq`, series-1, id, id, id)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
