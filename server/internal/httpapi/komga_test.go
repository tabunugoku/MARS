package httpapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tabunugoku/mars/server/internal/auth"
	"github.com/tabunugoku/mars/server/internal/library"
	"github.com/tabunugoku/mars/server/internal/media"
)

// The Komga-shaped API, tested through NewCompat with HTTP Basic sign-in.
//
// The field lists below are the contract the Mihon/Tachiyomi clients parse
// (docs/opds-komga.md, "Komga-shaped API"): a field the client declares
// without a default must be present, with the JSON type given here, even
// when it is null or empty. Extra fields are allowed.

// Shape kinds. A spec is a kind, a map of field specs (an object, every
// listed field present), or []any{spec} (an array whose elements match spec).
const (
	kString     = "string"
	kNullString = "string or null"
	kNumber     = "number"
	kNullNumber = "number or null"
	kBool       = "bool"
	kArray      = "array"
	kNullObject = "object or null"
)

var komgaSortSpec = map[string]any{"empty": kBool, "sorted": kBool, "unsorted": kBool}

// komgaPageSpec is the page wrapper around content items of spec content.
func komgaPageSpec(content any) map[string]any {
	return map[string]any{
		"content": []any{content},
		"pageable": map[string]any{
			"sort": komgaSortSpec, "offset": kNumber, "pageNumber": kNumber, "pageSize": kNumber,
			"paged": kBool, "unpaged": kBool,
		},
		"totalElements": kNumber, "totalPages": kNumber, "last": kBool, "first": kBool,
		"number": kNumber, "size": kNumber, "numberOfElements": kNumber, "empty": kBool, "sort": komgaSortSpec,
	}
}

var komgaAuthorSpec = map[string]any{"name": kString, "role": kString}

var komgaSeriesSpec = map[string]any{
	"id": kString, "libraryId": kString, "name": kString,
	"created": kNullString, "lastModified": kNullString, "fileLastModified": kString,
	"booksCount": kNumber, "booksReadCount": kNumber, "booksUnreadCount": kNumber, "booksInProgressCount": kNumber,
	"metadata": map[string]any{
		"status": kString, "created": kNullString, "lastModified": kNullString,
		"title": kString, "titleSort": kString, "summary": kString, "summaryLock": kBool,
		"readingDirection": kString, "readingDirectionLock": kBool,
		"publisher": kString, "publisherLock": kBool,
		"ageRating": kNullNumber, "ageRatingLock": kBool,
		"language": kString, "languageLock": kBool,
		"genres": kArray, "genresLock": kBool, "tags": kArray, "tagsLock": kBool,
		"totalBookCount": kNullNumber,
	},
	"booksMetadata": map[string]any{
		"authors": kArray, "tags": kArray, "releaseDate": kNullString,
		"summary": kString, "summaryNumber": kString, "created": kString, "lastModified": kString,
	},
}

var komgaBookSpec = map[string]any{
	"id": kString, "seriesId": kString, "seriesTitle": kString, "name": kString,
	"number": kNumber, "created": kNullString, "lastModified": kNullString,
	"fileLastModified": kString, "sizeBytes": kNumber, "size": kString,
	"media": map[string]any{
		"status": kString, "mediaType": kString, "pagesCount": kNumber,
		"mediaProfile": kString, "epubDivinaCompatible": kBool,
	},
	"metadata": map[string]any{
		"title": kString, "titleLock": kBool, "summary": kString, "summaryLock": kBool,
		"number": kString, "numberLock": kBool, "numberSort": kNumber, "numberSortLock": kBool,
		"releaseDate": kNullString, "releaseDateLock": kBool,
		"authors": kArray, "authorsLock": kBool, "tags": kArray, "tagsLock": kBool,
	},
	"readProgress": kNullObject,
}

var komgaPageDtoSpec = map[string]any{
	"number": kNumber, "fileName": kString, "mediaType": kString, "width": kNumber, "height": kNumber,
}

var komgaLibrarySpec = map[string]any{"id": kString, "name": kString}

var komgaReadListSpec = map[string]any{
	"id": kString, "name": kString, "summary": kString, "bookIds": kArray,
	"createdDate": kString, "lastModifiedDate": kString, "filtered": kBool,
}

var komgaCollectionSpec = map[string]any{
	"id": kString, "name": kString, "ordered": kBool, "seriesIds": kArray,
	"createdDate": kString, "lastModifiedDate": kString, "filtered": kBool,
}

