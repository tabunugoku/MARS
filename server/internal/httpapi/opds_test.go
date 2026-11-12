package httpapi

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/auth"
	"github.com/tabunugoku/cominavi/server/internal/library"
	"github.com/tabunugoku/cominavi/server/internal/media"
)

// opdsHandler is the /opds/ handler built the way cmd/cominavi builds it, with
// a library service on the test environment's database and folders, so pages,
// covers and files can be served.
func opdsHandler(e *env) http.Handler {
	lib := &library.Service{DB: e.db, Pages: &media.PageCache{Dir: filepath.Join(e.data, "pages"), MaxBytes: 1 << 30},
		ThumbDir: filepath.Join(e.data, "thumbs"), PageThumbDir: filepath.Join(e.data, "page-thumbs"), Events: e.hub}
	return NewCompat(Deps{DB: e.db, Version: "test", Library: lib, Auth: auth.Store{DB: e.db},
		Limiter: &auth.Limiter{}, Gate: e.gate, Events: e.hub})
}

// opdsGet sends a GET with HTTP Basic credentials (none when name is "").
func opdsGet(h http.Handler, path, name, pass string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	if name != "" {
		req.SetBasicAuth(name, pass)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// The feed as a client reads it. Prefixed names carry their namespace URI,
// so a prefix that was never declared fails to match.
type opdsFeedXML struct {
	ID      string         `xml:"id"`
	Title   string         `xml:"title"`
	Links   []opdsLinkXML  `xml:"link"`
	Entries []opdsEntryXML `xml:"entry"`
}

type opdsEntryXML struct {
	ID     string        `xml:"id"`
	Title  string        `xml:"title"`
	Issued string        `xml:"http://purl.org/dc/terms/ issued"`
	Links  []opdsLinkXML `xml:"link"`
}

type opdsLinkXML struct {
	Rel      string `xml:"rel,attr"`
	Href     string `xml:"href,attr"`
	Type     string `xml:"type,attr"`
	Length   int64  `xml:"length,attr"`
	Count    *int   `xml:"http://vaemendis.net/opds-pse/ns count,attr"`
	LastRead *int   `xml:"http://vaemendis.net/opds-pse/ns lastRead,attr"`
	LastDate string `xml:"http://vaemendis.net/opds-pse/ns lastReadDate,attr"`
}

func opdsParse(t *testing.T, rec *httptest.ResponseRecorder) opdsFeedXML {
	t.Helper()
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/atom+xml;profile=opds-catalog;kind=") {
		t.Fatalf("content type %q", ct)
	}
	var f opdsFeedXML
	if err := xml.Unmarshal(rec.Body.Bytes(), &f); err != nil {
		t.Fatalf("feed is not XML: %v\n%s", err, rec.Body)
	}
	for _, ns := range []string{`xmlns:opds="http://opds-spec.org/2010/catalog"`, `xmlns:dcterms="http://purl.org/dc/terms/"`,
		`xmlns:pse="http://vaemendis.net/opds-pse/ns"`, `xmlns:opensearch="http://a9.com/-/spec/opensearch/1.1/"`} {
		if !strings.Contains(rec.Body.String(), ns) {
			t.Fatalf("feed lacks %s", ns)
		}
	}
	return f
}

func linkRel(links []opdsLinkXML, rel string) (opdsLinkXML, bool) {
	for _, l := range links {
		if l.Rel == rel {
			return l, true
		}
	}
	return opdsLinkXML{}, false
}

func entryByID(f opdsFeedXML, id string) (opdsEntryXML, bool) {
	for _, e := range f.Entries {
		if e.ID == id {
			return e, true
		}
	}
	return opdsEntryXML{}, false
}

// opdsFixture starts an environment with OPDS on, a library with series
// Yofuke (books of 3 and 1 pages) and Hoshi (one book of 2 pages), and extra
// one-book series Batch 001... . A reader sees all of it; a limited user,
// who has no library, sees nothing. It returns the library's root folder.
func opdsFixture(t *testing.T, extra int) (e *env, root string, lib Library) {
	t.Helper()
	e = newEnv(t)
	root = t.TempDir()
	red, blue := pngBytes(t, 60, 90, color.RGBA{200, 0, 0, 255}), pngBytes(t, 60, 90, color.RGBA{0, 0, 200, 255})
	writeCBZ(t, filepath.Join(root, "Yofuke", "Yofuke v01.cbz"), map[string][]byte{"p1.png": red, "p2.png": blue, "p3.png": red})
	writeCBZ(t, filepath.Join(root, "Yofuke", "Yofuke v02.cbz"), map[string][]byte{"001.png": blue})
	writeCBZ(t, filepath.Join(root, "Hoshi", "Hoshi v01.cbz"), map[string][]byte{"001.png": red, "002.png": blue})
	for i := range extra {
		name := fmt.Sprintf("Batch %03d", i)
		writeCBZ(t, filepath.Join(root, name, name+" v01.cbz"), map[string][]byte{"001.png": red})
	}

	e.admin.json("PATCH", "/api/v1/settings", map[string]any{"opdsEnabled": true}, 200, nil)
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "読者", "email": "reader@example.com", "password": "reader pass",
		"role": "user", "allLibraries": true, "canStream": true, "canDownload": true}, 201, nil)
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "limited", "email": "limited@example.com", "password": "limited pass",
		"role": "user", "allLibraries": false, "libraryIds": []string{}}, 201, nil)
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "漫画", "root": root, "kind": "comics"}, 201, &lib)
	opdsWaitReady(t, e, lib.Id, 2+extra, 3+extra)
	return e, root, lib
}

