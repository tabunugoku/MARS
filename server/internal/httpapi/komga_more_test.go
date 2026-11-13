package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// The Komga endpoints added after the browse API: book and series progress
// writes, users/me, authors v2, file, next and previous, collection one and
// its thumbnail, and on deck, latest and the new and updated series lists.
//
// Every one is checked for the happy path and for visibility: the reader is
// restricted to library A, so Beta's book, series and collection answer 404
// and stay out of the lists.

// komgaNoDL may sign in but may not download files.
var komgaNoDL = komgaUser{"komga-nodl@example.com", "nodl pass"}

// komgaCall sends any method as u (komgaNobody for no credentials).
func komgaCall(f *komgaFix, u komgaUser, method, target, body string) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if u != komgaNobody {
		req.SetBasicAuth(u.mail, u.pass)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return rec
}

// wantCode sends the request and fails unless the status is code.
func wantCode(f *komgaFix, u komgaUser, method, target, body string, code int) *httptest.ResponseRecorder {
	f.t.Helper()
	rec := komgaCall(f, u, method, target, body)
	if rec.Code != code {
		f.t.Fatalf("%s %s %s: status %d, want %d: %s", method, target, body, rec.Code, code, rec.Body)
	}
	return rec
}

// coreBook reads a book through the main API as u.
func (f *komgaFix) coreBook(u komgaUser, id string) Book {
	f.t.Helper()
	var b Book
	f.e.login(u.mail, u.pass).json("GET", "/api/v1/books/"+id, nil, 200, &b)
	return b
}

// coreDashboard reads the main API's dashboard as u.
func (f *komgaFix) coreDashboard(u komgaUser) Dashboard {
	f.t.Helper()
	var d Dashboard
	f.e.login(u.mail, u.pass).json("GET", "/api/v1/dashboard", nil, 200, &d)
	return d
}

// createNoDownload adds komgaNoDL: a library A user without download rights.
func (f *komgaFix) createNoDownload() {
	f.t.Helper()
	var u User
	f.e.admin.json("POST", "/api/v1/users", map[string]any{"name": "NoDL", "email": komgaNoDL.mail, "password": komgaNoDL.pass,
		"role": "user", "canDownload": false, "allLibraries": false, "libraryIds": []string{f.libA.Id}}, 201, &u)
}

// sortedSet formats ids sorted, so two lists compare as sets.
func sortedSet(ids []string) string {
	c := append([]string(nil), ids...)
	sort.Strings(c)
	return fmt.Sprint(c)
}

// newestFirst fails unless the content's date field (created or lastModified,
// RFC 3339 text) never rises from one item to the next.
func (f *komgaFix) newestFirst(l komgaListing, field string) {
	f.t.Helper()
	for i := 1; i < len(l.Content); i++ {
		prev, cur := fmt.Sprint(l.Content[i-1][field]), fmt.Sprint(l.Content[i][field])
		if prev < cur {
			f.t.Errorf("%s not newest first at %d: %s before %s", field, i, prev, cur)
		}
	}
}

// alphaInOrder checks the fixture's Alpha books are four and in number order,
// which the sibling and on deck checks rely on.
func (f *komgaFix) alphaInOrder() {
	f.t.Helper()
	if len(f.alphaBooks) != 4 {
		f.t.Fatalf("Alpha has %d books, want 4", len(f.alphaBooks))
	}
	for i := 1; i < len(f.alphaBooks); i++ {
		if f.alphaBooks[i-1].NumberSort >= f.alphaBooks[i].NumberSort {
			f.t.Fatalf("Alpha books not in number order: %v", bookIDs(f.alphaBooks))
		}
	}
}

// ─── Account ─────────────────────────────────────────────────────────────────

