package httpapi

import (
	"bytes"
	"encoding/json"
	"image/color"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/tabunugoku/cominavi/server/internal/auth"
)

// komgaProgressClient calls the Komga-shaped API the way Mihon does: Basic
// credentials, or only the cn_compat cookie when user is empty.
type komgaProgressClient struct {
	t      *testing.T
	h      http.Handler
	user   string
	pass   string
	cookie string
}

func (c komgaProgressClient) do(method, url, body string) *httptest.ResponseRecorder {
	c.t.Helper()
	var r io.Reader
	if body != "" {
		r = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, url, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.user != "" {
		req.SetBasicAuth(c.user, c.pass)
	}
	if c.cookie != "" {
		req.AddCookie(&http.Cookie{Name: compatCookie, Value: c.cookie})
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	return rec
}

// want sends the request and fails unless the status is code.
func (c komgaProgressClient) want(method, url, body string, code int) *httptest.ResponseRecorder {
	c.t.Helper()
	rec := c.do(method, url, body)
	if rec.Code != code {
		c.t.Fatalf("%s %s %s: status %d, want %d: %s", method, url, body, rec.Code, code, rec.Body)
	}
	return rec
}

func (c komgaProgressClient) getJSON(url string, out any) {
	c.t.Helper()
	rec := c.want("GET", url, "", http.StatusOK)
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		c.t.Fatalf("GET %s: %v", url, err)
	}
}

// komgaProgressFix is a library A (series Alpha, three books) and library B
// (series Beta, one book). The reader may see only A; the other user sees
// both. The Komga-shaped API is switched on.
type komgaProgressFix struct {
	t        *testing.T
	e        *env
	compat   http.Handler
	reader   client
	other    client
	readerID string
	otherID  string
	alpha    Series
	alphaBks []Book
	beta     Series
	betaBks  []Book
}

func newKomgaProgressFix(t *testing.T) *komgaProgressFix {
	t.Helper()
	e := newEnv(t)
	red := pngBytes(t, 20, 30, color.RGBA{200, 0, 0, 255})
	rootA, rootB := t.TempDir(), t.TempDir()
	for _, v := range []string{"v01", "v02", "v03"} {
		writeCBZ(t, filepath.Join(rootA, "Alpha", "Alpha "+v+".cbz"), map[string][]byte{"1.png": red})
	}
	writeCBZ(t, filepath.Join(rootB, "Beta", "Beta v01.cbz"), map[string][]byte{"1.png": red})
	libA := e.libraryWith("A", rootA)
	libB := e.libraryWith("B", rootB)

	f := &komgaProgressFix{t: t, e: e}
	var u User
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "読者", "email": "reader@example.com", "password": "reader pass",
		"role": "user", "allLibraries": false, "libraryIds": []string{libA.Id}}, 201, &u)
	f.readerID = u.Id
	f.reader = e.login("reader@example.com", "reader pass")
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "他人", "email": "other@example.com", "password": "other pass",
		"role": "user", "allLibraries": true}, 201, &u)
	f.otherID = u.Id
	f.other = e.login("other@example.com", "other pass")

	e.admin.json("PATCH", "/api/v1/settings", map[string]any{"komgaApiEnabled": true}, 200, nil)
	f.compat = NewCompat(Deps{DB: e.db, Version: "test", Auth: auth.Store{DB: e.db}, Limiter: &auth.Limiter{}, Events: e.hub, Gate: e.gate})

	f.alpha = f.seriesIn(libA.Id, "Alpha")
	f.alphaBks = f.booksIn(f.alpha.Id)
	f.beta = f.seriesIn(libB.Id, "Beta")
	f.betaBks = f.booksIn(f.beta.Id)
	if len(f.alphaBks) != 3 || len(f.betaBks) != 1 {
		t.Fatalf("books: alpha %+v beta %+v", f.alphaBks, f.betaBks)
	}
	return f
}

// as returns a Komga-API caller with the reader's (or other user's) Basic
// credentials.
func (f *komgaProgressFix) as(name, pass string) komgaProgressClient {
	return komgaProgressClient{t: f.t, h: f.compat, user: name, pass: pass}
}

func (f *komgaProgressFix) seriesIn(libID, title string) Series {
	f.t.Helper()
	var list []Series
	f.e.admin.json("GET", "/api/v1/series?libraryId="+libID, nil, 200, &list)
	for _, s := range list {
		if s.Title == title {
			return s
		}
	}
	f.t.Fatalf("series %q not in %s: %+v", title, libID, list)
	return Series{}
}

func (f *komgaProgressFix) booksIn(seriesID string) []Book {
	f.t.Helper()
	var list []Book
	f.e.admin.json("GET", "/api/v1/series/"+seriesID+"/books", nil, 200, &list)
	return list
}

// readCount is how many books the user has completed in the database.
func (f *komgaProgressFix) readCount(userID string, books ...Book) int {
	f.t.Helper()
	n := 0
	for _, b := range books {
		var c int
		if err := f.e.db.QueryRow(`SELECT count(*) FROM read_progress WHERE user_id = ? AND book_id = ? AND completed = 1`,
			userID, b.Id).Scan(&c); err != nil {
			f.t.Fatal(err)
		}
		n += c
	}
	return n
}