// checkShape reports every way got differs from spec.
func checkShape(t *testing.T, where string, got, spec any) {
	t.Helper()
	switch sp := spec.(type) {
	case string:
		var ok bool
		switch sp {
		case kString:
			_, ok = got.(string)
		case kNullString:
			_, isStr := got.(string)
			ok = got == nil || isStr
		case kNumber:
			_, ok = got.(float64)
		case kNullNumber:
			_, isNum := got.(float64)
			ok = got == nil || isNum
		case kBool:
			_, ok = got.(bool)
		case kArray:
			_, ok = got.([]any)
		case kNullObject:
			_, isObj := got.(map[string]any)
			ok = got == nil || isObj
		default:
			t.Fatalf("bad spec kind %q", sp)
		}
		if !ok {
			t.Errorf("%s: want %s, got %T %v", where, sp, got, got)
		}
	case map[string]any:
		m, ok := got.(map[string]any)
		if !ok {
			t.Errorf("%s: want an object, got %T %v", where, got, got)
			return
		}
		keys := make([]string, 0, len(sp))
		for k := range sp {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v, present := m[k]
			if !present {
				t.Errorf("%s: missing field %q", where, k)
				continue
			}
			checkShape(t, where+"."+k, v, sp[k])
		}
	case []any:
		arr, ok := got.([]any)
		if !ok {
			t.Errorf("%s: want an array, got %T %v", where, got, got)
			return
		}
		if len(arr) == 0 {
			t.Errorf("%s: empty array, nothing to check its elements against", where)
			return
		}
		for i, el := range arr {
			checkShape(t, fmt.Sprintf("%s[%d]", where, i), el, sp[0])
		}
	default:
		t.Fatalf("bad spec %T", spec)
	}
}

// komgaUser is a Basic sign-in.
type komgaUser struct{ mail, pass string }

var (
	komgaAdmin  = komgaUser{"admin@example.com", "correct horse"}
	komgaReader = komgaUser{"komga-reader@example.com", "reader pass"}
	komgaNobody = komgaUser{}
)

// komgaFix is two libraries: A (the reader's) holds Alpha (three ready books
// and one that cannot be read), Gamma (one book) and collection/read list
// fixtures; B (not the reader's) holds Beta.
type komgaFix struct {
	t          *testing.T
	e          *env
	h          http.Handler
	reader     User
	libA, libB Library
	alpha      Series
	gamma      Series
	beta       Series
	alphaBooks []Book // every book of Alpha, by number: 1, 2, 10 ready, 11 broken
	beta1      Book
	alphaSet   Collection
	betaSet    Collection
	alphaList  ReadList
	betaList   ReadList
}

