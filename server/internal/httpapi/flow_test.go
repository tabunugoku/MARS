package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/auth"
	"github.com/tabunugoku/cominavi/server/internal/db"
	"github.com/tabunugoku/cominavi/server/internal/events"
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
	t      *testing.T
	h      http.Handler
	cookie string // session cookie value, if signed in
	bearer string // API key, if any
}

// do sends a request as this client. Writes carry a same-origin Origin
// header like a browser's; hdr pairs override any default.
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
	if c.cookie != "" {
		req.AddCookie(&http.Cookie{Name: cookieName, Value: c.cookie})
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	if !safeMethod(method) {
		req.Header.Set("Origin", "http://"+req.Host)
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

// sessionFrom returns the session cookie a response set, or "".
func sessionFrom(rec *httptest.ResponseRecorder) string {
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == cookieName {
			return ck.Value
		}
	}
	return ""
}

type env struct {
	t     *testing.T
	h     http.Handler
	db    *sql.DB
	gate  *SetupGate
	hub   *events.Hub
	admin client
	data  string // the data folder (database, caches)
}

// newEnv starts a server on a fresh database and runs first-run setup, so
// env.admin is a signed-in administrator.
func newEnv(t *testing.T) *env {
	t.Helper()
	return newEnvAt(t, t.TempDir())
}

// newEnvAt is newEnv with the data folder (database, thumbnails) in data.
func newEnvAt(t *testing.T, data string) *env {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	d, err := db.Open(ctx, filepath.Join(data, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	hub := events.New()
	t.Cleanup(hub.Close)
	q := task.New(d, 2)
	q.Events = hub
	if err := q.Run(ctx); err != nil {
		t.Fatal(err)
	}
	gate, err := NewSetupGate(ctx, d, data)
	if err != nil {
		t.Fatal(err)
	}
	lib := &library.Service{DB: d, Queue: q, ThumbDir: filepath.Join(data, "thumbs"), Pages: &media.PageCache{Dir: filepath.Join(data, "pages"), MaxBytes: 1 << 30}, PageThumbDir: filepath.Join(data, "page-thumbs"), Events: hub}
	h := http.StripPrefix("/api/v1", New(Deps{DB: d, Version: "test", Library: lib, Tasks: q,
		Auth: auth.Store{DB: d}, Limiter: &auth.Limiter{}, Gate: gate, Events: hub, Heartbeat: 50 * time.Millisecond}))
	e := &env{t: t, h: h, db: d, gate: gate, hub: hub, data: data}
	anon := e.anon()
	rec := anon.do("POST", "/api/v1/setup", map[string]any{"token": gate.token, "name": "管理人", "email": "admin@example.com", "password": "correct horse"})
	if rec.Code != 201 {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body)
	}
	e.admin = client{t: t, h: h, cookie: sessionFrom(rec)}
	return e
}

func (e *env) anon() client { return client{t: e.t, h: e.h} }

// login signs in as name and returns a client holding the session.
func (e *env) login(name, password string) client {
	e.t.Helper()
	rec := e.anon().do("POST", "/api/v1/auth/login", map[string]any{"username": name, "password": password})
	if rec.Code != 200 {
		e.t.Fatalf("login %s: %d %s", name, rec.Code, rec.Body)
	}
	return client{t: e.t, h: e.h, cookie: sessionFrom(rec)}
}

// waitTasks waits until no task is queued or running.
func (e *env) waitTasks() {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var ts []Task
		e.admin.json("GET", "/api/v1/tasks", nil, 200, &ts)
		busy := false
		for _, tk := range ts {
			busy = busy || tk.State == Running || tk.State == Queued
			if tk.State == Failed {
				e.t.Fatalf("task failed: %+v", tk)
			}
		}
		if !busy && len(ts) > 0 {
			return
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("tasks did not finish: %+v", ts)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestScanAndServe registers a library, lets the scan and analysis run, then
// reads series, books, pages and covers back through the API.
func TestScanAndServe(t *testing.T) {
	e := newEnv(t)
	c := e.admin
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
	if len(pages) != 3 || pages[2].Width != 120 || !strings.HasPrefix(pages[2].Url, "/api/v1/books/"+books[0].Id+"/pages/3?v=") {
		t.Fatalf("pages %+v", pages)
	}

	rec := c.do("GET", pages[2].Url, nil)
	if rec.Code != 200 || !bytes.Equal(rec.Body.Bytes(), blue) || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("page 3: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("versioned URL cache-control %q", cc)
	}
	if rec := c.do("GET", strings.Split(pages[2].Url, "?")[0], nil); rec.Header().Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("unversioned URL cache-control %q", rec.Header().Get("Cache-Control"))
	}
	etag := rec.Header().Get("ETag")
	if rec := c.do("GET", pages[2].Url, nil, "If-None-Match", etag); rec.Code != 304 {
		t.Fatalf("revalidate: %d", rec.Code)
	}
	if rec := c.do("GET", "/api/v1/books/"+books[0].Id+"/pages/9", nil); rec.Code != 404 {
		t.Fatalf("missing page: %d", rec.Code)
	}
	// Page thumbnails: a small JPEG, made once, then served from disk.
	if !strings.HasPrefix(pages[2].ThumbnailUrl, "/api/v1/books/"+books[0].Id+"/pages/3/thumbnail?v=") {
		t.Fatalf("page thumbnail url %q", pages[2].ThumbnailUrl)
	}
	rec = c.do("GET", pages[2].ThumbnailUrl, nil)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/jpeg" || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("page thumbnail: %d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Header().Get("Cache-Control"))
	}
	if cfg, err := jpeg.DecodeConfig(bytes.NewReader(rec.Body.Bytes())); err != nil || cfg.Width > 320 || cfg.Height > 480 {
		t.Fatalf("page thumbnail image %+v %v", cfg, err)
	}
	if rec := c.do("GET", pages[2].ThumbnailUrl, nil, "If-None-Match", rec.Header().Get("ETag")); rec.Code != 304 {
		t.Fatalf("page thumbnail revalidate: %d", rec.Code)
	}
	if rec := c.do("GET", "/api/v1/books/"+books[0].Id+"/pages/9/thumbnail", nil); rec.Code != 404 {
		t.Fatalf("missing page thumbnail: %d", rec.Code)
	}
	if n, _ := filepath.Glob(filepath.Join(e.data, "page-thumbs", books[0].Id, "*.jpg")); len(n) != 1 {
		t.Fatalf("page thumbnail files %v", n)
	}
	// The book file changes: the next thumbnail is made for the new version
	// and the old version's file goes.
	oldFiles, _ := filepath.Glob(filepath.Join(e.data, "page-thumbs", books[0].Id, "*.jpg"))
	if _, err := e.db.Exec(`UPDATE books SET mtime = '2001-01-01T00:00:00Z' WHERE id = ?`, books[0].Id); err != nil {
		t.Fatal(err)
	}
	if rec := c.do("GET", "/api/v1/books/"+books[0].Id+"/pages/3/thumbnail", nil); rec.Code != 200 || rec.Header().Get("ETag") == "" {
		t.Fatalf("page thumbnail after change: %d", rec.Code)
	}
	if n, _ := filepath.Glob(filepath.Join(e.data, "page-thumbs", books[0].Id, "*.jpg")); len(n) != 1 || n[0] == oldFiles[0] {
		t.Fatalf("page thumbnail files after change %v (before %v)", n, oldFiles)
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
