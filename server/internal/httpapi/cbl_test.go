package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/tabunugoku/cominavi/server/internal/auth"
)

// cblDocXML is a ReadingList document with the given Books children.
func cblDocXML(name string, books ...string) string {
	return `<?xml version="1.0" encoding="utf-8"?><ReadingList><Name>` + name + `</Name><Books>` +
		strings.Join(books, "") + `</Books></ReadingList>`
}

func cblBookXML(series, number string) string {
	return `<Book Series="` + series + `" Number="` + number + `"/>`
}

// postCBL posts body to the import route as c.
func postCBL(t *testing.T, c client, query, body string, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/read-lists/import"+query, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Origin", "http://"+req.Host)
	if c.cookie != "" {
		req.AddCookie(&http.Cookie{Name: cookieName, Value: c.cookie})
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	return rec
}

func decodeCBLResult(t *testing.T, rec *httptest.ResponseRecorder) CblImportResult {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("import status %d: %s", rec.Code, rec.Body)
	}
	var res CblImportResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("import body: %v", err)
	}
	return res
}

func matchedIDs(res CblImportResult) []string {
	out := []string{}
	for _, m := range res.Matched {
		out = append(out, m.Book.Id)
	}
	return out
}

// TestCBLMatchingFoldsWidthCaseAndSpacing: full and half width, case,
// spacing, numbers written with leading zeros or a trailing ".0", and an
// alternate title all reach the same books, in file order, and a book listed
// twice appears once in the new list.
func TestCBLMatchingFoldsWidthCaseAndSpacing(t *testing.T) {
	f := newListFixture(t)
	if f.books["Alpha"][0].Number != 1 || f.books["Alpha"][1].Number != 2 || f.books["Charlie"][1].Number != 2 {
		t.Fatalf("fixture numbers %v %v %v, want 1 and 2", f.books["Alpha"][0].Number, f.books["Alpha"][1].Number, f.books["Charlie"][1].Number)
	}
	if _, err := f.e.db.Exec(`UPDATE series SET alternate_titles = ? WHERE id = ?`, `["Ｄｅｌｔａ Saga"]`, f.sid("Charlie")); err != nil {
		t.Fatal(err)
	}
	body := cblDocXML("",
		cblBookXML("ＡＬＰＨＡ", "1"),
		cblBookXML(" a l p h a ", "2"),
		cblBookXML("bravo", "001"),
		cblBookXML("CHARLIE", "2.0"),
		cblBookXML("delta saga", "2"),
	)
	res := decodeCBLResult(t, postCBL(t, f.e.admin, "", body))
	want := []string{f.bid("Alpha", 0), f.bid("Alpha", 1), f.bid("Bravo", 0), f.bid("Charlie", 1), f.bid("Charlie", 1)}
	if !slices.Equal(matchedIDs(res), want) {
		t.Fatalf("matched %v, want %v", matchedIDs(res), want)
	}
	if len(res.Unmatched) != 0 {
		t.Fatalf("unmatched %+v, want none", res.Unmatched)
	}
	if res.Name != "読み込んだリスト" || res.ReadList == nil {
		t.Fatalf("name %q, read list %+v", res.Name, res.ReadList)
	}
	if !slices.Equal(res.ReadList.BookIds, []string{f.bid("Alpha", 0), f.bid("Alpha", 1), f.bid("Bravo", 0), f.bid("Charlie", 1)}) {
		t.Fatalf("read list books %v", res.ReadList.BookIds)
	}
}

// TestCBLUnmatchedAndNumbers: numbers that are not numbers, numbers with no
// book, unknown series and entries without a Number are listed as unmatched,
// in order, with their attributes.
func TestCBLUnmatchedAndNumbers(t *testing.T) {
	f := newListFixture(t)
	body := cblDocXML("Mixed",
		cblBookXML("Alpha", "1.5"),
		cblBookXML("Alpha", "x"),
		cblBookXML("Alpha", ""),
		cblBookXML("Alpha", "NaN"),
		cblBookXML("Nowhere", "1"),
		`<Book Series="Bravo"/>`,
		cblBookXML("Bravo", "2"),
	)
	res := decodeCBLResult(t, postCBL(t, f.e.admin, "", body))
	if res.Name != "Mixed" {
		t.Fatalf("name %q", res.Name)
	}
	if !slices.Equal(matchedIDs(res), []string{f.bid("Bravo", 1)}) {
		t.Fatalf("matched %v, want Bravo 2 only", matchedIDs(res))
	}
	if len(res.Unmatched) != 6 {
		t.Fatalf("unmatched %+v, want 6", res.Unmatched)
	}
	if e := res.Unmatched[0]; e.Series != "Alpha" || e.Number != "1.5" || e.Volume != "" || e.Year != "" {
		t.Fatalf("first unmatched %+v", e)
	}
	if e := res.Unmatched[5]; e.Series != "Bravo" || e.Number != "" {
		t.Fatalf("last unmatched %+v", e)
	}
}

