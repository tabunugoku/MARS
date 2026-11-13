package library

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/tabunugoku/mars/server/internal/db"
	"github.com/tabunugoku/mars/server/internal/task"
)

func TestReanalysePDFsOnce(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	const ts = "2026-01-01T00:00:00Z"
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO libraries (id, name, root, kind, watch, created_at) VALUES ('lib1', 'L', ?, 'manga', 0, ?)`,
		t.TempDir(), ts); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO series (id, library_id, dir, title, title_sort, created_at, updated_at) VALUES ('s1', 'lib1', 'Series', 'Series', 'Series', ?, ?)`,
		ts, ts); err != nil {
		t.Fatal(err)
	}
	books := []struct{ id, path, status, msg string }{
		{"page", "Series/page.pdf", "unsupported", "archive: unsupported: PDF page 2 is not a single image"},
		{"encrypted", "Series/encrypted.pdf", "unsupported", "archive: unsupported: encrypted PDF"},
		{"ready", "Series/ready.pdf", "ready", ""},
	}
	for _, b := range books {
		if _, err := conn.ExecContext(ctx, `INSERT INTO books
			(id, library_id, series_id, path, format, size_bytes, mtime, title, number, number_sort,
			 media_status, media_error, created_at, updated_at)
			VALUES (?, 'lib1', 's1', ?, 'pdf', 10, ?, ?, 1, 1, ?, ?, ?, ?)`,
			b.id, b.path, ts, b.id, b.status, b.msg, ts, ts); err != nil {
			t.Fatal(err)
		}
	}

	// The queue is not run, so the analysis it queues does not touch the books.
	s := &Service{DB: conn, Queue: task.New(conn, 1)}
	n, err := s.ReanalysePDFsOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("reset %d books, want 2", n)
	}
	want := map[string]string{"page": "outdated", "encrypted": "unsupported", "ready": "outdated"}
	for id, status := range want {
		var got string
		if err := conn.QueryRowContext(ctx, `SELECT media_status FROM books WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != status {
			t.Errorf("book %s is %q, want %q", id, got, status)
		}
	}

	// It runs once: a book refused again later (a page pdfium cannot read
	// either) is not queued on every start.
	if _, err := conn.ExecContext(ctx, `UPDATE books SET media_status = 'unsupported' WHERE id = 'page'`); err != nil {
		t.Fatal(err)
	}
	if n, err := s.ReanalysePDFsOnce(ctx); err != nil || n != 0 {
		t.Errorf("second run = %d, %v; want 0, nil", n, err)
	}
}
