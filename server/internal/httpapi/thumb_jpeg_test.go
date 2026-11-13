package httpapi

import (
	"bytes"
	"image"
	"image/color"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gen2brain/webp"
	"github.com/tabunugoku/mars/server/internal/auth"
	"github.com/tabunugoku/mars/server/internal/library"
	"github.com/tabunugoku/mars/server/internal/media"
)

// webpThumb is a WebP image of size w x h, filled with c.
func webpThumb(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := webp.Encode(&b, img, webp.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// storeThumb writes data as the stored thumbnail file name of a book and points
// the book row at it.
func storeThumb(t *testing.T, e *env, bookID, name string, data []byte) {
	t.Helper()
	dir := filepath.Join(e.data, "thumbs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`UPDATE books SET thumbnail = ? WHERE id = ?`, name, bookID); err != nil {
		t.Fatal(err)
	}
}

// jpegCacheDir is where the JPEG copies of WebP covers are kept in these tests.
func jpegCacheDir(e *env) string { return filepath.Join(e.data, "cache", "thumb-jpeg") }

// jpegCompatHandler is the /opds/ handler as cmd/mars builds it, with the JPEG
// cache folder set (opdsHandler leaves it out).
func jpegCompatHandler(e *env) http.Handler {
	lib := &library.Service{DB: e.db, Pages: &media.PageCache{Dir: filepath.Join(e.data, "pages"), MaxBytes: 1 << 30},
		ThumbDir: filepath.Join(e.data, "thumbs"), PageThumbDir: filepath.Join(e.data, "page-thumbs"), Events: e.hub}
	return NewCompat(Deps{DB: e.db, Version: "test", Library: lib, Auth: auth.Store{DB: e.db},
		Limiter: &auth.Limiter{}, Gate: e.gate, Events: e.hub, JPEGCacheDir: jpegCacheDir(e)})
}

// checkJPEG wants a 200 answer of type image/jpeg whose body is a JPEG of the
// given width.
func checkJPEG(t *testing.T, code int, ct string, body []byte, width int) {
	t.Helper()
	if code != http.StatusOK || ct != "image/jpeg" {
		t.Fatalf("answer %d %q, want 200 image/jpeg", code, ct)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil || format != "jpeg" {
		t.Fatalf("body is not a JPEG: %q %v", format, err)
	}
	if cfg.Width != width {
		t.Errorf("width %d, want %d", cfg.Width, width)
	}
}

func TestWebPThumbnailServedAsStoredAndAsJPEG(t *testing.T) {
	e, _, lib := opdsFixture(t, 0)
	yofuke := findSeries(t, e, lib.Id, "Yofuke")
	var books []Book
	e.admin.json("GET", "/api/v1/series/"+yofuke+"/books", nil, 200, &books)
	book := books[0].Id
	const name = "book-yofuke-test.webp"
	storeThumb(t, e, book, name, webpThumb(t, 40, 60, color.RGBA{200, 0, 0, 255}))

	// The core route answers with the stored type.
	rec := e.admin.do("GET", "/api/v1/books/"+book+"/thumbnail", nil)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/webp" {
		t.Fatalf("core: %d %q, want 200 image/webp", rec.Code, rec.Header().Get("Content-Type"))
	}

	// The OPDS routes answer with a JPEG, made once and kept on disk.
	h := jpegCompatHandler(e)
	rec = opdsGet(h, "/opds/v1.2/books/"+book+"/thumbnail", opdsReaderName, opdsReaderPass)
	checkJPEG(t, rec.Code, rec.Header().Get("Content-Type"), rec.Body.Bytes(), 40)
	cached := filepath.Join(jpegCacheDir(e), "book-yofuke-test.jpg")
	if _, err := os.Stat(cached); err != nil {
		t.Fatalf("cache copy: %v", err)
	}
	rec = opdsGet(h, "/opds/v1.2/series/"+yofuke+"/thumbnail", opdsReaderName, opdsReaderPass)
	checkJPEG(t, rec.Code, rec.Header().Get("Content-Type"), rec.Body.Bytes(), 40)
}

func TestWebPThumbnailUnreadableIsCoverError(t *testing.T) {
	e, _, lib := opdsFixture(t, 0)
	yofuke := findSeries(t, e, lib.Id, "Yofuke")
	var books []Book
	e.admin.json("GET", "/api/v1/series/"+yofuke+"/books", nil, 200, &books)
	storeThumb(t, e, books[0].Id, "book-bad-test.webp", []byte("not webp at all"))

	rec := opdsGet(jpegCompatHandler(e), "/opds/v1.2/books/"+books[0].Id+"/thumbnail", opdsReaderName, opdsReaderPass)
	if rec.Code != http.StatusInternalServerError || answerCode(t, rec) != "server.cover_unreadable" {
		t.Fatalf("answer %d, want 500 server.cover_unreadable: %s", rec.Code, rec.Body)
	}
}

func TestWebPThumbnailKomgaIsJPEG(t *testing.T) {
	f := newKomgaFix(t)
	// The fixture's compat handler has no JPEG cache folder; rebuild it with one.
	lib := &library.Service{DB: f.e.db, ThumbDir: filepath.Join(f.e.data, "thumbs"),
		Pages:        &media.PageCache{Dir: filepath.Join(f.e.data, "pages"), MaxBytes: 1 << 30},
		PageThumbDir: filepath.Join(f.e.data, "page-thumbs"), Events: f.e.hub}
	f.h = NewCompat(Deps{DB: f.e.db, Version: "test", Library: lib, Auth: auth.Store{DB: f.e.db},
		Limiter: &auth.Limiter{}, Gate: f.e.gate, Events: f.e.hub, JPEGCacheDir: jpegCacheDir(f.e)})

	book := f.alphaBooks[0].Id
	storeThumb(t, f.e, book, "book-alpha-test.webp", webpThumb(t, 40, 60, color.RGBA{0, 0, 200, 255}))
	rec := f.do(komgaReader, "/komga/api/v1/books/"+book+"/thumbnail")
	checkJPEG(t, rec.Code, rec.Header().Get("Content-Type"), rec.Body.Bytes(), 40)

	rec = f.do(komgaReader, "/komga/api/v1/series/"+f.alpha.Id+"/thumbnail")
	checkJPEG(t, rec.Code, rec.Header().Get("Content-Type"), rec.Body.Bytes(), 40)
}

// TestEmptyTrashRemovesJPEGCopy checks that emptying the trash removes a WebP
// thumbnail together with the JPEG copy made for OPDS and the Komga API.
func TestEmptyTrashRemovesJPEGCopy(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	img := pngBytes(t, 40, 60, color.RGBA{10, 10, 10, 255})
	writeCBZ(t, filepath.Join(root, "Gone", "Gone v01.cbz"), map[string][]byte{"1.png": img})
	var l Library
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "T", "root": root, "kind": "manga"}, 201, &l)
	e.waitTasks()
	var book string
	for _, s := range seriesOf(e, l.Id) {
		for _, bk := range booksOf(e, s.Id) {
			book = bk.Id
		}
	}
	thumbs := filepath.Join(e.data, "thumbs")
	stored := filepath.Join(thumbs, "trash-aa.webp")
	copied := filepath.Join(thumbs, library.JPEGCopyDir, "trash-aa.jpg")
	if err := os.MkdirAll(filepath.Dir(copied), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{stored, copied} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := e.db.Exec(`UPDATE books SET thumbnail = 'trash-aa.webp', deleted_at = ? WHERE id = ?`, now, book); err != nil {
		t.Fatal(err)
	}
	e.admin.json("POST", "/api/v1/trash/empty", nil, 200, nil)
	for _, p := range []string{stored, copied} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s still exists after emptying the trash", p)
		}
	}
}