func TestKomgaUsersMe(t *testing.T) {
	f := newKomgaFix(t)
	f.createNoDownload()

	for _, target := range []string{"/komga/api/v1/users/me", "/komga/api/v2/users/me"} {
		var admin, reader, nodl map[string]any
		f.get(komgaAdmin, target, 200, &admin)
		f.get(komgaReader, target, 200, &reader)
		f.get(komgaNoDL, target, 200, &nodl)

		mustEqual(t, target+" admin email", admin["email"], komgaAdmin.mail)
		mustEqual(t, target+" admin roles", fmt.Sprint(admin["roles"]), "[ADMIN USER FILE_DOWNLOAD PAGE_STREAMING]")
		mustEqual(t, target+" admin sharedAll", admin["sharedAllLibraries"], true)

		mustEqual(t, target+" reader email", reader["email"], komgaReader.mail)
		roles := fmt.Sprint(reader["roles"])
		if !strings.Contains(roles, "USER") || strings.Contains(roles, "ADMIN") || !strings.Contains(roles, "FILE_DOWNLOAD") {
			t.Errorf("%s reader roles %s", target, roles)
		}
		mustEqual(t, target+" reader sharedAll", reader["sharedAllLibraries"], false)
		mustEqual(t, target+" reader libraries", fmt.Sprint(reader["sharedLibrariesIds"]), fmt.Sprint([]string{f.libA.Id}))
		mustEqual(t, target+" reader ageRestriction", reader["ageRestriction"], nil)
		mustEqual(t, target+" reader labels", fmt.Sprint(reader["labelsAllow"], reader["labelsExclude"]), "[] []")

		if strings.Contains(fmt.Sprint(nodl["roles"]), "FILE_DOWNLOAD") {
			t.Errorf("%s: a user without download rights has %v", target, nodl["roles"])
		}
	}
	// Signed out: the usual challenge.
	if rec := komgaCall(f, komgaNobody, "GET", "/komga/api/v2/users/me", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous users/me: %d", rec.Code)
	}
}

// ─── Authors v2 ──────────────────────────────────────────────────────────────

func TestKomgaAuthorsV2(t *testing.T) {
	f := newKomgaFix(t)
	// Alpha and Beta carry the authors (the fixture's authors come from the
	// files; these make the filters testable).
	f.setAuthors(f.alpha.Id, `[{"name":"Alice","role":"writer"},{"name":"Bob","role":"penciller"}]`)
	f.setAuthors(f.beta.Id, `[{"name":"Carol","role":"writer"}]`)

	names := func(u komgaUser, query string) []string {
		return komgaNames(f.listing(u, "/komga/api/v2/authors?unpaged=true"+query))
	}
	mustEqual(t, "admin authors", fmt.Sprint(names(komgaAdmin, "")), "[Alice Bob Carol]")
	mustEqual(t, "reader authors", fmt.Sprint(names(komgaReader, "")), "[Alice Bob]")
	mustEqual(t, "search", fmt.Sprint(names(komgaReader, "&search=ali")), "[Alice]")
	mustEqual(t, "role, any case", fmt.Sprint(names(komgaAdmin, "&role=PENCILLER")), "[Bob]")
	mustEqual(t, "series", fmt.Sprint(names(komgaAdmin, "&series_id="+f.beta.Id)), "[Carol]")
	mustEqual(t, "series, reader", fmt.Sprint(names(komgaReader, "&series_id="+f.beta.Id)), "[]")
	mustEqual(t, "library", fmt.Sprint(names(komgaAdmin, "&library_id="+f.libB.Id)), "[Carol]")
	mustEqual(t, "library, reader", fmt.Sprint(names(komgaReader, "&library_id="+f.libB.Id)), "[]")

	// The collection holds Alpha: the reader sees its authors.
	mustEqual(t, "collection", fmt.Sprint(names(komgaReader, "&collection_id="+f.alphaSet.Id)), "[Alice Bob]")
	mustEqual(t, "beta collection, reader", fmt.Sprint(names(komgaReader, "&collection_id="+f.betaSet.Id)), "[]")

	// Paged: size 1, the second author.
	l := f.listing(komgaAdmin, "/komga/api/v2/authors?size=1&page=1")
	mustEqual(t, "paged names", fmt.Sprint(komgaNames(l)), "[Bob]")
	mustEqual(t, "paged total", l.TotalElements, 3)
	mustEqual(t, "paged number", l.Number, 1)
}

