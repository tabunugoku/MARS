package httpapi

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// trashFixture makes a scanned library with the folder series Gone (two
// books), the folder series Kept (three books) and the loose file Loose.cbz,
// a series of its own. Nothing is trashed yet.
func trashFixture(t *testing.T) (*env, Library) {
	t.Helper()
	e := newEnv(t)
	root := t.TempDir()
	img := pngBytes(t, 40, 60, color.RGBA{10, 10, 10, 255})
	for _, p := range []string{"Gone/Gone v01.cbz", "Gone/Gone v02.cbz", "Kept/Kept v01.cbz", "Kept/Kept v02.cbz", "Kept/Kept v03.cbz", "Loose.cbz"} {
		writeCBZ(t, filepath.Join(root, filepath.FromSlash(p)), map[string][]byte{"1.png": img})
	}
	var l Library
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "T", "root": root, "kind": "manga"}, 201, &l)
	e.waitTasks()
	return e, l
}

// trashSeriesID returns the id of the series at dir (folder series) or, with
// dir empty, of the one-shot file oneshot.
func trashSeriesID(t *testing.T, e *env, dir, oneshot string) string {
	t.Helper()
	var id string
	if err := e.db.QueryRow(`SELECT id FROM series WHERE dir = ? AND oneshot_file = ?`, dir, oneshot).Scan(&id); err != nil {
		t.Fatalf("series %q %q: %v", dir, oneshot, err)
	}
	return id
}

// trashBookID returns the id of the book stored at the slash-form path.
func trashBookID(t *testing.T, e *env, path string) string {
	t.Helper()
	var id string
	if err := e.db.QueryRow(`SELECT id FROM books WHERE path = ?`, path).Scan(&id); err != nil {
		t.Fatalf("book %q: %v", path, err)
	}
	return id
}

// trashRow marks one row trashed directly in the database. The files stay,
// so a scan would restore it; the tests do not scan afterwards.
func trashRow(t *testing.T, e *env, table, id string) {
	t.Helper()
	if _, err := e.db.Exec(`UPDATE `+table+` SET deleted_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
		t.Fatal(err)
	}
}

// trashCount counts rows matching a query with one argument.
func trashCount(t *testing.T, e *env, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestListTrash(t *testing.T) {
	e, _ := trashFixture(t)
	gone := trashSeriesID(t, e, "Gone", "")
	loose := trashSeriesID(t, e, "", "Loose.cbz")
	kept := trashSeriesID(t, e, "Kept", "")
	trashRow(t, e, "series", gone)
	trashRow(t, e, "series", loose)
	// A book of the trashed series is trashed too: it is still not listed.
	trashRow(t, e, "books", trashBookID(t, e, "Gone/Gone v01.cbz"))
	keptV02 := trashBookID(t, e, "Kept/Kept v02.cbz")
	trashRow(t, e, "books", keptV02)
	// A book with no title falls back to its file name.
	if _, err := e.db.Exec(`UPDATE books SET title = '' WHERE id = ?`, keptV02); err != nil {
		t.Fatal(err)
	}

	var tr TrashList
	e.admin.json("GET", "/api/v1/trash", nil, 200, &tr)
	if len(tr.Series) != 2 || len(tr.Books) != 1 {
		t.Fatalf("trash %+v, want 2 series and 1 book", tr)
	}
	for _, s := range tr.Series {
		switch s.Id {
		case gone:
			if s.Path != "Gone" || s.BookCount != 2 || s.Title == "" {
				t.Errorf("trashed folder series %+v, want path Gone with 2 books", s)
			}
		case loose:
			if s.Path != "Loose.cbz" || s.BookCount != 1 {
				t.Errorf("trashed one-shot %+v, want path Loose.cbz with 1 book", s)
			}
		default:
			t.Errorf("unexpected trashed series %+v", s)
		}
	}
	b := tr.Books[0]
	if b.Id != keptV02 || b.Path != "Kept/Kept v02.cbz" || b.SeriesId != kept || b.SeriesTitle == "" {
		t.Errorf("trashed book %+v", b)
	}
	if b.Title != "Kept v02.cbz" {
		t.Errorf("title %q, want the file name", b.Title)
	}
	if b.DeletedAt.IsZero() {
		t.Errorf("deletedAt not read")
	}
}

func TestListTrashEmptyArrays(t *testing.T) {
	e, _ := trashFixture(t)
	rec := e.admin.do("GET", "/api/v1/trash", nil)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"books":[]`) || !strings.Contains(body, `"series":[]`) {
		t.Fatalf("empty trash body %s, want empty arrays", body)
	}
}