// opdsWaitReady waits until the library holds the series and books, all with
// their pages analysed.
func opdsWaitReady(t *testing.T, e *env, libID string, series, books int) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		var ss []Series
		e.admin.json("GET", "/api/v1/series?libraryId="+libID, nil, 200, &ss)
		ready, all := 0, 0
		for _, s := range ss {
			var bs []Book
			e.admin.json("GET", "/api/v1/series/"+s.Id+"/books", nil, 200, &bs)
			for _, b := range bs {
				all++
				if b.MediaStatus == MediaStatusReady {
					ready++
				}
			}
		}
		if len(ss) == series && all == books && ready == books {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("library not ready: %d series, %d/%d books ready", len(ss), ready, books)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// findSeries returns the id of the visible series with the given title.
func findSeries(t *testing.T, e *env, libID, title string) string {
	t.Helper()
	var ss []Series
	e.admin.json("GET", "/api/v1/series?libraryId="+libID, nil, 200, &ss)
	for _, s := range ss {
		if s.Title == title {
			return s.Id
		}
	}
	t.Fatalf("no series %q", title)
	return ""
}

const (
	opdsReaderName = "reader@example.com"
	opdsReaderPass = "reader pass"
	opdsLimitName  = "limited@example.com"
	opdsLimitPass  = "limited pass"
)

// TestOpdsOffAnswers404: while opdsEnabled is off, nothing under /opds/ answers.
func TestOpdsOffAnswers404(t *testing.T) {
	e := newEnv(t)
	h := opdsHandler(e)
	for _, p := range []string{"/opds/v1.2/catalog", "/opds/", "/opds/v1.2/search.xml"} {
		if rec := opdsGet(h, p, opdsReaderName, opdsReaderPass); rec.Code != 404 {
			t.Fatalf("%s while off: %d", p, rec.Code)
		}
	}
}

// TestOpdsCatalogAndRedirect: the root redirects to the catalog, which lists
// the six sections and the search link.
func TestOpdsCatalogAndRedirect(t *testing.T) {
	e, _, _ := opdsFixture(t, 0)
	h := opdsHandler(e)

	if rec := opdsGet(h, "/opds/", "", ""); rec.Code != 401 {
		t.Fatalf("anonymous root: %d", rec.Code)
	}
	rec := opdsGet(h, "/opds/", opdsReaderName, opdsReaderPass)
	if rec.Code != 302 || rec.Header().Get("Location") != "/opds/v1.2/catalog" {
		t.Fatalf("root: %d %q", rec.Code, rec.Header().Get("Location"))
	}

	f := opdsParse(t, opdsGet(h, "/opds/v1.2/catalog", opdsReaderName, opdsReaderPass))
	if got := len(f.Entries); got != 6 {
		t.Fatalf("catalog has %d entries, want 6", got)
	}
	for _, id := range []string{"keep-reading", "series-latest", "books-latest", "libraries", "collections", "readlists"} {
		if _, ok := entryByID(f, "urn:cominavi:feed:"+id); !ok {
			t.Fatalf("catalog lacks %s", id)
		}
	}
	if _, ok := linkRel(f.Links, "search"); !ok {
		t.Fatal("catalog has no search link")
	}
	if l, ok := linkRel(f.Links, "self"); !ok || l.Href != "/opds/v1.2/catalog" {
		t.Fatalf("self link %+v", l)
	}
	if l, ok := linkRel(f.Links, "start"); !ok || l.Href != "/opds/v1.2/catalog" {
		t.Fatalf("start link %+v", l)
	}
}

// TestOpdsLibraryAndSeries: libraries, a library's series, a series' books
// with their acquisition, thumbnail and page-stream links.
func TestOpdsLibraryAndSeries(t *testing.T) {
	e, _, lib := opdsFixture(t, 0)
	h := opdsHandler(e)

	libs := opdsParse(t, opdsGet(h, "/opds/v1.2/libraries", opdsReaderName, opdsReaderPass))
	if _, ok := entryByID(libs, "urn:cominavi:library:"+lib.Id); !ok {
		t.Fatalf("libraries %+v", libs.Entries)
	}

	lf := opdsParse(t, opdsGet(h, "/opds/v1.2/libraries/"+lib.Id, opdsReaderName, opdsReaderPass))
	if len(lf.Entries) != 2 {
		t.Fatalf("library feed has %d series", len(lf.Entries))
	}
	yofuke := findSeries(t, e, lib.Id, "Yofuke")
	if _, ok := entryByID(lf, "urn:cominavi:series:"+yofuke); !ok {
		t.Fatalf("library feed lacks Yofuke: %+v", lf.Entries)
	}

	sf := opdsParse(t, opdsGet(h, "/opds/v1.2/series/"+yofuke, opdsReaderName, opdsReaderPass))
	if len(sf.Entries) != 2 {
		t.Fatalf("series feed has %d books", len(sf.Entries))
	}
	for _, b := range sf.Entries {
		acq, ok := linkRel(b.Links, "http://opds-spec.org/acquisition")
		if !ok || acq.Type != "application/zip" || acq.Length <= 0 || !strings.HasSuffix(acq.Href, "/file") {
			t.Fatalf("acquisition link %+v", acq)
		}
		if _, ok := linkRel(b.Links, "http://opds-spec.org/image/thumbnail"); !ok {
			t.Fatal("book has no thumbnail link")
		}
		pse, ok := linkRel(b.Links, "http://vaemendis.net/opds-pse/stream")
		if !ok || pse.Count == nil || pse.Type != "image/jpeg" {
			t.Fatalf("stream link %+v", pse)
		}
		if !strings.Contains(pse.Href, "/pages/{pageNumber}?maxWidth={maxWidth}") {
			t.Fatalf("stream template %q", pse.Href)
		}
		if pse.LastRead != nil {
			t.Fatalf("unread book has lastRead %d", *pse.LastRead)
		}
	}

	// The page stream of the three-page book is there.
	var sawThree bool
	for _, b := range sf.Entries {
		if pse, _ := linkRel(b.Links, "http://vaemendis.net/opds-pse/stream"); pse.Count != nil && *pse.Count == 3 {
			sawThree = true
		}
	}
	if !sawThree {
		t.Fatalf("no book with pse:count 3: %+v", sf.Entries)
	}
	if rec := opdsGet(h, "/opds/v1.2/series/"+yofuke+"/thumbnail", opdsReaderName, opdsReaderPass); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("series thumbnail: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec := opdsGet(h, "/opds/v1.2/series/nope", opdsReaderName, opdsReaderPass); rec.Code != 404 {
		t.Fatalf("unknown series: %d", rec.Code)
	}
}

// TestOpdsPSEAndDownload: the stream's page 0 is the first page, as the main
// API serves it; the file link returns the book's file.
func TestOpdsPSEAndDownload(t *testing.T) {
	e, root, lib := opdsFixture(t, 0)
	h := opdsHandler(e)
	yofuke := findSeries(t, e, lib.Id, "Yofuke")
	var bs []Book
	e.admin.json("GET", "/api/v1/series/"+yofuke+"/books", nil, 200, &bs)
	var three Book
	for _, b := range bs {
		if b.PageCount == 3 {
			three = b
		}
	}
	if three.Id == "" {
		t.Fatalf("no three-page book in %+v", bs)
	}

	var pages []Page
	e.admin.json("GET", "/api/v1/books/"+three.Id+"/pages", nil, 200, &pages)
	first := e.admin.do("GET", "/api/v1/books/"+three.Id+"/pages/1", nil)
	if first.Code != 200 {
		t.Fatalf("main API page 1: %d", first.Code)
	}

	base := "/opds/v1.2/books/" + three.Id
	rec := opdsGet(h, base+"/pages/0?maxWidth=600", opdsReaderName, opdsReaderPass)
	if rec.Code != 200 || !bytes.Equal(rec.Body.Bytes(), first.Body.Bytes()) {
		t.Fatalf("PSE page 0: %d, %d bytes (want %d)", rec.Code, rec.Body.Len(), first.Body.Len())
	}
	if ct := rec.Header().Get("Content-Type"); ct != pages[0].MediaType {
		t.Fatalf("PSE content type %q, want %q", ct, pages[0].MediaType)
	}
	if rec := opdsGet(h, base+"/pages/3", opdsReaderName, opdsReaderPass); rec.Code != 404 {
		t.Fatalf("page past the end: %d", rec.Code)
	}
	if rec := opdsGet(h, base+"/pages/x", opdsReaderName, opdsReaderPass); rec.Code != 404 {
		t.Fatalf("bad page number: %d", rec.Code)
	}

	want, err := os.ReadFile(filepath.Join(root, "Yofuke", "Yofuke v01.cbz"))
	if err != nil {
		t.Fatal(err)
	}
	dl := opdsGet(h, base+"/file", opdsReaderName, opdsReaderPass)
	if dl.Code != 200 || !bytes.Equal(dl.Body.Bytes(), want) {
		t.Fatalf("download: %d, %d bytes (want %d)", dl.Code, dl.Body.Len(), len(want))
	}
	if cd := dl.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("content disposition %q", cd)
	}
}