// setAuthors writes a series' authors column (the JSON the import keeps).
func (f *komgaFix) setAuthors(seriesID, authors string) {
	f.t.Helper()
	if _, err := f.e.db.Exec(`UPDATE series SET authors = ? WHERE id = ?`, authors, seriesID); err != nil {
		f.t.Fatal(err)
	}
}

// ─── Book progress ───────────────────────────────────────────────────────────

func TestKomgaBookProgressWrites(t *testing.T) {
	f := newKomgaFix(t)
	f.alphaInOrder()
	pages := f.alphaBooks[0].PageCount
	if pages < 2 {
		t.Fatalf("Alpha v01 has %d pages, want at least 2", pages)
	}
	first, second := f.alphaBooks[0], f.alphaBooks[1]
	base := "/komga/api/v1/books/"

	// Out of range is 400, the book is unchanged.
	wantCode(f, komgaReader, "PATCH", base+first.Id+"/read-progress", `{"page":0}`, 400)
	wantCode(f, komgaReader, "PATCH", base+first.Id+"/read-progress", fmt.Sprintf(`{"page":%d}`, pages+1), 400)
	// Malformed and empty bodies are 400.
	wantCode(f, komgaReader, "PATCH", base+first.Id+"/read-progress", `not json`, 400)
	wantCode(f, komgaReader, "PATCH", base+first.Id+"/read-progress", `{}`, 400)

	// A page inside the book sets the progress, unfinished before the last.
	wantCode(f, komgaReader, "PATCH", base+first.Id+"/read-progress", `{"page":1}`, 204)
	if p := f.coreBook(komgaReader, first.Id).Progress; p == nil || p.Page != 1 || p.Completed {
		t.Errorf("after page 1: progress %+v", p)
	}
	// completed=true marks the book read.
	wantCode(f, komgaReader, "PATCH", base+second.Id+"/read-progress", `{"completed":true}`, 204)
	if p := f.coreBook(komgaReader, second.Id).Progress; p == nil || !p.Completed {
		t.Errorf("after completed: progress %+v", p)
	}
	// DELETE marks it unread again.
	wantCode(f, komgaReader, "DELETE", base+second.Id+"/read-progress", "", 204)
	if p := f.coreBook(komgaReader, second.Id).Progress; p != nil {
		t.Errorf("after delete: progress %+v, want none", p)
	}

	// Beta's book is another library's: 404 for the reader, both ways.
	wantCode(f, komgaReader, "PATCH", base+f.beta1.Id+"/read-progress", `{"completed":true}`, 404)
	wantCode(f, komgaReader, "DELETE", base+f.beta1.Id+"/read-progress", "", 404)
	wantCode(f, komgaAdmin, "PATCH", base+f.beta1.Id+"/read-progress", `{"completed":true}`, 204)
}

