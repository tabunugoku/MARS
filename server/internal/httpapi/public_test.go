package httpapi

import (
	"image/color"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverWall(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	red := pngBytes(t, 40, 60, color.RGBA{200, 0, 0, 255})
	writeCBZ(t, filepath.Join(root, "Alpha", "Alpha v01.cbz"), map[string][]byte{"1.png": red})
	var lib Library
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "A", "root": root, "kind": "manga"}, 201, &lib)
	e.waitTasks()
	var series []Series
	e.admin.json("GET", "/api/v1/series", nil, 200, &series)
	id := series[0].Id
	anon := e.anon()

	var ls LoginScreen
	anon.json("GET", "/api/v1/public/login-screen", nil, 200, &ls)
	if ls.Backdrop != Compass || len(ls.Covers) != 0 {
		t.Fatalf("default login screen %+v", ls)
	}
	if rec := anon.do("GET", "/api/v1/public/covers/"+id, nil); rec.Code != 404 {
		t.Fatalf("cover while the wall is off: %d", rec.Code)
	}

	if _, err := e.db.Exec(`INSERT INTO settings (key, value) VALUES ('server', '{"loginBackdrop":"covers"}')`); err != nil {
		t.Fatal(err)
	}
	anon.json("GET", "/api/v1/public/login-screen", nil, 200, &ls)
	if ls.Backdrop != Covers || len(ls.Covers) != 1 || ls.Covers[0].ThumbnailUrl != "/api/v1/public/covers/"+id {
		t.Fatalf("cover wall %+v", ls)
	}
	rec := anon.do("GET", "/api/v1/public/covers/"+id, nil)
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "image/") {
		t.Fatalf("public cover: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	for _, u := range []string{
		"/api/v1/public/covers/nope",
		"/api/v1/public/covers/" + id + "/x",
		"/api/v1/series/" + id + "/thumbnail",
	} {
		if rec := anon.do("GET", u, nil); rec.Code != 404 && rec.Code != 401 {
			t.Errorf("anonymous %s: %d", u, rec.Code)
		}
	}
	if rec := anon.do("GET", "/api/v1/series/"+id+"/thumbnail", nil); rec.Code != 401 {
		t.Errorf("private thumbnail without sign-in: %d", rec.Code)
	}
}

func TestBookCoverWall(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	red := pngBytes(t, 40, 60, color.RGBA{200, 0, 0, 255})
	writeCBZ(t, filepath.Join(root, "Alpha", "Alpha v01.cbz"), map[string][]byte{"1.png": red})
	writeCBZ(t, filepath.Join(root, "Alpha", "Alpha v02.cbz"), map[string][]byte{"1.png": red})
	lib := e.libraryWith("A", root)
	series, books := e.onlySeries(lib.Id)
	if len(books) != 2 {
		t.Fatalf("books %+v", books)
	}
	anon := e.anon()

	if _, err := e.db.Exec(`INSERT INTO settings (key, value) VALUES ('server', '{"loginBackdrop":"bookCovers"}')`); err != nil {
		t.Fatal(err)
	}
	var ls LoginScreen
	anon.json("GET", "/api/v1/public/login-screen", nil, 200, &ls)
	if ls.Backdrop != BookCovers || len(ls.Covers) != len(books) {
		t.Fatalf("book cover wall %+v", ls)
	}
	for _, b := range books {
		want := "/api/v1/public/covers/" + b.Id
		found := false
		for _, c := range ls.Covers {
			if c.ThumbnailUrl == want && c.Title == series.Title+" "+b.Title {
				found = true
			}
		}
		if !found {
			t.Fatalf("book %s not on the wall: %+v", b.Id, ls.Covers)
		}
		rec := anon.do("GET", want, nil)
		if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "image/") {
			t.Fatalf("public book cover: %d %s", rec.Code, rec.Header().Get("Content-Type"))
		}
	}

	// A book without a thumbnail is not on the wall, so its cover is not served.
	hidden := books[1].Id
	if _, err := e.db.Exec(`UPDATE books SET thumbnail = '' WHERE id = ?`, hidden); err != nil {
		t.Fatal(err)
	}
	anon.json("GET", "/api/v1/public/login-screen", nil, 200, &ls)
	if len(ls.Covers) != 1 {
		t.Fatalf("wall after hiding a book: %+v", ls.Covers)
	}
	if rec := anon.do("GET", "/api/v1/public/covers/"+hidden, nil); rec.Code != 404 {
		t.Fatalf("cover of a book off the wall: %d", rec.Code)
	}

	// With the series wall, a book id is not on the wall.
	if _, err := e.db.Exec(`UPDATE settings SET value = '{"loginBackdrop":"covers"}' WHERE key = 'server'`); err != nil {
		t.Fatal(err)
	}
	if rec := anon.do("GET", "/api/v1/public/covers/"+books[0].Id, nil); rec.Code != 404 {
		t.Fatalf("book id while the series wall is on: %d", rec.Code)
	}
}
