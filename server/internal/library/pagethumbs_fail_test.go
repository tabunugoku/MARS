package library

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tabunugoku/mars/server/internal/db"
)

// TestPageThumbFailureRemembered checks that a page whose image cannot be
// decoded fails once with a real error, then fails again at once with
// ErrPageThumbFailed, without decoding it a second time.
func TestPageThumbFailureRemembered(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cbz := filepath.Join(root, "bad.cbz")
	zf, err := os.Create(cbz)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	w, err := zw.Create("1.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("not an image")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zf.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(cbz)
	if err != nil {
		t.Fatal(err)
	}

	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	const ts = "2026-01-01T00:00:00Z"
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := conn.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO libraries (id, name, root, kind, watch, created_at) VALUES ('lib1', 'L', ?, 'manga', 0, ?)`, root, ts)
	exec(`INSERT INTO series (id, library_id, dir, title, title_sort, created_at, updated_at) VALUES ('s1', 'lib1', 'Series', 'Series', 'Series', ?, ?)`, ts, ts)
	exec(`INSERT INTO books (id, library_id, series_id, path, format, size_bytes, mtime, title, number, number_sort, media_status, created_at, updated_at)
		VALUES ('b1', 'lib1', 's1', 'bad.cbz', 'cbz', ?, ?, 'b1', 1, 1, 'ready', ?, ?)`, st.Size(), ts, ts, ts)
	exec(`INSERT INTO pages (book_id, number, entry, media_type, width, height) VALUES ('b1', 1, '1.jpg', 'image/jpeg', 1, 1)`)

	s := &Service{DB: conn, PageThumbDir: t.TempDir()}
	_, err = s.PageThumbnail(ctx, "b1", 1)
	if err == nil || errors.Is(err, ErrPageThumbFailed) {
		t.Fatalf("first call: err = %v, want a decode error", err)
	}
	_, err = s.PageThumbnail(ctx, "b1", 1)
	if !errors.Is(err, ErrPageThumbFailed) {
		t.Fatalf("second call: err = %v, want ErrPageThumbFailed", err)
	}
}