func TestKomgaSeriesProgressWrites(t *testing.T) {
	f := newKomgaFix(t)
	f.alphaInOrder()
	target := "/komga/api/v1/series/"

	// Every live book of Alpha, the unreadable one included, goes read.
	wantCode(f, komgaReader, "POST", target+f.alpha.Id+"/read-progress", "", 204)
	for _, b := range f.alphaBooks {
		if p := f.coreBook(komgaReader, b.Id).Progress; p == nil || !p.Completed {
			t.Errorf("book %s after series read: progress %+v", b.Id, p)
		}
	}
	wantCode(f, komgaReader, "DELETE", target+f.alpha.Id+"/read-progress", "", 204)
	for _, b := range f.alphaBooks {
		if p := f.coreBook(komgaReader, b.Id).Progress; p != nil && p.Completed {
			t.Errorf("book %s after series unread: progress %+v", b.Id, p)
		}
	}

	// Beta is another library's series.
	wantCode(f, komgaReader, "POST", target+f.beta.Id+"/read-progress", "", 404)
	wantCode(f, komgaReader, "DELETE", target+f.beta.Id+"/read-progress", "", 404)
	var rows int
	if err := f.e.db.QueryRow(`SELECT count(*) FROM read_progress WHERE user_id = ? AND book_id = ?`,
		f.reader.Id, f.beta1.Id).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	mustEqual(t, "Beta progress rows of the reader", rows, 0)
}

// ─── File ────────────────────────────────────────────────────────────────────

func TestKomgaBookFile(t *testing.T) {
	f := newKomgaFix(t)
	f.createNoDownload()
	file := "/komga/api/v1/books/" + f.alphaBooks[0].Id + "/file"

	rec := wantCode(f, komgaReader, "GET", file, "", 200)
	if rec.Body.Len() == 0 {
		t.Errorf("file: empty body")
	}
	// Without the download permission: 403, the file is not sent.
	wantCode(f, komgaNoDL, "GET", file, "", 403)
	// Beta's file is not the reader's; the admin may take it.
	wantCode(f, komgaReader, "GET", "/komga/api/v1/books/"+f.beta1.Id+"/file", "", 404)
	wantCode(f, komgaAdmin, "GET", "/komga/api/v1/books/"+f.beta1.Id+"/file", "", 200)
}

// ─── Next and previous ───────────────────────────────────────────────────────

func TestKomgaSiblingBooks(t *testing.T) {
	f := newKomgaFix(t)
	f.alphaInOrder()
	b := f.alphaBooks
	base := "/komga/api/v1/books/"

	sibling := func(u komgaUser, id, which string, code int) string {
		t.Helper()
		rec := wantCode(f, u, "GET", base+id+"/"+which, "", code)
		if code != 200 {
			return ""
		}
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return fmt.Sprint(got["id"])
	}
	mustEqual(t, "next of first", sibling(komgaReader, b[0].Id, "next", 200), b[1].Id)
	mustEqual(t, "next of middle", sibling(komgaReader, b[1].Id, "next", 200), b[2].Id)
	mustEqual(t, "previous of last", sibling(komgaReader, b[3].Id, "previous", 200), b[2].Id)
	// The ends of the series: 404.
	sibling(komgaReader, b[0].Id, "previous", 404)
	sibling(komgaReader, b[3].Id, "next", 404)
	// Beta has one book, and it is not the reader's.
	sibling(komgaAdmin, f.beta1.Id, "next", 404)
	sibling(komgaAdmin, f.beta1.Id, "previous", 404)
	sibling(komgaReader, f.beta1.Id, "next", 404)
}

// ─── On deck, latest, new and updated ────────────────────────────────────────