// jpegBytes encodes a plain image as JPEG (a page the server does not
// serve as PNG).
func jpegBytes(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func newKomgaFix(t *testing.T) *komgaFix {
	t.Helper()
	e := newEnv(t)
	red := pngBytes(t, 40, 60, color.RGBA{200, 0, 0, 255})
	blue := pngBytes(t, 40, 60, color.RGBA{0, 0, 200, 255})
	jpg := jpegBytes(t, 30, 50, color.RGBA{0, 160, 0, 255})
	rootA, rootB := t.TempDir(), t.TempDir()

	info := []byte(`<?xml version="1.0"?><ComicInfo><Series>Alpha</Series><Number>1</Number><Writer>北条 灯</Writer></ComicInfo>`)
	writeCBZ(t, filepath.Join(rootA, "Alpha", "Alpha v01.cbz"), map[string][]byte{"001.jpg": jpg, "002.png": red, "ComicInfo.xml": info})
	writeCBZ(t, filepath.Join(rootA, "Alpha", "Alpha v2.cbz"), map[string][]byte{"001.png": blue})
	writeCBZ(t, filepath.Join(rootA, "Alpha", "Alpha v10.cbz"), map[string][]byte{"001.png": red})
	if err := os.MkdirAll(filepath.Join(rootA, "Alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootA, "Alpha", "Alpha v11.cbz"), []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCBZ(t, filepath.Join(rootA, "Gamma", "Gamma v01.cbz"), map[string][]byte{"001.png": blue})
	writeCBZ(t, filepath.Join(rootB, "Beta", "Beta v01.cbz"), map[string][]byte{"001.png": red})

	f := &komgaFix{t: t, e: e}
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "A", "root": rootA, "kind": "manga"}, 201, &f.libA)
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "B", "root": rootB, "kind": "manga"}, 201, &f.libB)
	e.waitTasks()

	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "読者", "email": komgaReader.mail, "password": komgaReader.pass,
		"role": "user", "allLibraries": false, "libraryIds": []string{f.libA.Id}}, 201, &f.reader)

	f.alpha = f.seriesIn(f.libA.Id, "Alpha")
	f.gamma = f.seriesIn(f.libA.Id, "Gamma")
	f.beta = f.seriesIn(f.libB.Id, "Beta")
	e.admin.json("GET", "/api/v1/series/"+f.alpha.Id+"/books", nil, 200, &f.alphaBooks)
	f.beta1 = f.booksOf(f.beta.Id)[0]

	// Progress: the reader finished Alpha v01 only, so Alpha is in progress.
	// Gamma is untouched.
	if _, err := e.db.Exec(`INSERT INTO read_progress (user_id, book_id, page, completed, read_at) VALUES (?, ?, ?, ?, ?)`,
		f.reader.Id, f.alphaBooks[0].Id, 2, 1, "2026-11-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}

	e.admin.json("POST", "/api/v1/collections", map[string]any{"name": "Alpha set", "seriesIds": []string{f.alpha.Id}}, 201, &f.alphaSet)
	e.admin.json("POST", "/api/v1/collections", map[string]any{"name": "Beta set", "seriesIds": []string{f.beta.Id}}, 201, &f.betaSet)
	e.admin.json("POST", "/api/v1/read-lists", map[string]any{"name": "Alpha reading", "bookIds": []string{f.alphaBooks[0].Id, f.alphaBooks[1].Id}}, 201, &f.alphaList)
	e.admin.json("POST", "/api/v1/read-lists", map[string]any{"name": "Beta reading", "bookIds": []string{f.beta1.Id}}, 201, &f.betaList)

	// The compat handler, with the same dependencies as the main API. The
	// page and thumbnail readers use the same data folder.
	lib := &library.Service{DB: e.db, ThumbDir: filepath.Join(e.data, "thumbs"),
		Pages:        &media.PageCache{Dir: filepath.Join(e.data, "pages"), MaxBytes: 1 << 30},
		PageThumbDir: filepath.Join(e.data, "page-thumbs"), Events: e.hub}
	f.h = NewCompat(Deps{DB: e.db, Version: "test", Library: lib, Auth: auth.Store{DB: e.db},
		Limiter: &auth.Limiter{}, Gate: e.gate, Events: e.hub})

	f.setEnabled(true)
	return f
}

func (f *komgaFix) setEnabled(on bool) {
	f.t.Helper()
	f.e.admin.json("PATCH", "/api/v1/settings", map[string]any{"komgaApiEnabled": on}, 200, nil)
}

func (f *komgaFix) seriesIn(libID, title string) Series {
	f.t.Helper()
	var list []Series
	f.e.admin.json("GET", "/api/v1/series?libraryId="+libID, nil, 200, &list)
	for _, s := range list {
		if s.Title == title {
			return s
		}
	}
	f.t.Fatalf("series %q not in library %s: %+v", title, libID, list)
	return Series{}
}

func (f *komgaFix) booksOf(seriesID string) []Book {
	f.t.Helper()
	var list []Book
	f.e.admin.json("GET", "/api/v1/series/"+seriesID+"/books", nil, 200, &list)
	return list
}

// do sends a GET as user (or without credentials when user is komgaNobody).
func (f *komgaFix) do(u komgaUser, target string) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if u != komgaNobody {
		req.SetBasicAuth(u.mail, u.pass)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return rec
}

// get expects status want and decodes the body into out (when not nil).
func (f *komgaFix) get(u komgaUser, target string, want int, out any) {
	f.t.Helper()
	rec := f.do(u, target)
	if rec.Code != want {
		f.t.Fatalf("GET %s: status %d, want %d: %s", target, rec.Code, want, rec.Body)
	}
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			f.t.Fatalf("GET %s: %v: %s", target, err, rec.Body)
		}
	}
}

// anyJSON decodes a 200 body into a generic value for shape checks.
func (f *komgaFix) anyJSON(u komgaUser, target string) any {
	f.t.Helper()
	var v any
	f.get(u, target, 200, &v)
	return v
}