func TestDeleteTrashedNeverRemovesLiveRows(t *testing.T) {
	e, _ := trashFixture(t)
	kept := trashSeriesID(t, e, "Kept", "")
	keptV01 := trashBookID(t, e, "Kept/Kept v01.cbz")

	// Live series and live book: not trashed, so 404 and nothing removed.
	e.admin.json("DELETE", "/api/v1/trash/series/"+kept, nil, 404, nil)
	e.admin.json("DELETE", "/api/v1/trash/books/"+keptV01, nil, 404, nil)
	// Ids of the wrong kind: a book id on the series route, a series id on the book route.
	e.admin.json("DELETE", "/api/v1/trash/series/"+keptV01, nil, 404, nil)
	e.admin.json("DELETE", "/api/v1/trash/books/"+kept, nil, 404, nil)
	e.admin.json("DELETE", "/api/v1/trash/series/nope", nil, 404, nil)
	if n := trashCount(t, e, `SELECT count(*) FROM series`); n != 3 {
		t.Fatalf("series rows %d after 404s, want 3", n)
	}
	if n := trashCount(t, e, `SELECT count(*) FROM books`); n != 6 {
		t.Fatalf("book rows %d after 404s, want 6", n)
	}
}

func TestDeleteTrashedBookRemovesOnlyIt(t *testing.T) {
	e, _ := trashFixture(t)
	kept := trashSeriesID(t, e, "Kept", "")
	keptV01 := trashBookID(t, e, "Kept/Kept v01.cbz")
	keptV02 := trashBookID(t, e, "Kept/Kept v02.cbz")
	trashRow(t, e, "books", keptV02)

	e.admin.json("DELETE", "/api/v1/trash/books/"+keptV02, nil, 204, nil)
	if n := trashCount(t, e, `SELECT count(*) FROM books WHERE id = ?`, keptV02); n != 0 {
		t.Fatalf("trashed book still there")
	}
	if n := trashCount(t, e, `SELECT count(*) FROM books WHERE id = ?`, keptV01); n != 1 {
		t.Fatalf("live sibling removed")
	}
	if n := trashCount(t, e, `SELECT count(*) FROM series WHERE id = ?`, kept); n != 1 {
		t.Fatalf("series removed with its book")
	}
	// Gone now: a second delete is a 404.
	e.admin.json("DELETE", "/api/v1/trash/books/"+keptV02, nil, 404, nil)
}