// completedInAPI reads a book through the main API as the given user.
func (f *komgaProgressFix) completedInAPI(c client, b Book) bool {
	f.t.Helper()
	var got Book
	c.json("GET", "/api/v1/books/"+b.Id, nil, 200, &got)
	return got.Progress != nil && got.Progress.Completed
}

func TestKomgaProgressSeriesTachiyomi(t *testing.T) {
	f := newKomgaProgressFix(t)
	rd := f.as("reader@example.com", "reader pass")
	url := "/komga/api/v2/series/" + f.alpha.Id + "/read-progress/tachiyomi"

	var p komgaTachiProgressV2
	rd.getJSON(url, &p)
	if p.BooksCount != 3 || p.BooksReadCount != 0 || p.BooksUnreadCount != 3 || p.BooksInProgressCount != 0 ||
		p.LastReadContinuousNumberSort != 0 || p.MaxNumberSort != 3 {
		t.Fatalf("fresh series: %+v", p)
	}

	rd.want("PUT", url, `{"lastBookNumberSortRead":1}`, http.StatusNoContent)
	rd.getJSON(url, &p)
	if p.BooksReadCount != 1 || p.BooksUnreadCount != 2 || p.LastReadContinuousNumberSort != 1 {
		t.Fatalf("after marking 1: %+v", p)
	}
	if !f.completedInAPI(f.reader, f.alphaBks[0]) {
		t.Fatal("book 1 not completed for the reader in the main API")
	}
	if f.completedInAPI(f.reader, f.alphaBks[1]) {
		t.Fatal("book 2 completed by a write up to 1")
	}
	if f.completedInAPI(f.other, f.alphaBks[0]) {
		t.Fatal("book 1 completed for another user")
	}

	rd.want("PUT", url, `{"lastBookNumberSortRead":2.5}`, http.StatusNoContent)
	rd.getJSON(url, &p)
	if p.BooksReadCount != 2 || p.LastReadContinuousNumberSort != 2 || p.BooksUnreadCount != 1 {
		t.Fatalf("after marking up to 2.5: %+v", p)
	}
	if f.completedInAPI(f.reader, f.alphaBks[2]) {
		t.Fatal("book 3 marked by a write up to 2.5")
	}

	rd.want("GET", "/komga/api/v2/series/nope/read-progress/tachiyomi", "", http.StatusNotFound)
}

func TestKomgaProgressReadListTachiyomi(t *testing.T) {
	f := newKomgaProgressFix(t)
	rd := f.as("reader@example.com", "reader pass")
	var l ReadList
	f.e.admin.json("POST", "/api/v1/read-lists", map[string]any{"name": "順番", "bookIds": []string{f.alphaBks[1].Id, f.alphaBks[0].Id}}, 201, &l)
	url := "/komga/api/v1/readlists/" + l.Id + "/read-progress/tachiyomi"

	var p komgaTachiProgressV1
	rd.getJSON(url, &p)
	if p.BooksCount != 2 || p.BooksReadCount != 0 || p.LastReadContinuousIndex != 0 {
		t.Fatalf("fresh list: %+v", p)
	}

	rd.want("PUT", url, `{"lastBookRead":1}`, http.StatusNoContent)
	rd.getJSON(url, &p)
	if p.BooksReadCount != 1 || p.LastReadContinuousIndex != 1 {
		t.Fatalf("after first listed: %+v", p)
	}
	if !f.completedInAPI(f.reader, f.alphaBks[1]) {
		t.Fatal("first listed book not completed")
	}
	if f.completedInAPI(f.reader, f.alphaBks[0]) {
		t.Fatal("second listed book completed by lastBookRead=1")
	}
	if f.completedInAPI(f.other, f.alphaBks[1]) {
		t.Fatal("first listed book completed for another user")
	}

	rd.want("PUT", url, `{"lastBookRead":2}`, http.StatusNoContent)
	rd.getJSON(url, &p)
	if p.BooksReadCount != 2 || p.LastReadContinuousIndex != 2 {
		t.Fatalf("after both: %+v", p)
	}
}

