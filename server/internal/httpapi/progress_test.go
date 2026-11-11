package httpapi

import (
	"image/color"
	"path/filepath"
	"strings"
	"testing"
)

// progressEnv makes library A with series Alpha (books of 3 and 2 pages) and
// library B with series Beta (one 1-page book), scanned and analysed.
func progressEnv(t *testing.T) (*env, Library, Library) {
	t.Helper()
	e := newEnv(t)
	red := pngBytes(t, 40, 60, color.RGBA{200, 0, 0, 255})
	rootA, rootB := t.TempDir(), t.TempDir()
	writeCBZ(t, filepath.Join(rootA, "Alpha", "Alpha v01.cbz"), map[string][]byte{"1.png": red, "2.png": red, "3.png": red})
	writeCBZ(t, filepath.Join(rootA, "Alpha", "Alpha v02.cbz"), map[string][]byte{"1.png": red, "2.png": red})
	writeCBZ(t, filepath.Join(rootB, "Beta", "Beta v01.cbz"), map[string][]byte{"1.png": red})
	var a, b Library
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "A", "root": rootA, "kind": "manga"}, 201, &a)
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "B", "root": rootB, "kind": "manga"}, 201, &b)
	e.waitTasks()
	return e, a, b
}

// libSeries returns the only series of a library.
func libSeries(t *testing.T, c client, libID string) Series {
	t.Helper()
	var ss []Series
	c.json("GET", "/api/v1/series?libraryId="+libID, nil, 200, &ss)
	if len(ss) != 1 {
		t.Fatalf("library %s has %d series, want 1", libID, len(ss))
	}
	return ss[0]
}

// seriesBooks returns a series' books in number order.
func seriesBooks(t *testing.T, c client, seriesID string) []Book {
	t.Helper()
	var bs []Book
	c.json("GET", "/api/v1/series/"+seriesID+"/books", nil, 200, &bs)
	return bs
}

func TestProgress(t *testing.T) {
	e, a, _ := progressEnv(t)
	books := seriesBooks(t, e.admin, libSeries(t, e.admin, a.Id).Id)
	if len(books) != 2 || books[0].PageCount != 3 {
		t.Fatalf("books %+v", books)
	}
	id := books[0].Id
	get := func() Book {
		var b Book
		e.admin.json("GET", "/api/v1/books/"+id, nil, 200, &b)
		return b
	}

	var p ReadProgress
	e.admin.json("PUT", "/api/v1/books/"+id+"/progress", map[string]any{"page": 2}, 200, &p)
	if p.Page != 2 || p.Completed {
		t.Fatalf("set page 2: %+v", p)
	}
	if b := get(); b.Progress == nil || b.Progress.Page != 2 || b.Progress.Completed {
		t.Fatalf("book after page 2: %+v", b.Progress)
	}

	e.admin.json("PUT", "/api/v1/books/"+id+"/progress", map[string]any{"page": 3}, 200, &p)
	if !p.Completed {
		t.Fatalf("last page should complete by default: %+v", p)
	}
	e.admin.json("PUT", "/api/v1/books/"+id+"/progress", map[string]any{"page": 0}, 400, nil)
	e.admin.json("PUT", "/api/v1/books/"+id+"/progress", map[string]any{"page": 4}, 400, nil)
	e.admin.json("PUT", "/api/v1/books/"+id+"/progress", map[string]any{"page": 3, "completed": false}, 200, &p)
	if p.Completed {
		t.Fatalf("explicit completed=false ignored: %+v", p)
	}

	e.admin.json("DELETE", "/api/v1/books/"+id+"/progress", nil, 204, nil)
	if b := get(); b.Progress != nil {
		t.Fatalf("progress after clear: %+v", b.Progress)
	}
	e.admin.json("DELETE", "/api/v1/books/"+id+"/progress", nil, 204, nil)
	e.admin.json("PUT", "/api/v1/books/nope/progress", map[string]any{"page": 1}, 404, nil)
	e.admin.json("DELETE", "/api/v1/books/nope/progress", nil, 404, nil)
}