func TestDeleteTrashedSeriesRemovesItsBooksAndCovers(t *testing.T) {
	e, _ := trashFixture(t)
	gone := trashSeriesID(t, e, "Gone", "")
	goneV01 := trashBookID(t, e, "Gone/Gone v01.cbz")
	keptV01 := trashBookID(t, e, "Kept/Kept v01.cbz")

	// Covers of a book of the series that goes, and of a book of a live series
	// that must stay.
	storeThumb(t, e, goneV01, "trash-gone-test.webp", []byte("x"))
	storeThumb(t, e, keptV01, "trash-kept-test.webp", []byte("x"))
	thumbs := filepath.Join(e.data, "thumbs")
	trashRow(t, e, "series", gone)

	e.admin.json("DELETE", "/api/v1/trash/series/"+gone, nil, 204, nil)
	if n := trashCount(t, e, `SELECT count(*) FROM series WHERE id = ?`, gone); n != 0 {
		t.Fatalf("trashed series still there")
	}
	if n := trashCount(t, e, `SELECT count(*) FROM books WHERE series_id = ?`, gone); n != 0 {
		t.Fatalf("books of the removed series still there: %d", n)
	}
	if n := trashCount(t, e, `SELECT count(*) FROM books`); n != 4 {
		t.Fatalf("book rows %d, want the 4 of the live series and Loose", n)
	}
	if n := trashCount(t, e, `SELECT count(*) FROM search_rows WHERE kind = 'book' AND ref = ?`, goneV01); n != 0 {
		t.Fatalf("search rows of a removed book remain")
	}
	if n := trashCount(t, e, `SELECT count(*) FROM search_rows WHERE kind = 'book' AND ref = ?`, keptV01); n == 0 {
		t.Fatalf("search rows of a live book were removed")
	}
	if _, err := os.Stat(filepath.Join(thumbs, "trash-gone-test.webp")); !os.IsNotExist(err) {
		t.Errorf("cover of a removed book still on disk")
	}
	if _, err := os.Stat(filepath.Join(thumbs, "trash-kept-test.webp")); err != nil {
		t.Errorf("cover of a live book was removed: %v", err)
	}
	e.admin.json("DELETE", "/api/v1/trash/series/"+gone, nil, 404, nil)
}

func TestTrashRoutesRefuseNonAdmin(t *testing.T) {
	e, l := trashFixture(t)
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "読者", "email": "reader@example.com", "password": "reader pass", "role": "user",
		"allLibraries": false, "libraryIds": []string{l.Id}}, 201, nil)
	r := e.login("読者", "reader pass")
	gone := trashSeriesID(t, e, "Gone", "")
	keptV02 := trashBookID(t, e, "Kept/Kept v02.cbz")
	trashRow(t, e, "series", gone)
	trashRow(t, e, "books", keptV02)

	r.json("GET", "/api/v1/trash", nil, 403, nil)
	r.json("DELETE", "/api/v1/trash/series/"+gone, nil, 403, nil)
	r.json("DELETE", "/api/v1/trash/books/"+keptV02, nil, 403, nil)
	if n := trashCount(t, e, `SELECT count(*) FROM series WHERE id = ?`, gone); n != 1 {
		t.Fatalf("refused request removed the series")
	}
	if n := trashCount(t, e, `SELECT count(*) FROM books WHERE id = ?`, keptV02); n != 1 {
		t.Fatalf("refused request removed the book")
	}
}

func TestEmptyTrashStillWorks(t *testing.T) {
	e, _ := trashFixture(t)
	gone := trashSeriesID(t, e, "Gone", "")
	keptV01 := trashBookID(t, e, "Kept/Kept v01.cbz")
	keptV02 := trashBookID(t, e, "Kept/Kept v02.cbz")
	trashRow(t, e, "series", gone)
	trashRow(t, e, "books", keptV02)

	var res TrashResult
	e.admin.json("POST", "/api/v1/trash/empty", nil, 200, &res)
	// The two books of Gone go by cascade and are not counted.
	if res.Books != 1 || res.Series != 1 {
		t.Fatalf("empty trash %+v, want books 1 series 1", res)
	}
	if n := trashCount(t, e, `SELECT count(*) FROM books WHERE id = ?`, keptV01); n != 1 {
		t.Fatalf("live book removed by emptying the trash")
	}
	if n := trashCount(t, e, `SELECT count(*) FROM books WHERE series_id = ?`, gone); n != 0 {
		t.Fatalf("books of the emptied series remain")
	}
	rec := e.admin.do("GET", "/api/v1/trash", nil)
	if !strings.Contains(rec.Body.String(), `"books":[]`) || !strings.Contains(rec.Body.String(), `"series":[]`) {
		t.Fatalf("trash after emptying: %s", rec.Body)
	}
}
