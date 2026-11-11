package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/db"
	"github.com/tabunugoku/cominavi/server/internal/library"
	"github.com/tabunugoku/cominavi/server/internal/media"
	"github.com/tabunugoku/cominavi/server/internal/task"
)

func pngBytes(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func writeCBZ(t *testing.T, p string, files map[string][]byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(body)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

type client struct {
	t *testing.T
	h http.Handler
}

func (c client) do(method, url string, body any, hdr ...string) *httptest.ResponseRecorder {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, url, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	return rec
}

func (c client) json(method, url string, body any, want int, out any) {
	c.t.Helper()
	rec := c.do(method, url, body)
	if rec.Code != want {
		c.t.Fatalf("%s %s: status %d, want %d: %s", method, url, rec.Code, want, rec.Body)
	}
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			c.t.Fatalf("%s %s: %v", method, url, err)
		}
	}
}

// TestScanAndServe registers a library, lets the scan and analysis run, then
// reads series, books, pages and covers back through the API.
func TestScanAndServe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	data := t.TempDir()
	d, err := db.Open(ctx, filepath.Join(data, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	q := task.New(d, 2)
	if err := q.Run(ctx); err != nil {
		t.Fatal(err)
	}
	lib := &library.Service{DB: d, Queue: q, ThumbDir: filepath.Join(data, "thumbs"), Pages: &media.PageCache{Dir: filepath.Join(data, "pages"), MaxBytes: 1 << 30}}
	c := client{t, http.StripPrefix("/api/v1", New(Deps{DB: d, Version: "test", Library: lib, Tasks: q}))}

	root := t.TempDir()
	red, blue := pngBytes(t, 60, 90, color.RGBA{200, 0, 0, 255}), pngBytes(t, 120, 90, color.RGBA{0, 0, 200, 255})
	info := []byte(`<?xml version="1.0"?><ComicInfo><Series>夜更けの測量士</Series><Number>1</Number><Writer>北条 灯</Writer><Manga>YesAndRightToLeft</Manga></ComicInfo>`)
	writeCBZ(t, filepath.Join(root, "Yofuke", "Yofuke v01.cbz"), map[string][]byte{"p10.png": blue, "p2.png": red, "p1.png": red, "ComicInfo.xml": info})
	writeCBZ(t, filepath.Join(root, "Yofuke", "Yofuke v02.cbz"), map[string][]byte{"001.png": red})
	os.WriteFile(filepath.Join(root, "broken.cbz"), []byte("not a zip"), 0o644)

	var l Library
	c.json("POST", "/api/v1/libraries", map[string]any{"name": "漫画", "root": root, "kind": "comics"}, 201, &l)
	c.json("POST", "/api/v1/libraries", map[string]any{"name": "again", "root": root, "kind": "manga"}, 400, nil)
	c.json("POST", "/api/v1/libraries", map[string]any{"name": "rel", "root": "relative/path", "kind": "manga"}, 400, nil)

	deadline := time.Now().Add(10 * time.Second)
	for {
		var ts []Task
		c.json("GET", "/api/v1/tasks", nil, 200, &ts)
		busy, analyzed := false, false
		for _, tk := range ts {
			busy = busy || tk.State == Running || tk.State == Queued
			analyzed = analyzed || (tk.Kind == Analyze && tk.State == Done)
			if tk.State == Failed {
				t.Fatalf("task failed: %+v", tk)
			}
		}
		if !busy && analyzed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("tasks did not finish: %+v", ts)
		}
		time.Sleep(20 * time.Millisecond)
	}

	var series []Series
	c.json("GET", "/api/v1/series?libraryId="+l.Id, nil, 200, &series)
	if len(series) != 2 {
		t.Fatalf("series %+v", series)
	}
	var yofuke Series
	for _, s := range series {
		if s.BookCount == 2 {
			yofuke = s
		}
	}
	if yofuke.Title != "夜更けの測量士" || yofuke.ReadingDirection != ReadingDirectionRtl || len(yofuke.Authors) != 1 {
		t.Fatalf("series metadata %+v", yofuke)
	}

	var books []Book
	c.json("GET", "/api/v1/series/"+yofuke.Id+"/books", nil, 200, &books)
	if len(books) != 2 || books[0].Number != 1 || books[0].PageCount != 3 || books[0].MediaStatus != MediaStatusReady {
		t.Fatalf("books %+v", books)
	}
	var pages []Page
	c.json("GET", "/api/v1/books/"+books[0].Id+"/pages", nil, 200, &pages)
	if len(pages) != 3 || pages[2].Width != 120 || pages[2].Url != "/api/v1/books/"+books[0].Id+"/pages/3" {
		t.Fatalf("pages %+v", pages)
	}

	rec := c.do("GET", pages[2].Url, nil)
	if rec.Code != 200 || !bytes.Equal(rec.Body.Bytes(), blue) || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("page 3: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	etag := rec.Header().Get("ETag")
	if rec := c.do("GET", pages[2].Url, nil, "If-None-Match", etag); rec.Code != 304 {
		t.Fatalf("revalidate: %d", rec.Code)
	}
	if rec := c.do("GET", "/api/v1/books/"+books[0].Id+"/pages/9", nil); rec.Code != 404 {
		t.Fatalf("missing page: %d", rec.Code)
	}
	for _, u := range []string{books[0].ThumbnailUrl, yofuke.ThumbnailUrl} {
		rec := c.do("GET", u, nil)
		if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/jpeg" {
			t.Fatalf("thumbnail %s: %d", u, rec.Code)
		}
	}

	var issues []MediaIssue
	c.json("GET", "/api/v1/media/issues", nil, 200, &issues)
	if len(issues) != 1 || issues[0].Kind != MediaIssueKindUnsupported || !strings.Contains(issues[0].Detail, "broken.cbz") {
		t.Fatalf("issues %+v", issues)
	}

	c.json("POST", "/api/v1/tasks/nope/cancel", nil, 404, nil)
	c.json("GET", "/api/v1/libraries/nope", nil, 404, nil)
	c.json("DELETE", "/api/v1/libraries/"+l.Id, nil, 204, nil)
	c.json("GET", "/api/v1/series", nil, 200, &series)
	if len(series) != 0 {
		t.Fatalf("series after delete %+v", series)
	}
}
