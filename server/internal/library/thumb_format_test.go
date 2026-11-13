package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tabunugoku/mars/server/internal/db"
	"github.com/tabunugoku/mars/server/internal/media"
)

// TestThumbFormatWebPNamesTheFile checks that a thumbnail made with
// ThumbFormat set to WebP is stored as a .webp file, and that the default
// (no ThumbFormat) keeps .jpg.
func TestThumbFormatWebPNamesTheFile(t *testing.T) {
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

	thumbs := t.TempDir()
	webpSvc := &Service{DB: conn, ThumbDir: thumbs,
		ThumbFormat: func(context.Context) media.Format { return media.FormatWebP }}
	if err := webpSvc.SetSeriesUpload(ctx, "s1", testPNG(t)); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := conn.QueryRowContext(ctx, `SELECT thumbnail FROM series WHERE id = 's1'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(name, ".webp") {
		t.Fatalf("thumbnail %q, want a .webp name", name)
	}
	if _, err := os.Stat(filepath.Join(thumbs, name)); err != nil {
		t.Fatal(err)
	}

	jpgSvc := &Service{DB: conn, ThumbDir: thumbs}
	if err := jpgSvc.SetSeriesUpload(ctx, "s1", testPNG(t)); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT thumbnail FROM series WHERE id = 's1'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(name, ".jpg") {
		t.Fatalf("default thumbnail %q, want a .jpg name", name)
	}
}

func TestNewThumbNameEnds(t *testing.T) {
	for _, ext := range []string{".jpg", ".webp", ".avif"} {
		if name := newThumbName("book-1", ext); !strings.HasPrefix(name, "book-1-") || !strings.HasSuffix(name, ext) {
			t.Errorf("newThumbName(%q) = %q", ext, name)
		}
	}
}

func TestRemoveThumbRemovesJPEGCopy(t *testing.T) {
	dir := t.TempDir()
	s := &Service{ThumbDir: dir}
	copyDir := filepath.Join(dir, JPEGCopyDir)
	if err := os.MkdirAll(copyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(dir, "b1-aa.webp"), filepath.Join(copyDir, "b1-aa.jpg")} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s.RemoveThumb("b1-aa.webp")
	for _, p := range []string{filepath.Join(dir, "b1-aa.webp"), filepath.Join(copyDir, "b1-aa.jpg")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s still exists", p)
		}
	}
}
