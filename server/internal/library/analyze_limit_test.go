package library

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tabunugoku/mars/server/internal/db"
	"github.com/tabunugoku/mars/server/internal/task"
)

// alWritePDF writes a one-page PDF with an empty page, drawn by pdfium when
// analysed. The xref offsets are computed, so every reader accepts it.
func alWritePDF(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Contents 4 0 R >>",
		"<< /Length 0 >>\nstream\n\nendstream",
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&buf, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// alService opens a fresh database and returns a service on it.
func alService(t *testing.T) (context.Context, *Service) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return ctx, &Service{DB: conn, Queue: task.New(conn, 1), ThumbDir: t.TempDir()}
}

// alAddLibrary makes library id from the files write puts under its root,
// scans it and returns it with its book ids in path order.
func alAddLibrary(t *testing.T, ctx context.Context, s *Service, id string, write func(root string)) (lib, []string) {
	t.Helper()
	root := t.TempDir()
	write(root)
	const ts = "2026-01-01T00:00:00Z"
	if _, err := s.DB.ExecContext(ctx,
		`INSERT INTO libraries (id, name, root, kind, watch, created_at) VALUES (?, ?, ?, 'comics', 0, ?)`,
		id, id, root, ts); err != nil {
		t.Fatal(err)
	}
	if err := s.scanLibrary(ctx, id, nil, true, func(float64) {}); err != nil {
		t.Fatal(err)
	}
	l, err := s.load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM books WHERE library_id = ? ORDER BY path`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var todo []string
	for rows.Next() {
		var bid string
		if err := rows.Scan(&bid); err != nil {
			t.Fatal(err)
		}
		todo = append(todo, bid)
	}
	return l, todo
}

// alCountStatus counts the books of a library in each media status.
func alCountStatus(t *testing.T, s *Service, id string) map[string]int {
	t.Helper()
	rows, err := s.DB.Query(`SELECT media_status, COUNT(*) FROM books WHERE library_id = ? GROUP BY media_status`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st sql.NullString
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			t.Fatal(err)
		}
		out[st.String] = n
	}
	return out
}

// Two libraries analysed at once, with AnalyzeWorkers 2 each wanting 4, never
// have more than 2 books in flight, and both finish with their rows.
func TestAnalyzeBooksSharedLimit(t *testing.T) {
	ctx, s, l1, todo1 := abSetup(t, 12)
	l2, todo2 := alAddLibrary(t, ctx, s, "lib2", func(root string) {
		for i := 1; i <= 12; i++ {
			abWriteCBZ(t, filepath.Join(root, "S", fmt.Sprintf("c%02d.cbz", i)), 2+i%3)
		}
		if err := os.WriteFile(filepath.Join(root, "S", "broken.cbz"), []byte("not a zip"), 0o644); err != nil {
			t.Fatal(err)
		}
		abWriteCBZ(t, filepath.Join(root, "S", "empty.cbz"), 0)
	})
	s.AnalyzeWorkers = 2
	var inflight, peak atomic.Int32
	s.analyzeHook = func(string) func() {
		n := inflight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		return func() { inflight.Add(-1) }
	}

	var wg sync.WaitGroup
	var err1, err2 error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err1 = s.analyzeBooks(ctx, l1, todo1, func(float64) {})
	}()
	go func() {
		defer wg.Done()
		_, err2 = s.analyzeBooks(ctx, l2, todo2, func(float64) {})
	}()
	wg.Wait()
	if err1 != nil || err2 != nil {
		t.Fatalf("errors: %v, %v", err1, err2)
	}
	if p := peak.Load(); p != 2 {
		t.Errorf("peak books in flight = %d, want exactly 2 (the limit)", p)
	}
	for _, id := range []string{"lib1", "lib2"} {
		st := alCountStatus(t, s, id)
		if st["ready"] != 12 || st["error"]+st["unsupported"] != 2 {
			t.Errorf("%s: statuses %v, want 12 ready and 2 failed", id, st)
		}
	}
}

// A panic in one book's analysis becomes an error naming the book and the
// panic value; the run stops, its other workers are cancelled, and it returns.
func TestAnalyzeBooksPanicContained(t *testing.T) {
	ctx, s, l, todo := abSetup(t, 20)
	s.AnalyzeWorkers = 4
	target := todo[0]
	var entered atomic.Int32
	s.analyzeHook = func(id string) func() {
		entered.Add(1)
		if id == target {
			panic("boom from test")
		}
		return func() {}
	}
	done := make(chan error, 1)
	go func() {
		_, err := s.analyzeBooks(ctx, l, todo, func(float64) {})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), target) || !strings.Contains(err.Error(), "boom from test") {
			t.Fatalf("err = %v, want an error naming book %s and the panic value", err, target)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("analyzeBooks did not return after a panic")
	}
	if n := entered.Load(); n >= int32(len(todo)) {
		t.Errorf("%d of %d books started after the panic; siblings should stop", n, len(todo))
	}
	if n := len(s.analysisSlots.ch); n != 0 {
		t.Errorf("%d analysis slots still held after the run", n)
	}
}

// PDF books take the PDF slot one at a time when it has size 1, and both are
// analysed. The wait happens before AnalyzeCover's timeout, so neither is left
// outdated or marked as failed.
func TestAnalyzeBooksPDFSlot(t *testing.T) {
	ctx, s := alService(t)
	l, todo := alAddLibrary(t, ctx, s, "pdfs", func(root string) {
		alWritePDF(t, filepath.Join(root, "S", "p1.pdf"))
		alWritePDF(t, filepath.Join(root, "S", "p2.pdf"))
	})
	if len(todo) != 2 {
		t.Fatalf("scanned %d books, want 2", len(todo))
	}
	s.AnalyzeWorkers = 2
	s.pdfSlotSize = 1
	if _, err := s.analyzeBooks(ctx, l, todo, func(float64) {}); err != nil {
		t.Fatal(err)
	}
	st := alCountStatus(t, s, "pdfs")
	if st["ready"] != 2 {
		t.Errorf("statuses %v, want both PDFs ready", st)
	}
	if st["outdated"] != 0 {
		t.Errorf("%d PDFs left outdated", st["outdated"])
	}
}
