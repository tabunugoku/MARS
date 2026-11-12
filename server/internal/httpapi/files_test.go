package httpapi

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// libraryWith registers a library at root as the admin and waits for its
// scan and analysis.
func (e *env) libraryWith(name, root string) Library {
	e.t.Helper()
	var l Library
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": name, "root": root, "kind": "manga"}, 201, &l)
	e.waitTasks()
	return l
}

func (e *env) onlySeries(libraryID string) (Series, []Book) {
	e.t.Helper()
	var ss []Series
	e.admin.json("GET", "/api/v1/series?libraryId="+libraryID, nil, 200, &ss)
	if len(ss) != 1 {
		e.t.Fatalf("series %+v", ss)
	}
	var bs []Book
	e.admin.json("GET", "/api/v1/series/"+ss[0].Id+"/books", nil, 200, &bs)
	return ss[0], bs
}

func TestDownloadPermission(t *testing.T) {
	e := newEnv(t)
	rootA, rootB := t.TempDir(), t.TempDir()
	red := pngBytes(t, 20, 30, color.RGBA{200, 0, 0, 255})
	writeCBZ(t, filepath.Join(rootA, "A", "A v01.cbz"), map[string][]byte{"1.png": red})
	writeCBZ(t, filepath.Join(rootB, "B", "B v01.cbz"), map[string][]byte{"1.png": red})
	a := e.libraryWith("A", rootA)
	b := e.libraryWith("B", rootB)
	sa, ba := e.onlySeries(a.Id)
	sbs, bb := e.onlySeries(b.Id)

	var u User
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "読者", "email": "r@example.com", "password": "reader pass",
		"role": "user", "canDownload": false, "allLibraries": false, "libraryIds": []string{a.Id}}, 201, &u)
	r := e.login("読者", "reader pass")

	mine := []string{"/api/v1/books/" + ba[0].Id + "/file", "/api/v1/series/" + sa.Id + "/file"}
	hidden := []string{"/api/v1/books/" + bb[0].Id + "/file", "/api/v1/series/" + sbs.Id + "/file"}
	for _, p := range mine {
		if rec := r.do("GET", p, nil); rec.Code != 403 {
			t.Errorf("%s without permission: %d, want 403", p, rec.Code)
		}
	}
	for _, p := range hidden { // outside the user's libraries: 404 before any permission check
		if rec := r.do("GET", p, nil); rec.Code != 404 {
			t.Errorf("%s outside scope: %d, want 404", p, rec.Code)
		}
	}
	if rec := e.anon().do("GET", mine[0], nil); rec.Code != 401 {
		t.Errorf("signed out: %d", rec.Code)
	}

	// The administrator always may, even with the flag off on their own row.
	if _, err := e.db.Exec(`UPDATE users SET can_download = 0 WHERE role = 'admin'`); err != nil {
		t.Fatal(err)
	}
	for _, p := range append(mine, hidden...) {
		if rec := e.admin.do("GET", p, nil); rec.Code != 200 {
			t.Errorf("admin %s: %d", p, rec.Code)
		}
	}

	e.admin.json("PATCH", "/api/v1/users/"+u.Id, map[string]any{"canDownload": true}, 200, nil)
	rec := r.do("GET", mine[0], nil)
	want, _ := os.ReadFile(filepath.Join(rootA, "A", "A v01.cbz"))
	if rec.Code != 200 || !bytes.Equal(rec.Body.Bytes(), want) {
		t.Fatalf("book download: %d, %d bytes", rec.Code, rec.Body.Len())
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="A v01.cbz"; filename*=UTF-8''A%20v01.cbz` {
		t.Fatalf("content-disposition %q", cd)
	}
	// Range works, so a broken download can resume.
	rec = r.do("GET", mine[0], nil, "Range", "bytes=0-9")
	if rec.Code != 206 || !bytes.Equal(rec.Body.Bytes(), want[:10]) {
		t.Fatalf("range: %d", rec.Code)
	}
}

func TestSeriesZip(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	dir := filepath.Join(root, "夜更けの測量士")
	red := pngBytes(t, 20, 30, color.RGBA{200, 0, 0, 255})
	blue := pngBytes(t, 20, 30, color.RGBA{0, 0, 200, 255})
	writeCBZ(t, filepath.Join(dir, "第02巻.cbz"), map[string][]byte{"1.png": blue, "2.png": red})
	writeCBZ(t, filepath.Join(dir, "第01巻.cbz"), map[string][]byte{"1.png": red})
	writeCBZ(t, filepath.Join(dir, "第03巻.cbz"), map[string][]byte{"1.png": red, "2.png": blue})
	l := e.libraryWith("漫画", root)

	// Book 3 goes to the trash: a ZIP must leave it out.
	if err := os.Remove(filepath.Join(dir, "第03巻.cbz")); err != nil {
		t.Fatal(err)
	}
	e.admin.json("POST", "/api/v1/libraries/"+l.Id+"/scan", nil, 202, nil)
	e.waitTasks()
	s, books := e.onlySeries(l.Id)
	if len(books) != 2 {
		t.Fatalf("books %+v", books)
	}

	rec := e.admin.do("GET", "/api/v1/series/"+s.Id+"/file", nil)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("zip: %d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Content-Length") != "" {
		t.Fatalf("a streamed ZIP has no Content-Length")
	}
	cd := rec.Header().Get("Content-Disposition")
	wantCD := `attachment; filename="_______.zip"; filename*=UTF-8''%E5%A4%9C%E6%9B%B4%E3%81%91%E3%81%AE%E6%B8%AC%E9%87%8F%E5%A3%AB.zip`
	if cd != wantCD {
		t.Fatalf("content-disposition %q", cd)
	}

	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
		if f.Method != zip.Store {
			t.Errorf("%s: method %d, want stored", f.Name, f.Method)
		}
		if f.Flags&0x800 == 0 {
			t.Errorf("%s: UTF-8 name flag not set", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(rc)
		rc.Close()
		want, _ := os.ReadFile(filepath.Join(dir, f.Name))
		if sha256.Sum256(got) != sha256.Sum256(want) {
			t.Errorf("%s: bytes differ from the file on disk", f.Name)
		}
	}
	if strings.Join(names, ",") != "第01巻.cbz,第02巻.cbz" {
		t.Fatalf("entries %v", names)
	}
}

func TestZipNamesAndDisposition(t *testing.T) {
	got := zipNames([]Book{{Path: "a/x.cbz"}, {Path: "b/x.cbz"}, {Path: "c/X.cbz"}, {Path: "y.cbz"}})
	if strings.Join(got, "|") != "x.cbz|x (2).cbz|X (3).cbz|y.cbz" {
		t.Fatalf("names %v", got)
	}
	cd := attachment(`it's "a"/b%.zip`)
	if cd != `attachment; filename="it's _a__b_.zip"; filename*=UTF-8''it%27s%20%22a%22_b%25.zip` {
		t.Fatalf("disposition %q", cd)
	}
}