// The tracker numbers a list's books as the browse endpoint does: an
// unreadable book is not a chapter, so lastBookRead=1 marks the ready book
// after it, not the unreadable one before it.
func TestKomgaProgressReadListSkipsUnreadable(t *testing.T) {
	f := newKomgaProgressFix(t)
	rd := f.as("reader@example.com", "reader pass")
	var l ReadList
	f.e.admin.json("POST", "/api/v1/read-lists", map[string]any{"name": "壊れた巻", "bookIds": []string{f.alphaBks[1].Id, f.alphaBks[0].Id}}, 201, &l)
	if _, err := f.e.db.Exec(`UPDATE books SET media_status = 'error' WHERE id = ?`, f.alphaBks[1].Id); err != nil {
		t.Fatal(err)
	}
	url := "/komga/api/v1/readlists/" + l.Id + "/read-progress/tachiyomi"

	var p komgaTachiProgressV1
	rd.getJSON(url, &p)
	if p.BooksCount != 1 || p.BooksReadCount != 0 || p.LastReadContinuousIndex != 0 {
		t.Fatalf("fresh list: %+v", p)
	}

	rd.want("PUT", url, `{"lastBookRead":1}`, http.StatusNoContent)
	rd.getJSON(url, &p)
	if p.BooksCount != 1 || p.BooksReadCount != 1 || p.LastReadContinuousIndex != 1 {
		t.Fatalf("after lastBookRead=1: %+v", p)
	}
	if n := f.readCount(f.readerID, f.alphaBks[0]); n != 1 {
		t.Fatal("the ready book was not marked read")
	}
	if n := f.readCount(f.readerID, f.alphaBks[1]); n != 0 {
		t.Fatal("the unreadable book was marked read")
	}
}

func TestKomgaProgressNoAccess(t *testing.T) {
	f := newKomgaProgressFix(t)
	rd := f.as("reader@example.com", "reader pass")
	var l ReadList
	f.e.admin.json("POST", "/api/v1/read-lists", map[string]any{"name": "Beta", "bookIds": []string{f.betaBks[0].Id}}, 201, &l)

	seriesURL := "/komga/api/v2/series/" + f.beta.Id + "/read-progress/tachiyomi"
	listURL := "/komga/api/v1/readlists/" + l.Id + "/read-progress/tachiyomi"
	rd.want("GET", seriesURL, "", http.StatusNotFound)
	rd.want("PUT", seriesURL, `{"lastBookNumberSortRead":1}`, http.StatusNotFound)
	rd.want("GET", listURL, "", http.StatusNotFound)
	rd.want("PUT", listURL, `{"lastBookRead":1}`, http.StatusNotFound)
	rd.want("PUT", "/komga/api/v1/readlists/nope/read-progress/tachiyomi", `{"lastBookRead":1}`, http.StatusNotFound)
	if n := f.readCount(f.readerID, f.betaBks...); n != 0 {
		t.Fatalf("hidden book written: %d", n)
	}

	// The other user sees the same list and series.
	other := f.as("other@example.com", "other pass")
	other.want("PUT", listURL, `{"lastBookRead":1}`, http.StatusNoContent)
	if !f.completedInAPI(f.other, f.betaBks[0]) {
		t.Fatal("other user's list write did not complete the book")
	}
}

func TestKomgaProgressBadBody(t *testing.T) {
	f := newKomgaProgressFix(t)
	rd := f.as("reader@example.com", "reader pass")
	var l ReadList
	f.e.admin.json("POST", "/api/v1/read-lists", map[string]any{"name": "順番", "bookIds": []string{f.alphaBks[0].Id}}, 201, &l)
	seriesURL := "/komga/api/v2/series/" + f.alpha.Id + "/read-progress/tachiyomi"
	listURL := "/komga/api/v1/readlists/" + l.Id + "/read-progress/tachiyomi"

	for _, body := range []string{``, `not json`, `{}`, `{"lastBookNumberSortRead":null}`, `{"lastBookNumberSortRead":"1"}`} {
		rd.want("PUT", seriesURL, body, http.StatusBadRequest)
	}
	for _, body := range []string{``, `{}`, `{"lastBookRead":"1"}`, `{"lastBookRead":1.5}`, `{"lastBookRead":-1}`} {
		rd.want("PUT", listURL, body, http.StatusBadRequest)
	}
	if n := f.readCount(f.readerID, f.alphaBks...); n != 0 {
		t.Fatalf("a malformed body wrote progress: %d", n)
	}
}

func TestKomgaProgressCookieOnly(t *testing.T) {
	f := newKomgaProgressFix(t)
	url := "/komga/api/v2/series/" + f.alpha.Id + "/read-progress/tachiyomi"

	// No credentials: 401 with a Basic challenge.
	rec := komgaProgressClient{t: t, h: f.compat}.do("GET", url, "")
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("anonymous: %d %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}

	// A Basic sign-in sets the compat cookie; the cookie alone is enough after.
	rec = f.as("reader@example.com", "reader pass").want("GET", url, "", http.StatusOK)
	var token string
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == compatCookie {
			token = ck.Value
		}
	}
	if token == "" {
		t.Fatal("no cn_compat cookie after a Basic sign-in")
	}
	cookieOnly := komgaProgressClient{t: t, h: f.compat, cookie: token}
	var p komgaTachiProgressV2
	cookieOnly.getJSON(url, &p)
	if p.BooksCount != 3 {
		t.Fatalf("cookie GET: %+v", p)
	}
	cookieOnly.want("PUT", url, `{"lastBookNumberSortRead":3}`, http.StatusNoContent)
	if !f.completedInAPI(f.reader, f.alphaBks[2]) {
		t.Fatal("cookie PUT did not complete book 3")
	}
}