func TestKomgaOnDeckAndLatest(t *testing.T) {
	f := newKomgaFix(t)
	f.alphaInOrder()
	d := f.coreDashboard(komgaReader)

	// The on deck list is the main API's (the fixture finished Alpha v01).
	ondeck := f.listing(komgaReader, "/komga/api/v1/books/ondeck?unpaged=true")
	mustEqual(t, "ondeck follows the main API", sortedSet(komgaIDs(ondeck)), sortedSet(bookIDs(d.OnDeck)))
	mustEqual(t, "ondeck is the next Alpha book", fmt.Sprint(komgaIDs(ondeck)), fmt.Sprint([]string{f.alphaBooks[1].Id}))
	// Another library's on deck is left out for the reader.
	mustEqual(t, "ondeck library B, reader", len(f.listing(komgaReader, "/komga/api/v1/books/ondeck?unpaged=true&library_id="+f.libB.Id).Content), 0)

	// Latest: the reader's books, newest first, matching the main API's list
	// of recently added books.
	latest := f.listing(komgaReader, "/komga/api/v1/books/latest?unpaged=true")
	mustEqual(t, "latest follows the main API", sortedSet(komgaIDs(latest)), sortedSet(bookIDs(d.RecentlyAddedBooks)))
	f.newestFirst(latest, "created")
	for _, id := range komgaIDs(latest) {
		if id == f.beta1.Id {
			t.Errorf("reader sees Beta's book in latest")
		}
	}
	mustEqual(t, "latest library B, reader", len(f.listing(komgaReader, "/komga/api/v1/books/latest?unpaged=true&library_id="+f.libB.Id).Content), 0)
	mustEqual(t, "latest library B, admin", len(f.listing(komgaAdmin, "/komga/api/v1/books/latest?unpaged=true&library_id="+f.libB.Id).Content), 1)
	// Paged: the first page holds the first two.
	paged := f.listing(komgaReader, "/komga/api/v1/books/latest?size=2")
	mustEqual(t, "latest page size", len(paged.Content), 2)
	mustEqual(t, "latest page total", paged.TotalElements, len(latest.Content))
}

func TestKomgaSeriesNewAndUpdated(t *testing.T) {
	f := newKomgaFix(t)
	d := f.coreDashboard(komgaReader)

	added := f.listing(komgaReader, "/komga/api/v1/series/new?unpaged=true")
	mustEqual(t, "new follows the main API", sortedSet(komgaIDs(added)), sortedSet(seriesIDs(d.RecentlyAddedSeries)))
	f.newestFirst(added, "created")
	for _, id := range komgaIDs(added) {
		if id == f.beta.Id {
			t.Errorf("reader sees Beta in series/new")
		}
	}
	mustEqual(t, "new library B, admin", fmt.Sprint(komgaIDs(f.listing(komgaAdmin, "/komga/api/v1/series/new?unpaged=true&library_id="+f.libB.Id))), fmt.Sprint([]string{f.beta.Id}))

	// Updated: the reader's series only, last modified first.
	updated := f.listing(komgaReader, "/komga/api/v1/series/updated?unpaged=true")
	mustEqual(t, "updated series", sortedSet(komgaIDs(updated)), sortedSet([]string{f.alpha.Id, f.gamma.Id}))
	f.newestFirst(updated, "lastModified")
	mustEqual(t, "updated library B, reader", len(f.listing(komgaReader, "/komga/api/v1/series/updated?unpaged=true&library_id="+f.libB.Id).Content), 0)
}

// ─── Collections ─────────────────────────────────────────────────────────────

func TestKomgaCollectionOneAndThumbnail(t *testing.T) {
	f := newKomgaFix(t)
	base := "/komga/api/v1/collections/"

	var one map[string]any
	f.get(komgaReader, base+f.alphaSet.Id, 200, &one)
	mustEqual(t, "collection name", one["name"], "Alpha set")
	mustEqual(t, "collection seriesIds", fmt.Sprint(one["seriesIds"]), fmt.Sprint([]string{f.alpha.Id}))

	// Another library's collection: 404 for the reader, one and thumbnail.
	wantCode(f, komgaReader, "GET", base+f.betaSet.Id, "", 404)
	wantCode(f, komgaReader, "GET", base+f.betaSet.Id+"/thumbnail", "", 404)
	wantCode(f, komgaAdmin, "GET", base+f.betaSet.Id, "", 200)

	// The thumbnail is the first visible series' cover, a JPEG.
	rec := wantCode(f, komgaReader, "GET", base+f.alphaSet.Id+"/thumbnail", "", 200)
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("collection thumbnail type %q", ct)
	}
	wantCode(f, komgaAdmin, "GET", base+f.betaSet.Id+"/thumbnail", "", 200)
}
