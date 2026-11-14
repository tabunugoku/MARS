package library

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tabunugoku/mars/server/internal/db"
	"github.com/tabunugoku/mars/server/internal/task"
)

// abWriteCBZ writes a CBZ with the given number of small PNG pages.
func abWriteCBZ(t *testing.T, path string, pages int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for i := 1; i <= pages; i++ {
		img := image.NewGray(image.Rect(0, 0, 4+i, 6))
		img.SetGray(0, 0, color.Gray{Y: uint8(i)})
		w, err := zw.Create(fmt.Sprintf("p%02d.png", i))
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(buf.Bytes()); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

// abSetup makes a library of n good books, one broken file and one CBZ with
// no pages, scans it, and returns the service and the book ids in path order.
func abSetup(t *testing.T, n int) (context.Context, *Service, lib, []string) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	root := t.TempDir()
	for i := 1; i <= n; i++ {
		abWriteCBZ(t, filepath.Join(root, "S", fmt.Sprintf("b%02d.cbz", i)), 2+i%3)
	}
	if err := os.WriteFile(filepath.Join(root, "S", "broken.cbz"), []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	abWriteCBZ(t, filepath.Join(root, "S", "empty.cbz"), 0)
	const ts = "2026-01-01T00:00:00Z"
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO libraries (id, name, root, kind, watch, created_at) VALUES ('lib1', 'L', ?, 'comics', 0, ?)`,
		root, ts); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: conn, Queue: task.New(conn, 1), ThumbDir: t.TempDir()}
	if err := s.scanLibrary(ctx, "lib1", nil, true, func(float64) {}); err != nil {
		t.Fatal(err)
	}
	l, err := s.load(ctx, "lib1")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := conn.QueryContext(ctx, `SELECT id FROM books WHERE library_id = 'lib1' ORDER BY path`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var todo []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		todo = append(todo, id)
	}
	if len(todo) != n+2 {
		t.Fatalf("scanned %d books, want %d", len(todo), n+2)
	}
	return ctx, s, l, todo
}

// abSnapshot returns every book's analysis result and its pages.
func abSnapshot(t *testing.T, s *Service) string {
	t.Helper()
	var out []string
	rows, err := s.DB.Query(`SELECT b.path, b.media_status, b.media_error, b.page_count, b.cover_page, p.number, p.entry, p.media_type, p.width, p.height
		FROM books b LEFT JOIN pages p ON p.book_id = b.id WHERE b.library_id = 'lib1' ORDER BY b.path, p.number`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var path, status, msg string
		var pages, cover int
		var num, w, h sql.NullInt64
		var entry, mt sql.NullString
		if err := rows.Scan(&path, &status, &msg, &pages, &cover, &num, &entry, &mt, &w, &h); err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprintf("%s|%s|%s|%d|%d|%v|%v|%v|%v|%v", path, status, msg, pages, cover, num, entry, mt, w, h))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(out, "\n")
}

func TestAnalyzeBooksWorkersGiveSameRows(t *testing.T) {
	ctx, s, l, todo := abSetup(t, 20)

	var snaps []string
	for _, workers := range []int{1, 4} {
		s.AnalyzeWorkers = workers
		var mu sync.Mutex
		var seen []float64
		_, err := s.analyzeBooks(ctx, l, todo, func(p float64) {
			mu.Lock()
			seen = append(seen, p)
			mu.Unlock()
		})
		if err != nil {
			t.Fatalf("workers=%d: %v", workers, err)
		}
		if len(seen) != len(todo) {
			t.Fatalf("workers=%d: %d progress calls, want %d", workers, len(seen), len(todo))
		}
		if !sort.Float64sAreSorted(seen) {
			t.Errorf("workers=%d: progress not monotonic: %v", workers, seen)
		}
		if last := seen[len(seen)-1]; last != 1 {
			t.Errorf("workers=%d: final progress %v, want 1", workers, last)
		}
		snaps = append(snaps, abSnapshot(t, s))
	}
	if snaps[0] != snaps[1] {
		t.Fatalf("rows differ between 1 and 4 workers:\n--- 1\n%s\n--- 4\n%s", snaps[0], snaps[1])
	}
	var ready, failed int
	if err := s.DB.QueryRowContext(ctx, `SELECT
		COUNT(CASE WHEN media_status = 'ready' THEN 1 END),
		COUNT(CASE WHEN media_status IN ('error', 'unsupported') THEN 1 END)
		FROM books WHERE library_id = 'lib1'`).Scan(&ready, &failed); err != nil {
		t.Fatal(err)
	}
	if ready != 20 || failed != 2 {
		t.Errorf("%d ready and %d failed books, want 20 and 2", ready, failed)
	}
}

func TestAnalyzeBooksDatabaseErrorStopsRun(t *testing.T) {
	ctx, s, l, todo := abSetup(t, 20)
	s.AnalyzeWorkers = 4
	if _, err := s.DB.ExecContext(ctx,
		`CREATE TRIGGER boom BEFORE INSERT ON pages BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	last := 0.0
	done := make(chan error, 1)
	go func() {
		_, err := s.analyzeBooks(ctx, l, todo, func(p float64) {
			mu.Lock()
			last = p
			mu.Unlock()
		})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("err = %v, want the database error", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("analyzeBooks did not return after a database error")
	}
	mu.Lock()
	defer mu.Unlock()
	if last >= 1 {
		t.Errorf("progress reached %v after an error", last)
	}
}

func TestAnalyzeBooksCancelled(t *testing.T) {
	ctx, s, l, todo := abSetup(t, 5)
	s.AnalyzeWorkers = 4
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	_, err := s.analyzeBooks(cctx, l, todo, func(float64) {})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