// TestOpdsPageProgress: a book in progress is in keep reading with its
// lastRead (0-based) and lastReadDate; once completed it leaves the feed.
func TestOpdsPageProgress(t *testing.T) {
	e, _, lib := opdsFixture(t, 0)
	h := opdsHandler(e)
	yofuke := findSeries(t, e, lib.Id, "Yofuke")
	var bs []Book
	e.admin.json("GET", "/api/v1/series/"+yofuke+"/books", nil, 200, &bs)
	var three Book
	for _, b := range bs {
		if b.PageCount == 3 {
			three = b
		}
	}
	reader := e.login(opdsReaderName, opdsReaderPass)
	reader.json("PUT", "/api/v1/books/"+three.Id+"/progress", map[string]any{"page": 2}, 200, nil)

	kr := opdsParse(t, opdsGet(h, "/opds/v1.2/keep-reading", opdsReaderName, opdsReaderPass))
	ent, ok := entryByID(kr, "urn:cominavi:book:"+three.Id)
	if !ok {
		t.Fatalf("keep reading lacks the book: %+v", kr.Entries)
	}
	pse, _ := linkRel(ent.Links, "http://vaemendis.net/opds-pse/stream")
	if pse.LastRead == nil || *pse.LastRead != 1 || pse.LastDate == "" {
		t.Fatalf("lastRead %+v", pse)
	}

	reader.json("PUT", "/api/v1/books/"+three.Id+"/progress", map[string]any{"page": 3}, 200, nil)
	kr = opdsParse(t, opdsGet(h, "/opds/v1.2/keep-reading", opdsReaderName, opdsReaderPass))
	if _, ok := entryByID(kr, "urn:cominavi:book:"+three.Id); ok {
		t.Fatal("completed book still in keep reading")
	}
}