func TestProgressWriterOrder(t *testing.T) {
	e, a, _ := progressEnv(t)
	id := seriesBooks(t, e.admin, libSeries(t, e.admin, a.Id).Id)[0].Id
	path := "/api/v1/books/" + id + "/progress"
	put := func(body map[string]any) ReadProgress {
		var p ReadProgress
		e.admin.json("PUT", path, body, 200, &p)
		return p
	}

	// The unload write (seq 2, last page) lands first; the older page-2 write
	// from the same reader arrives later and must not undo it.
	put(map[string]any{"page": 3, "writer": "tab-a", "seq": 2})
	if p := put(map[string]any{"page": 2, "writer": "tab-a", "seq": 1}); p.Page != 3 || !p.Completed {
		t.Fatalf("stale write should answer the stored progress: %+v", p)
	}
	// The same seq again is stale too.
	if p := put(map[string]any{"page": 1, "writer": "tab-a", "seq": 2}); p.Page != 3 {
		t.Fatalf("equal seq applied: %+v", p)
	}
	// Another reader (another device) is never blocked by tab-a's numbers.
	if p := put(map[string]any{"page": 1, "writer": "tab-b", "seq": 1}); p.Page != 1 {
		t.Fatalf("other writer blocked: %+v", p)
	}
	// A write without a writer (mark read on the book page) applies and keeps
	// tab-b's seq, so a late stale tab-b write cannot undo it.
	put(map[string]any{"page": 3})
	if p := put(map[string]any{"page": 1, "writer": "tab-b", "seq": 1}); p.Page != 3 {
		t.Fatalf("late write undid mark read: %+v", p)
	}
	if p := put(map[string]any{"page": 2, "writer": "tab-b", "seq": 2}); p.Page != 2 {
		t.Fatalf("newer write from tab-b refused: %+v", p)
	}
	e.admin.json("PUT", path, map[string]any{"page": 1, "writer": strings.Repeat("x", 65), "seq": 3}, 400, nil)
}

func TestProgressIsolation(t *testing.T) {
	e, a, _ := progressEnv(t)
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "友人", "email": "friend@example.com", "password": "friend pass", "role": "user"}, 201, nil)
	friend := e.login("友人", "friend pass")
	id := seriesBooks(t, e.admin, libSeries(t, e.admin, a.Id).Id)[0].Id

	e.admin.json("PUT", "/api/v1/books/"+id+"/progress", map[string]any{"page": 2}, 200, nil)
	var b Book
	friend.json("GET", "/api/v1/books/"+id, nil, 200, &b)
	if b.Progress != nil {
		t.Fatalf("friend sees admin's progress: %+v", b.Progress)
	}
	friend.json("PUT", "/api/v1/books/"+id+"/progress", map[string]any{"page": 1}, 200, nil)
	friend.json("DELETE", "/api/v1/books/"+id+"/progress", nil, 204, nil)

	b = Book{}
	e.admin.json("GET", "/api/v1/books/"+id, nil, 200, &b)
	if b.Progress == nil || b.Progress.Page != 2 {
		t.Fatalf("admin's progress changed by friend: %+v", b.Progress)
	}
}

func TestMarkSeriesRead(t *testing.T) {
	e, a, _ := progressEnv(t)
	alpha := libSeries(t, e.admin, a.Id)
	seriesPath := "/api/v1/series/" + alpha.Id + "/read-progress"

	e.admin.json("PUT", seriesPath, map[string]any{"completed": true}, 204, nil)
	var s Series
	e.admin.json("GET", "/api/v1/series/"+alpha.Id, nil, 200, &s)
	if s.BooksRead != 2 {
		t.Fatalf("booksRead after mark read = %d, want 2", s.BooksRead)
	}
	books := seriesBooks(t, e.admin, alpha.Id)
	if books[0].Progress == nil || !books[0].Progress.Completed || books[0].Progress.Page != 3 ||
		books[1].Progress == nil || !books[1].Progress.Completed || books[1].Progress.Page != 2 {
		t.Fatalf("progress after mark read: %+v %+v", books[0].Progress, books[1].Progress)
	}

	e.admin.json("PUT", seriesPath, map[string]any{"completed": false}, 204, nil)
	e.admin.json("GET", "/api/v1/series/"+alpha.Id, nil, 200, &s)
	if s.BooksRead != 0 {
		t.Fatalf("booksRead after mark unread = %d, want 0", s.BooksRead)
	}
	e.admin.json("PUT", "/api/v1/series/nope/read-progress", map[string]any{"completed": true}, 404, nil)
}