// TestDeepScan changes a book's ComicInfo without changing its size or
// modification time. A normal scan sees nothing to do; a deep scan reads
// every book again and picks the change up.
func TestDeepScan(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	p := filepath.Join(root, "S", "S v01.cbz")
	red := pngBytes(t, 20, 30, color.RGBA{200, 0, 0, 255})
	info := func(title string) []byte {
		return []byte(`<?xml version="1.0"?><ComicInfo><Title>` + title + `</Title><Number>1</Number></ComicInfo>`)
	}
	writeCBZ(t, p, map[string][]byte{"1.png": red, "ComicInfo.xml": info("一回目")})
	l := e.libraryWith("漫画", root)
	_, books := e.onlySeries(l.Id)
	if books[0].Title != "一回目" {
		t.Fatalf("first title %q", books[0].Title)
	}
	st, _ := os.Stat(p)

	// Same length title, same size; mtime put back.
	writeCBZ(t, p, map[string][]byte{"1.png": red, "ComicInfo.xml": info("二回目")})
	if st2, _ := os.Stat(p); st2.Size() != st.Size() {
		t.Fatalf("fixture size changed: %d -> %d", st.Size(), st2.Size())
	}
	if err := os.Chtimes(p, st.ModTime(), st.ModTime()); err != nil {
		t.Fatal(err)
	}
	// Damage the stored analysis too: a deep scan must rebuild it.
	if _, err := e.db.Exec(`DELETE FROM pages WHERE book_id = ?`, books[0].Id); err != nil {
		t.Fatal(err)
	}

	e.admin.json("POST", "/api/v1/libraries/"+l.Id+"/scan", nil, 202, nil)
	e.waitTasks()
	var b Book
	e.admin.json("GET", "/api/v1/books/"+books[0].Id, nil, 200, &b)
	if b.Title != "一回目" {
		t.Fatalf("a normal scan re-read an unchanged book: %q", b.Title)
	}

	var tk Task
	e.admin.json("POST", "/api/v1/libraries/"+l.Id+"/scan?deep=true", nil, 202, &tk)
	if tk.Kind != Scan || !strings.Contains(tk.Label, "詳細スキャン") {
		t.Fatalf("task %+v", tk)
	}
	e.waitTasks()
	e.admin.json("GET", "/api/v1/books/"+books[0].Id, nil, 200, &b)
	if b.Title != "二回目" || b.MediaStatus != MediaStatusReady {
		t.Fatalf("after deep scan: %q %s", b.Title, b.MediaStatus)
	}
	var pages []Page
	e.admin.json("GET", "/api/v1/books/"+b.Id+"/pages", nil, 200, &pages)
	if len(pages) != 1 {
		t.Fatalf("pages after deep scan: %+v", pages)
	}

	// Non-admins may not start one.
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "読者", "email": "r@example.com", "password": "reader pass", "role": "user"}, 201, nil)
	r := e.login("読者", "reader pass")
	r.json("POST", "/api/v1/libraries/"+l.Id+"/scan?deep=true", nil, 403, nil)
}