// TestOpdsSearch: a title search finds the series; the description is served.
func TestOpdsSearch(t *testing.T) {
	e, _, _ := opdsFixture(t, 0)
	h := opdsHandler(e)

	f := opdsParse(t, opdsGet(h, "/opds/v1.2/search?q=Hoshi", opdsReaderName, opdsReaderPass))
	if len(f.Entries) != 1 || f.Entries[0].Title != "Hoshi" {
		t.Fatalf("search Hoshi: %+v", f.Entries)
	}
	f = opdsParse(t, opdsGet(h, "/opds/v1.2/search?q=yof", opdsReaderName, opdsReaderPass))
	if len(f.Entries) != 1 || f.Entries[0].Title != "Yofuke" {
		t.Fatalf("search yof: %+v", f.Entries)
	}
	f = opdsParse(t, opdsGet(h, "/opds/v1.2/search?q=nothing-like-this", opdsReaderName, opdsReaderPass))
	if len(f.Entries) != 0 {
		t.Fatalf("search miss: %+v", f.Entries)
	}

	rec := opdsGet(h, "/opds/v1.2/search.xml", opdsReaderName, opdsReaderPass)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/opensearchdescription+xml" ||
		!strings.Contains(rec.Body.String(), `template="/opds/v1.2/search?q={searchTerms}"`) {
		t.Fatalf("search description: %d %q", rec.Code, rec.Body)
	}
}