// page is a decoded page wrapper: its content and the total.
type komgaListing struct {
	Content       []map[string]any `json:"content"`
	TotalElements int              `json:"totalElements"`
	TotalPages    int              `json:"totalPages"`
	Last          bool             `json:"last"`
	First         bool             `json:"first"`
	Number        int              `json:"number"`
	Size          int              `json:"size"`
	Empty         bool             `json:"empty"`
}

func (f *komgaFix) listing(u komgaUser, target string) komgaListing {
	f.t.Helper()
	var l komgaListing
	f.get(u, target, 200, &l)
	return l
}

// names returns the content's "name" (series, read list, collection) or
// "title" (books) fields in order.
func komgaNames(l komgaListing) []string {
	var out []string
	for _, c := range l.Content {
		if n, ok := c["name"].(string); ok {
			out = append(out, n)
		} else if n, ok := c["title"].(string); ok {
			out = append(out, n)
		}
	}
	return out
}

// ids returns the content's "id" fields in order.
func komgaIDs(l komgaListing) []string {
	var out []string
	for _, c := range l.Content {
		out = append(out, c["id"].(string))
	}
	return out
}

func mustEqual(t *testing.T, what string, got, want any) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("%s: got %v, want %v", what, got, want)
	}
}

// ─── Access and switch ───────────────────────────────────────────────────────