// thumbColor decodes a thumbnail and returns its centre pixel.
func thumbColor(t *testing.T, c client, url string) (r, g, b uint32) {
	t.Helper()
	rec := c.do("GET", url, nil)
	if rec.Code != 200 {
		t.Fatalf("thumbnail %s: %d", url, rec.Code)
	}
	img, err := jpeg.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	bd := img.Bounds()
	r, g, b, _ = img.At(bd.Dx()/2, bd.Dy()/2).RGBA()
	return r >> 8, g >> 8, b >> 8
}

func TestCoverAndThumbnails(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	red := pngBytes(t, 20, 30, color.RGBA{220, 0, 0, 255})
	blue := pngBytes(t, 20, 30, color.RGBA{0, 0, 220, 255})
	writeCBZ(t, filepath.Join(root, "S", "S v01.cbz"), map[string][]byte{"1.png": red, "2.png": blue})
	l := e.libraryWith("漫画", root)
	s, books := e.onlySeries(l.Id)
	id := books[0].Id
	if books[0].CoverPage != 1 {
		t.Fatalf("cover page %d", books[0].CoverPage)
	}
	if r, _, b := thumbColor(t, e.admin, books[0].ThumbnailUrl); r < 150 || b > 80 {
		t.Fatalf("first cover not red: %d %d", r, b)
	}

	e.admin.json("PUT", "/api/v1/books/"+id+"/cover", map[string]any{"page": 3}, 400, nil)
	e.admin.json("PUT", "/api/v1/books/"+id+"/cover", map[string]any{"page": 0}, 400, nil)
	var b Book
	e.admin.json("PUT", "/api/v1/books/"+id+"/cover", map[string]any{"page": 2}, 200, &b)
	if b.CoverPage != 2 {
		t.Fatalf("cover page after set: %d", b.CoverPage)
	}
	if r, _, bl := thumbColor(t, e.admin, b.ThumbnailUrl); bl < 150 || r > 80 {
		t.Fatalf("chosen cover not blue: %d %d", r, bl)
	}

	// The choice survives a deep scan, which analyses the book again.
	e.admin.json("POST", "/api/v1/libraries/"+l.Id+"/scan?deep=true", nil, 202, nil)
	e.waitTasks()
	e.admin.json("GET", "/api/v1/books/"+id, nil, 200, &b)
	if r, _, bl := thumbColor(t, e.admin, b.ThumbnailUrl); b.CoverPage != 2 || bl < 150 || r > 80 {
		t.Fatalf("deep scan undid the chosen cover: page %d, %d %d", b.CoverPage, r, bl)
	}

	// Regeneration brings back a lost thumbnail, per book, series and library.
	if _, err := e.db.Exec(`UPDATE books SET thumbnail = '' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if rec := e.admin.do("GET", b.ThumbnailUrl, nil); rec.Code != 404 {
		t.Fatalf("thumbnail before regeneration: %d", rec.Code)
	}
	e.admin.json("POST", "/api/v1/books/"+id+"/regenerate-thumbnail", nil, 204, nil)
	if r, _, bl := thumbColor(t, e.admin, b.ThumbnailUrl); bl < 150 || r > 80 {
		t.Fatalf("regenerated book cover not from page 2: %d %d", r, bl)
	}
	e.admin.json("POST", "/api/v1/books/nope/regenerate-thumbnail", nil, 404, nil)
	var tk Task
	e.admin.json("POST", "/api/v1/series/"+s.Id+"/regenerate-thumbnails", nil, 202, &tk)
	if tk.Kind != Thumbnail {
		t.Fatalf("series task %+v", tk)
	}
	e.waitTasks()
	e.admin.json("POST", "/api/v1/libraries/"+l.Id+"/regenerate-thumbnails", nil, 202, &tk)
	e.waitTasks()
	if r, _, bl := thumbColor(t, e.admin, b.ThumbnailUrl); bl < 150 || r > 80 {
		t.Fatalf("regenerated cover not from page 2: %d %d", r, bl)
	}
	e.admin.json("POST", "/api/v1/series/nope/regenerate-thumbnails", nil, 404, nil)
	e.admin.json("POST", "/api/v1/libraries/nope/regenerate-thumbnails", nil, 404, nil)

	// All four are for administrators only.
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "読者", "email": "r@example.com", "password": "reader pass", "role": "user"}, 201, nil)
	r := e.login("読者", "reader pass")
	r.json("PUT", "/api/v1/books/"+id+"/cover", map[string]any{"page": 1}, 403, nil)
	r.json("POST", "/api/v1/books/"+id+"/regenerate-thumbnail", nil, 403, nil)
	r.json("POST", "/api/v1/series/"+s.Id+"/regenerate-thumbnails", nil, 403, nil)
	r.json("POST", "/api/v1/libraries/"+l.Id+"/regenerate-thumbnails", nil, 403, nil)
}

// Two series' thumbnail tasks queued back to back are two tasks, not one.
func TestThumbnailTasksPerSeries(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	red := pngBytes(t, 20, 30, color.RGBA{220, 0, 0, 255})
	writeCBZ(t, filepath.Join(root, "A", "A v01.cbz"), map[string][]byte{"1.png": red})
	writeCBZ(t, filepath.Join(root, "B", "B v01.cbz"), map[string][]byte{"1.png": red})
	l := e.libraryWith("漫画", root)
	var ss []Series
	e.admin.json("GET", "/api/v1/series?libraryId="+l.Id, nil, 200, &ss)
	if len(ss) != 2 {
		t.Fatalf("series %+v", ss)
	}
	var t1, t2 Task
	e.admin.json("POST", "/api/v1/series/"+ss[0].Id+"/regenerate-thumbnails", nil, 202, &t1)
	e.admin.json("POST", "/api/v1/series/"+ss[1].Id+"/regenerate-thumbnails", nil, 202, &t2)
	if t1.Id == t2.Id {
		t.Fatalf("series B folded into series A's task")
	}
	e.waitTasks()
}