// TestOpdsAccess: a user without the library gets 404 for its series, its
// books' pages and files, and sees no series in search.
func TestOpdsAccess(t *testing.T) {
	e, _, lib := opdsFixture(t, 0)
	h := opdsHandler(e)
	yofuke := findSeries(t, e, lib.Id, "Yofuke")
	var bs []Book
	e.admin.json("GET", "/api/v1/series/"+yofuke+"/books", nil, 200, &bs)
	book := bs[0].Id

	for _, p := range []string{
		"/opds/v1.2/series/" + yofuke,
		"/opds/v1.2/series/" + yofuke + "/thumbnail",
		"/opds/v1.2/books/" + book + "/pages/0",
		"/opds/v1.2/books/" + book + "/file",
		"/opds/v1.2/books/" + book + "/thumbnail",
		"/opds/v1.2/libraries/" + lib.Id,
	} {
		if rec := opdsGet(h, p, opdsLimitName, opdsLimitPass); rec.Code != 404 {
			t.Fatalf("%s for a limited user: %d", p, rec.Code)
		}
	}
	f := opdsParse(t, opdsGet(h, "/opds/v1.2/search?q=Yofuke", opdsLimitName, opdsLimitPass))
	if len(f.Entries) != 0 {
		t.Fatalf("limited user finds %+v", f.Entries)
	}
	if rec := opdsGet(h, "/opds/v1.2/series/latest", opdsLimitName, opdsLimitPass); rec.Code != 200 || strings.Contains(rec.Body.String(), "Yofuke") {
		t.Fatalf("latest series for a limited user: %d", rec.Code)
	}
}

// TestOpdsCollection: a collection lists its visible series; a collection
// the caller cannot see is 404.
func TestOpdsCollection(t *testing.T) {
	e, _, lib := opdsFixture(t, 0)
	h := opdsHandler(e)
	hoshi := findSeries(t, e, lib.Id, "Hoshi")
	var c Collection
	e.admin.json("POST", "/api/v1/collections", map[string]any{"name": "星", "seriesIds": []string{hoshi}}, 201, &c)

	f := opdsParse(t, opdsGet(h, "/opds/v1.2/collections/"+c.Id, opdsReaderName, opdsReaderPass))
	if len(f.Entries) != 1 || f.Entries[0].ID != "urn:cominavi:series:"+hoshi {
		t.Fatalf("collection feed %+v", f.Entries)
	}
	if rec := opdsGet(h, "/opds/v1.2/collections/nope", opdsReaderName, opdsReaderPass); rec.Code != 404 {
		t.Fatalf("unknown collection: %d", rec.Code)
	}
	// The limited user has no visible member, so the collection is not there.
	if rec := opdsGet(h, "/opds/v1.2/collections/"+c.Id, opdsLimitName, opdsLimitPass); rec.Code != 404 {
		t.Fatalf("collection for a limited user: %d", rec.Code)
	}
}

// TestOpdsPaging: the newest series page by 50, with next and previous links.
func TestOpdsPaging(t *testing.T) {
	e, _, _ := opdsFixture(t, 49)
	h := opdsHandler(e)

	first := opdsParse(t, opdsGet(h, "/opds/v1.2/series/latest", opdsReaderName, opdsReaderPass))
	if len(first.Entries) != opdsPageSize {
		t.Fatalf("page 0 has %d entries", len(first.Entries))
	}
	if _, ok := linkRel(first.Links, "next"); !ok {
		t.Fatal("page 0 has no next link")
	}
	if _, ok := linkRel(first.Links, "previous"); ok {
		t.Fatal("page 0 has a previous link")
	}

	second := opdsParse(t, opdsGet(h, "/opds/v1.2/series/latest?page=1", opdsReaderName, opdsReaderPass))
	if len(second.Entries) != 1 {
		t.Fatalf("page 1 has %d entries", len(second.Entries))
	}
	if _, ok := linkRel(second.Links, "next"); ok {
		t.Fatal("last page has a next link")
	}
	if l, ok := linkRel(second.Links, "previous"); !ok || l.Href != "/opds/v1.2/series/latest" {
		t.Fatalf("previous link %+v", l)
	}
	seen := map[string]bool{}
	for _, en := range append(first.Entries, second.Entries...) {
		if seen[en.ID] {
			t.Fatalf("series %s on two pages", en.ID)
		}
		seen[en.ID] = true
	}
	if len(seen) != 51 {
		t.Fatalf("%d distinct series over both pages, want 51", len(seen))
	}
}