func TestKomgaOffAndSignIn(t *testing.T) {
	f := newKomgaFix(t)

	// Signed in with Basic, the API answers.
	f.get(komgaAdmin, "/komga/api/v1/libraries", 200, nil)

	// No credentials: 401 with a Basic challenge, which OkHttp needs to retry.
	rec := f.do(komgaNobody, "/komga/api/v1/series")
	if rec.Code != http.StatusUnauthorized || !strings.HasPrefix(rec.Header().Get("WWW-Authenticate"), "Basic") {
		t.Errorf("anonymous: %d %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
	// A wrong password is refused the same way.
	req := httptest.NewRequest(http.MethodGet, "/komga/api/v1/series", nil)
	req.SetBasicAuth(komgaReader.mail, "wrong password")
	rec = httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong password: %d", rec.Code)
	}

	// Switched off: every path answers 404, even for a signed-in caller.
	f.setEnabled(false)
	if rec := f.do(komgaAdmin, "/komga/api/v1/series"); rec.Code != http.StatusNotFound {
		t.Errorf("switched off: %d", rec.Code)
	}
	f.setEnabled(true)
	f.get(komgaAdmin, "/komga/api/v1/series", 200, nil)
}

// ─── Shapes ──────────────────────────────────────────────────────────────────

func TestKomgaShapes(t *testing.T) {
	f := newKomgaFix(t)
	a := komgaAdmin
	alpha, alphaV01 := f.alpha.Id, f.alphaBooks[0].Id

	checkShape(t, "series list", f.anyJSON(a, "/komga/api/v1/series?size=5"), komgaPageSpec(komgaSeriesSpec))
	checkShape(t, "series", f.anyJSON(a, "/komga/api/v1/series/"+alpha), komgaSeriesSpec)
	checkShape(t, "series books", f.anyJSON(a, "/komga/api/v1/series/"+alpha+"/books?unpaged=true&media_status=READY"),
		komgaPageSpec(komgaBookSpec))
	checkShape(t, "books list", f.anyJSON(a, "/komga/api/v1/books?unpaged=true"), komgaPageSpec(komgaBookSpec))
	checkShape(t, "book", f.anyJSON(a, "/komga/api/v1/books/"+alphaV01), komgaBookSpec)
	checkShape(t, "pages", f.anyJSON(a, "/komga/api/v1/books/"+alphaV01+"/pages"), []any{komgaPageDtoSpec})
	checkShape(t, "libraries", f.anyJSON(a, "/komga/api/v1/libraries"), []any{komgaLibrarySpec})
	checkShape(t, "collections", f.anyJSON(a, "/komga/api/v1/collections?unpaged=true"), komgaPageSpec(komgaCollectionSpec))
	checkShape(t, "collection series", f.anyJSON(a, "/komga/api/v1/collections/"+f.alphaSet.Id+"/series?unpaged=true"),
		komgaPageSpec(komgaSeriesSpec))
	checkShape(t, "read lists", f.anyJSON(a, "/komga/api/v1/readlists?unpaged=true"), komgaPageSpec(komgaReadListSpec))
	checkShape(t, "read list", f.anyJSON(a, "/komga/api/v1/readlists/"+f.alphaList.Id), komgaReadListSpec)
	checkShape(t, "read list books", f.anyJSON(a, "/komga/api/v1/readlists/"+f.alphaList.Id+"/books?unpaged=true"),
		komgaPageSpec(komgaBookSpec))
	checkShape(t, "genres", f.anyJSON(a, "/komga/api/v1/genres"), kArray)
	checkShape(t, "tags", f.anyJSON(a, "/komga/api/v1/tags"), kArray)
	checkShape(t, "publishers", f.anyJSON(a, "/komga/api/v1/publishers"), kArray)
	checkShape(t, "authors", f.anyJSON(a, "/komga/api/v1/authors"), []any{komgaAuthorSpec})

	// The author and the aggregated book metadata of the book with ComicInfo.
	var book map[string]any
	f.get(a, "/komga/api/v1/books/"+alphaV01, 200, &book)
	meta := book["metadata"].(map[string]any)
	checkShape(t, "book authors", meta["authors"], []any{komgaAuthorSpec})
	mustEqual(t, "book author role", meta["authors"].([]any)[0].(map[string]any)["role"], "writer")
	// The counts are per caller: the reader finished Alpha v01.
	var series map[string]any
	f.get(komgaReader, "/komga/api/v1/series/"+alpha, 200, &series)
	mustEqual(t, "series booksCount", series["booksCount"], 4)
	mustEqual(t, "series booksReadCount", series["booksReadCount"], 1)
	mustEqual(t, "series booksInProgressCount", series["booksInProgressCount"], 0)
	mustEqual(t, "series booksUnreadCount", series["booksUnreadCount"], 3)
	switch st := series["metadata"].(map[string]any)["status"]; st {
	case "ONGOING", "ENDED", "HIATUS", "ABANDONED":
	default:
		t.Errorf("series status %v is not a Komga status", st)
	}

	// Page 1 of Alpha v01 is the first image in the archive (the JPEG).
	var pages []map[string]any
	f.get(a, "/komga/api/v1/books/"+alphaV01+"/pages", 200, &pages)
	mustEqual(t, "page 1 fileName", pages[0]["fileName"], "001.jpg")
	mustEqual(t, "page 1 mediaType", pages[0]["mediaType"], "image/jpeg")
	mustEqual(t, "page 1 size", fmt.Sprint(pages[0]["width"], "x", pages[0]["height"]), "30x50")

	// Thumbnails: series, book, read list (the first book's).
	for _, u := range []string{
		"/komga/api/v1/series/" + alpha + "/thumbnail",
		"/komga/api/v1/books/" + alphaV01 + "/thumbnail",
		"/komga/api/v1/readlists/" + f.alphaList.Id + "/thumbnail",
	} {
		rec := f.do(a, u)
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/jpeg" {
			t.Errorf("thumbnail %s: %d %q", u, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
}

// ─── Paging ──────────────────────────────────────────────────────────────────

func TestKomgaPaging(t *testing.T) {
	f := newKomgaFix(t)
	r := komgaReader

	// Default size is 20: both of the reader's series on one page.
	l := f.listing(r, "/komga/api/v1/series")
	mustEqual(t, "default size", l.Size, 20)
	mustEqual(t, "default total", l.TotalElements, 2)
	mustEqual(t, "default pages", l.TotalPages, 1)
	mustEqual(t, "default last", l.Last, true)
	mustEqual(t, "default names", komgaNames(l), []string{"Alpha", "Gamma"})

	// Pages are 0-based.
	l = f.listing(r, "/komga/api/v1/series?size=1&page=0")
	mustEqual(t, "page 0 names", komgaNames(l), []string{"Alpha"})
	mustEqual(t, "page 0 totalPages", l.TotalPages, 2)
	mustEqual(t, "page 0 first/last", fmt.Sprint(l.First, l.Last), "true false")
	l = f.listing(r, "/komga/api/v1/series?size=1&page=1")
	mustEqual(t, "page 1 names", komgaNames(l), []string{"Gamma"})
	mustEqual(t, "page 1 number", l.Number, 1)
	mustEqual(t, "page 1 first/last", fmt.Sprint(l.First, l.Last), "false true")

	// Past the end: empty content, still the total.
	l = f.listing(r, "/komga/api/v1/series?size=1&page=5")
	mustEqual(t, "past end empty", fmt.Sprint(l.Empty, len(l.Content), l.TotalElements), "true 0 2")

	// unpaged=true returns everything on one page.
	l = f.listing(r, "/komga/api/v1/series?unpaged=true")
	mustEqual(t, "unpaged names", komgaNames(l), []string{"Alpha", "Gamma"})
	mustEqual(t, "unpaged pages", fmt.Sprint(l.TotalPages, l.Last), "1 true")

	// The admin sees Beta too.
	mustEqual(t, "admin total", f.listing(komgaAdmin, "/komga/api/v1/series?size=2").TotalElements, 3)

	// Books of a series page the same way: numbers 1, 2 and 10 are ready.
	var books []map[string]any
	var bl komgaListing
	f.get(komgaAdmin, "/komga/api/v1/series/"+f.alpha.Id+"/books?size=2&page=1&media_status=READY", 200, &bl)
	books = bl.Content
	mustEqual(t, "book page total", bl.TotalElements, 3)
	mustEqual(t, "book page numbers", len(books), 1)
	mustEqual(t, "book page number", books[0]["number"], 10.0)
}

// ─── Filters ─────────────────────────────────────────────────────────────────

func TestKomgaSeriesFilters(t *testing.T) {
	f := newKomgaFix(t)
	r := komgaReader

	cases := []struct {
		name, target string
		who          komgaUser
		want         []string
	}{
		{"library A", "/komga/api/v1/series?library_id=" + f.libA.Id + "&unpaged=true", r, []string{"Alpha", "Gamma"}},
		// The reader has no access to B: its series are not listed.
		{"library B for reader", "/komga/api/v1/series?library_id=" + f.libB.Id + "&unpaged=true", r, nil},
		{"library B for admin", "/komga/api/v1/series?library_id=" + f.libB.Id + "&unpaged=true", komgaAdmin, []string{"Beta"}},
		{"two libraries for admin", "/komga/api/v1/series?library_id=" + f.libA.Id + "," + f.libB.Id + "&unpaged=true", komgaAdmin, []string{"Alpha", "Beta", "Gamma"}},
		{"search", "/komga/api/v1/series?search=Alpha&unpaged=true", r, []string{"Alpha"}},
		{"search no match", "/komga/api/v1/series?search=Zeta&unpaged=true", r, nil},
		{"search other library hidden", "/komga/api/v1/series?search=Beta&unpaged=true", r, nil},
		{"read status in progress", "/komga/api/v1/series?read_status=IN_PROGRESS&unpaged=true", r, []string{"Alpha"}},
		{"read status unread", "/komga/api/v1/series?read_status=UNREAD&unpaged=true", r, []string{"Gamma"}},
		{"read status both", "/komga/api/v1/series?read_status=UNREAD&read_status=IN_PROGRESS&unpaged=true", r, []string{"Alpha", "Gamma"}},
		{"read status read", "/komga/api/v1/series?read_status=READ&unpaged=true", r, nil},
		{"unknown status ignored", "/komga/api/v1/series?status=BOGUS&unpaged=true", r, []string{"Alpha", "Gamma"}},
		{"unknown sort ignored", "/komga/api/v1/series?sort=nonsense,asc&unpaged=true", r, []string{"Alpha", "Gamma"}},
		{"sort by title desc", "/komga/api/v1/series?sort=metadata.titleSort,desc&unpaged=true", r, []string{"Gamma", "Alpha"}},
		{"deleted only is empty", "/komga/api/v1/series?deleted=true&unpaged=true", r, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ft := *f
			ft.t = t
			l := ft.listing(c.who, c.target)
			mustEqual(t, "names", komgaNames(l), c.want)
			if len(c.want) == 0 && l.TotalElements != 0 {
				t.Errorf("total %d, want 0", l.TotalElements)
			}
		})
	}

	// Books: read_status uses the reader's progress (Alpha v01 is finished).
	bl := f.listing(r, "/komga/api/v1/books?read_status=READ&unpaged=true")
	mustEqual(t, "read books", komgaIDs(bl), []string{f.alphaBooks[0].Id})
	bl = f.listing(r, "/komga/api/v1/books?read_status=IN_PROGRESS&unpaged=true")
	mustEqual(t, "in progress books", len(bl.Content), 0)
}

// ─── Books of a series ───────────────────────────────────────────────────────

func TestKomgaSeriesBooksReadyInNumberOrder(t *testing.T) {
	f := newKomgaFix(t)
	for _, who := range []komgaUser{komgaReader, komgaAdmin} {
		var l komgaListing
		f.get(who, "/komga/api/v1/series/"+f.alpha.Id+"/books?unpaged=true&media_status=READY", 200, &l)
		var numbers []any
		for _, b := range l.Content {
			numbers = append(numbers, b["number"])
		}
		// v01, v2, v10 are ready and sorted by number, not by file name.
		mustEqual(t, "numbers", fmt.Sprint(numbers), "[1 2 10]")
		mustEqual(t, "unpaged total", l.TotalElements, 3)
		for _, b := range l.Content {
			mustEqual(t, "media status", b["media"].(map[string]any)["status"], "READY")
		}
	}
	// Without media_status the list is the same: a book that cannot be read is
	// left out.
	var l komgaListing
	f.get(komgaReader, "/komga/api/v1/series/"+f.alpha.Id+"/books?unpaged=true", 200, &l)
	mustEqual(t, "default media filter", len(l.Content), 3)
}

// ─── Pages and convert=png ───────────────────────────────────────────────────

func TestKomgaPagesOneBasedAndConvert(t *testing.T) {
	f := newKomgaFix(t)
	r := komgaReader
	v01 := f.alphaBooks[0].Id

	var pages []map[string]any
	f.get(r, "/komga/api/v1/books/"+v01+"/pages", 200, &pages)
	var numbers []any
	for _, p := range pages {
		numbers = append(numbers, p["number"])
	}
	mustEqual(t, "page numbers", fmt.Sprint(numbers), "[1 2]")

	// Without convert the page is served as stored.
	rec := f.do(r, "/komga/api/v1/books/"+v01+"/pages/1")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("page 1: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}

	// convert=png re-encodes it as PNG at the same size.
	rec = f.do(r, "/komga/api/v1/books/"+v01+"/pages/1?convert=png")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("page 1 converted: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(rec.Body.Bytes()))
	if err != nil || cfg.Width != 30 || cfg.Height != 50 {
		t.Errorf("converted page: %+v %v", cfg, err)
	}
	// Page 2 is already PNG and converts to PNG too.
	rec = f.do(r, "/komga/api/v1/books/"+v01+"/pages/2?convert=png")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("page 2 converted: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	// Pages are 1-based: page 0 and a page past the end are 404.
	for _, p := range []string{"0", "3", "9"} {
		if rec := f.do(r, "/komga/api/v1/books/"+v01+"/pages/"+p); rec.Code != http.StatusNotFound {
			t.Errorf("page %s: %d, want 404", p, rec.Code)
		}
	}
	// The unreadable book has no pages.
	var broken []map[string]any
	f.get(r, "/komga/api/v1/books/"+f.alphaBooks[3].Id+"/pages", 200, &broken)
	mustEqual(t, "unreadable book pages", len(broken), 0)
}

// pngHeaderOnly is a PNG that is a signature, an IHDR of w by h 8-bit RGB and
// an IEND: its header claims the size, and there is no pixel data.
func pngHeaderOnly(w, h uint32) []byte {
	chunk := func(typ string, data []byte) []byte {
		var b bytes.Buffer
		_ = binary.Write(&b, binary.BigEndian, uint32(len(data)))
		b.WriteString(typ)
		b.Write(data)
		_ = binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), data...)))
		return b.Bytes()
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], w)
	binary.BigEndian.PutUint32(ihdr[4:8], h)
	ihdr[8], ihdr[9] = 8, 2
	out := []byte("\x89PNG\r\n\x1a\n")
	out = append(out, chunk("IHDR", ihdr)...)
	return append(out, chunk("IEND", nil)...)
}