func TestReaderPrefs(t *testing.T) {
	e, a, _ := progressEnv(t)
	alpha := libSeries(t, e.admin, a.Id)
	book := seriesBooks(t, e.admin, alpha.Id)[0]

	var all ReaderPrefsAll
	e.admin.json("GET", "/api/v1/me/reader-prefs", nil, 200, &all)
	if all.Global.Direction != GlobalReaderPrefsDirectionSeries || all.Global.Spread != GlobalReaderPrefsSpreadOff ||
		all.Global.Fit != GlobalReaderPrefsFitScreen || all.Global.Bg != GlobalReaderPrefsBgBlack {
		t.Fatalf("default global %+v", all.Global)
	}
	if all.Series == nil || all.Books == nil || len(all.Series) != 0 || len(all.Books) != 0 {
		t.Fatalf("default maps %+v %+v", all.Series, all.Books)
	}

	e.admin.json("PUT", "/api/v1/me/reader-prefs/global", map[string]any{"direction": "rtl", "spread": "on", "fit": "width", "bg": "gray"}, 204, nil)
	e.admin.json("PUT", "/api/v1/me/reader-prefs/global", map[string]any{"direction": "rtl", "spread": "on", "fit": "huge", "bg": "gray"}, 400, nil)
	e.admin.json("PUT", "/api/v1/me/reader-prefs/series/"+alpha.Id,
		map[string]any{"direction": "ltr", "spread": "cover", "fit": "height", "bg": "white"}, 204, nil)
	e.admin.json("PUT", "/api/v1/me/reader-prefs/series/"+alpha.Id,
		map[string]any{"direction": "series", "spread": "off", "fit": "screen", "bg": "black"}, 400, nil)
	e.admin.json("PUT", "/api/v1/me/reader-prefs/books/"+book.Id,
		map[string]any{"direction": "vertical", "spread": "off", "fit": "original", "bg": "gray"}, 204, nil)
	e.admin.json("PUT", "/api/v1/me/reader-prefs/series/nope",
		map[string]any{"direction": "ltr", "spread": "off", "fit": "screen", "bg": "black"}, 404, nil)
	e.admin.json("PUT", "/api/v1/me/reader-prefs/books/nope",
		map[string]any{"direction": "ltr", "spread": "off", "fit": "screen", "bg": "black"}, 404, nil)

	all = ReaderPrefsAll{}
	e.admin.json("GET", "/api/v1/me/reader-prefs", nil, 200, &all)
	if all.Global.Direction != GlobalReaderPrefsDirectionRtl || all.Global.Fit != GlobalReaderPrefsFitWidth || all.Global.Bg != GlobalReaderPrefsBgGray {
		t.Fatalf("global %+v", all.Global)
	}
	if sp := all.Series[alpha.Id]; sp.Direction != ReadingDirectionLtr || sp.Spread != ReaderPrefsSpreadCover {
		t.Fatalf("series prefs %+v", sp)
	}
	if bp := all.Books[book.Id]; bp.Direction != ReadingDirectionVertical || bp.Fit != ReaderPrefsFitOriginal {
		t.Fatalf("book prefs %+v", bp)
	}

	e.admin.json("DELETE", "/api/v1/me/reader-prefs/series/"+alpha.Id, nil, 204, nil)
	e.admin.json("DELETE", "/api/v1/me/reader-prefs/series/nope", nil, 404, nil)
	all = ReaderPrefsAll{}
	e.admin.json("GET", "/api/v1/me/reader-prefs", nil, 200, &all)
	if _, ok := all.Series[alpha.Id]; ok {
		t.Fatalf("series prefs survived delete: %+v", all.Series)
	}
	if _, ok := all.Books[book.Id]; !ok {
		t.Fatalf("book prefs lost when series prefs deleted: %+v", all.Books)
	}
}