// TestCBLDryRunWritesNothing: dryRun returns the matches with a null read
// list and creates nothing; the real import then creates the list.
func TestCBLDryRunWritesNothing(t *testing.T) {
	f := newListFixture(t)
	body := cblDocXML("Preview", cblBookXML("Alpha", "1"))

	rec := postCBL(t, f.e.admin, "?dryRun=true", body)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"readList":null`) {
		t.Fatalf("dry run %d: %s", rec.Code, rec.Body)
	}
	res := decodeCBLResult(t, rec)
	if res.ReadList != nil || !slices.Equal(matchedIDs(res), []string{f.bid("Alpha", 0)}) {
		t.Fatalf("dry run result %+v", res)
	}
	var lists []ReadList
	f.e.admin.json("GET", "/api/v1/read-lists", nil, 200, &lists)
	if len(lists) != 0 {
		t.Fatalf("dry run created %d lists", len(lists))
	}

	res = decodeCBLResult(t, postCBL(t, f.e.admin, "", body))
	if res.ReadList == nil || !slices.Equal(res.ReadList.BookIds, []string{f.bid("Alpha", 0)}) {
		t.Fatalf("import result %+v", res)
	}
	f.e.admin.json("GET", "/api/v1/read-lists", nil, 200, &lists)
	if len(lists) != 1 || lists[0].Name != "Preview" {
		t.Fatalf("lists after import %+v", lists)
	}
}

// TestCBLNothingMatched: without dryRun, a file with no match is refused
// (nothing is created); with dryRun it is an ordinary result.
func TestCBLNothingMatched(t *testing.T) {
	f := newListFixture(t)
	body := cblDocXML("", cblBookXML("Nowhere", "1"))
	rec := postCBL(t, f.e.admin, "", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", rec.Code, rec.Body)
	}
	if got := errorBody(t, rec.Body.Bytes()); got.Code != "bad_request.cbl_no_match" {
		t.Fatalf("error %+v", got)
	}
	res := decodeCBLResult(t, postCBL(t, f.e.admin, "?dryRun=true", body))
	if len(res.Matched) != 0 || len(res.Unmatched) != 1 || res.ReadList != nil {
		t.Fatalf("dry run %+v", res)
	}
	var lists []ReadList
	f.e.admin.json("GET", "/api/v1/read-lists", nil, 200, &lists)
	if len(lists) != 0 {
		t.Fatalf("refused import created %d lists", len(lists))
	}
}

// TestCBLHidesOtherLibraries: series in a library the caller cannot see never
// match, even when the title is exact. The route itself is administrator
// only, so the scope is checked by calling the handler with a restricted
// caller's context.
func TestCBLHidesOtherLibraries(t *testing.T) {
	f := newListFixture(t)
	admin := f.e.admin
	var restricted User
	admin.json("POST", "/api/v1/users", map[string]any{"name": "restricted", "email": "restricted@example.com",
		"password": "reader pass", "role": "user", "allLibraries": false, "libraryIds": []string{f.libA.Id}}, 201, &restricted)

	srv := &server{Deps: Deps{DB: f.e.db}}
	ctx := context.WithValue(context.Background(), userKey, auth.User{ID: restricted.Id, Name: "restricted",
		Role: "user", AllLibraries: false, LibraryIDs: []string{f.libA.Id}})
	dry := true
	out, err := srv.ImportReadList(ctx, ImportReadListRequestObject{
		Params: ImportReadListParams{DryRun: &dry},
		Body:   strings.NewReader(cblDocXML("", cblBookXML("Alpha", "1"), cblBookXML("Charlie", "1"))),
	})
	if err != nil {
		t.Fatal(err)
	}
	res, ok := out.(ImportReadList200JSONResponse)
	if !ok {
		t.Fatalf("response %T", out)
	}
	if !slices.Equal(matchedIDs(CblImportResult(res)), []string{f.bid("Alpha", 0)}) {
		t.Fatalf("restricted matched %v, want Alpha only", matchedIDs(CblImportResult(res)))
	}
	if len(res.Unmatched) != 1 || res.Unmatched[0].Series != "Charlie" {
		t.Fatalf("restricted unmatched %+v, want Charlie", res.Unmatched)
	}

	r := f.e.login("restricted", "reader pass")
	rec := postCBL(t, r, "", cblDocXML("", cblBookXML("Alpha", "1")))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("restricted import status %d, want 403", rec.Code)
	}
}

// TestCBLMalformed: documents that are not well-formed, have the wrong root,
// or use entities the parser does not expand, are refused with the same
// code. A UTF-16 file with its byte order mark is accepted.
func TestCBLMalformed(t *testing.T) {
	f := newListFixture(t)
	bad := map[string]string{
		"truncated":     `<ReadingList><Books>`,
		"empty":         ``,
		"not xml":       `hello`,
		"wrong root":    `<Foo><Book Series="Alpha" Number="1"/></Foo>`,
		"dtd entity":    `<!DOCTYPE r [<!ENTITY x "boom">]><ReadingList><Name>&x;</Name></ReadingList>`,
		"utf-16 no bom": `<?xml version="1.0" encoding="UTF-16"?><ReadingList/>`,
		"unknown enc":   `<?xml version="1.0" encoding="shift_jis"?><ReadingList/>`,
	}
	for name, body := range bad {
		rec := postCBL(t, f.e.admin, "", body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400: %s", name, rec.Code, rec.Body)
		}
		if got := errorBody(t, rec.Body.Bytes()); got.Code != "bad_request.cbl_malformed" {
			t.Fatalf("%s: code %q", name, got.Code)
		}
	}

	raw := []byte{0xFF, 0xFE}
	for _, u := range utf16.Encode([]rune(cblDocXML("", cblBookXML("Alpha", "1")))) {
		raw = append(raw, byte(u), byte(u>>8))
	}
	res := decodeCBLResult(t, postCBL(t, f.e.admin, "?dryRun=true", string(raw)))
	if !slices.Equal(matchedIDs(res), []string{f.bid("Alpha", 0)}) {
		t.Fatalf("utf-16 matched %v", matchedIDs(res))
	}
}

// TestCBLTooLarge: a body over the route's cap is refused with the size code
// (the route allows 5 MB; the request body cap lets that much through).
func TestCBLTooLarge(t *testing.T) {
	f := newListFixture(t)
	body := cblDocXML("", cblBookXML("Alpha", "1")) + "<!--" + strings.Repeat("x", 5<<20) + "-->"
	rec := postCBL(t, f.e.admin, "", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
	if got := errorBody(t, rec.Body.Bytes()); got.Code != "bad_request.cbl_too_large" {
		t.Fatalf("code %q", got.Code)
	}

	srv := &server{Deps: Deps{DB: f.e.db}}
	out, err := srv.ImportReadList(context.Background(), ImportReadListRequestObject{
		Body: strings.NewReader(strings.Repeat(" ", maxCBL+1)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := out.(ImportReadList400JSONResponse); !ok || got.Code != "bad_request.cbl_too_large" {
		t.Fatalf("handler over the limit: %#v", out)
	}
}

// TestCBLDefaultNameFollowsLanguage: a file without a Name gets the default
// name in the request's language.
func TestCBLDefaultNameFollowsLanguage(t *testing.T) {
	f := newListFixture(t)
	body := cblDocXML("  ", cblBookXML("Alpha", "1"))
	res := decodeCBLResult(t, postCBL(t, f.e.admin, "?dryRun=true", body, "Accept-Language", "en"))
	if res.Name != "Imported read list" {
		t.Fatalf("english default name %q", res.Name)
	}
	res = decodeCBLResult(t, postCBL(t, f.e.admin, "?dryRun=true", body))
	if res.Name != "読み込んだリスト" {
		t.Fatalf("japanese default name %q", res.Name)
	}
}