func TestKomgaConvertGuards(t *testing.T) {
	small := pngBytes(t, 3, 2, color.RGBA{1, 2, 3, 255})
	img, err := komgaDecodePage(small)
	if err != nil {
		t.Fatalf("small page: %v", err)
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatalf("small page encode: %v", err)
	}
	if cfg, err := png.DecodeConfig(bytes.NewReader(out.Bytes())); err != nil || cfg.Width != 3 || cfg.Height != 2 {
		t.Fatalf("small page output: %+v %v", cfg, err)
	}
	// 6000 x 6000 is 36e6 pixels: over komgaConvertMaxPixels (32e6) though
	// under media.MaxPixels, so it is refused from the header.
	if _, err := komgaDecodePage(pngHeaderOnly(6000, 6000)); !errors.Is(err, errPageTooLarge) {
		t.Fatalf("huge header: %v, want errPageTooLarge", err)
	}
	if _, err := komgaDecodePage([]byte("not an image")); !errors.Is(err, errPageFormat) {
		t.Fatalf("garbage: %v, want errPageFormat", err)
	}
}

func TestKomgaConvertSlots(t *testing.T) {
	f := newKomgaFix(t)
	r := komgaReader
	page := "/komga/api/v1/books/" + f.alphaBooks[0].Id + "/pages/1?convert=png"

	// Five conversions in a row all answer, so each one gives its slot back.
	for i := range 5 {
		rec := f.do(r, page)
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
			t.Fatalf("convert %d: %d %q", i, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
	mustEqual(t, "slots in use after 5 converts", len(convertSlots), 0)

	// With every slot taken a conversion waits. A client that goes away while
	// it waits gets no page, and the slots it never took stay as they were.
	for range komgaConvertSlots {
		convertSlots <- struct{}{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, page, nil).WithContext(ctx)
	req.SetBasicAuth(r.mail, r.pass)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		f.h.ServeHTTP(rec, req)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("a conversion ran with every slot taken")
	case <-time.After(100 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a waiting conversion did not return after its client left")
	}
	if rec.Header().Get("Content-Type") == "image/png" {
		t.Fatal("a client that left was sent a page")
	}
	for range komgaConvertSlots {
		<-convertSlots
	}
	mustEqual(t, "slots in use at the end", len(convertSlots), 0)
}

// ─── Access ──────────────────────────────────────────────────────────────────

func TestKomgaAccessIsScoped(t *testing.T) {
	f := newKomgaFix(t)
	r := komgaReader
	beta, betaBook := f.beta.Id, f.beta1.Id

	// Another library's series, book, pages and thumbnails: 404.
	for _, target := range []string{
		"/komga/api/v1/series/" + beta,
		"/komga/api/v1/series/" + beta + "/books",
		"/komga/api/v1/series/" + beta + "/thumbnail",
		"/komga/api/v1/books/" + betaBook,
		"/komga/api/v1/books/" + betaBook + "/thumbnail",
		"/komga/api/v1/books/" + betaBook + "/pages",
		"/komga/api/v1/books/" + betaBook + "/pages/1",
		"/komga/api/v1/books/" + betaBook + "/pages/1?convert=png",
		"/komga/api/v1/collections/" + f.betaSet.Id + "/series",
		"/komga/api/v1/readlists/" + f.betaList.Id,
		"/komga/api/v1/readlists/" + f.betaList.Id + "/books",
		"/komga/api/v1/readlists/" + f.betaList.Id + "/thumbnail",
	} {
		if rec := f.do(r, target); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: %d, want 404 (%s)", target, rec.Code, rec.Body)
		}
	}
	// The admin may see all of them.
	f.get(komgaAdmin, "/komga/api/v1/series/"+beta, 200, nil)
	f.get(komgaAdmin, "/komga/api/v1/books/"+betaBook+"/pages/1?convert=png", 200, nil)

	// Lists leave them out.
	var libs []map[string]any
	f.get(r, "/komga/api/v1/libraries", 200, &libs)
	mustEqual(t, "reader libraries", len(libs), 1)
	mustEqual(t, "reader library", libs[0]["id"], f.libA.Id)
	mustEqual(t, "reader collections", komgaNames(f.listing(r, "/komga/api/v1/collections?unpaged=true")), []string{"Alpha set"})
	mustEqual(t, "reader read lists", komgaNames(f.listing(r, "/komga/api/v1/readlists?unpaged=true")), []string{"Alpha reading"})
	mustEqual(t, "reader books", len(f.listing(r, "/komga/api/v1/books?unpaged=true").Content), 4)
	// Books in the reader's read list are only the visible ones.
	mustEqual(t, "reader read list books", len(f.listing(r, "/komga/api/v1/readlists/"+f.alphaList.Id+"/books?unpaged=true").Content), 2)
	for _, b := range f.listing(r, "/komga/api/v1/books?unpaged=true").Content {
		if b["id"] == betaBook {
			t.Errorf("reader sees Beta's book in /books")
		}
	}
}