func TestProgressAndPrefsHiddenLibrary(t *testing.T) {
	e, a, b := progressEnv(t)
	betaSeries := libSeries(t, e.admin, b.Id)
	betaBook := seriesBooks(t, e.admin, betaSeries.Id)[0]
	alphaSeries := libSeries(t, e.admin, a.Id)

	var u User
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "読者", "email": "reader@example.com", "password": "reader pass", "role": "user",
		"allLibraries": false, "libraryIds": []string{a.Id}}, 201, &u)
	r := e.login("読者", "reader pass")

	prefs := map[string]any{"direction": "ltr", "spread": "off", "fit": "screen", "bg": "black"}
	bookPath := "/api/v1/books/" + betaBook.Id
	seriesPath := "/api/v1/series/" + betaSeries.Id
	for _, c := range []struct {
		method, url string
		body        any
	}{
		{"PUT", bookPath + "/progress", map[string]any{"page": 1}},
		{"DELETE", bookPath + "/progress", nil},
		{"PUT", seriesPath + "/read-progress", map[string]any{"completed": true}},
		{"PUT", seriesPath + "/read-progress", map[string]any{"completed": false}},
		{"PUT", "/api/v1/me/reader-prefs/series/" + betaSeries.Id, prefs},
		{"DELETE", "/api/v1/me/reader-prefs/series/" + betaSeries.Id, nil},
		{"PUT", "/api/v1/me/reader-prefs/books/" + betaBook.Id, prefs},
		{"DELETE", "/api/v1/me/reader-prefs/books/" + betaBook.Id, nil},
	} {
		if rec := r.do(c.method, c.url, c.body); rec.Code != 404 {
			t.Errorf("%s %s as restricted user: %d, want 404", c.method, c.url, rec.Code)
		}
	}
	e.admin.json("PUT", bookPath+"/progress", map[string]any{"page": 1}, 200, nil)
	r.json("GET", bookPath, nil, 404, nil)

	// Their own library still works.
	alphaBook := seriesBooks(t, e.admin, alphaSeries.Id)[0]
	r.json("PUT", "/api/v1/books/"+alphaBook.Id+"/progress", map[string]any{"page": 1}, 200, nil)
	r.json("PUT", "/api/v1/me/reader-prefs/series/"+alphaSeries.Id, prefs, 204, nil)

	// A pref row for a hidden series, written directly, must not be listed.
	if _, err := e.db.Exec(`INSERT INTO reader_prefs (user_id, scope, target, prefs) VALUES (?, 'series', ?, ?)`,
		u.Id, betaSeries.Id, `{"direction":"ltr","spread":"off","fit":"screen","bg":"black"}`); err != nil {
		t.Fatal(err)
	}
	var all ReaderPrefsAll
	r.json("GET", "/api/v1/me/reader-prefs", nil, 200, &all)
	if _, ok := all.Series[betaSeries.Id]; ok {
		t.Fatalf("hidden series prefs listed: %+v", all.Series)
	}
	if _, ok := all.Series[alphaSeries.Id]; !ok {
		t.Fatalf("visible series prefs missing: %+v", all.Series)
	}

	// The administrator sees all of it.
	var adminBook Book
	e.admin.json("GET", bookPath, nil, 200, &adminBook)
	if adminBook.Progress == nil || adminBook.Progress.Page != 1 {
		t.Fatalf("admin progress on B: %+v", adminBook.Progress)
	}
	e.admin.json("DELETE", bookPath+"/progress", nil, 204, nil)
}
