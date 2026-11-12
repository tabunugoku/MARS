package httpapi

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// errorBody decodes an error response.
func errorBody(t *testing.T, body []byte) Error {
	t.Helper()
	var e Error
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("error body %q: %v", body, err)
	}
	return e
}

// TestErrorsFollowAcceptLanguage: the same error carries the same code in
// both languages, with the text in the requested one.
func TestErrorsFollowAcceptLanguage(t *testing.T) {
	e := newEnv(t)
	ja := e.admin.do("GET", "/api/v1/libraries/nope", nil)
	en := e.admin.do("GET", "/api/v1/libraries/nope", nil, "Accept-Language", "en-US,en;q=0.9")
	if ja.Code != 404 || en.Code != 404 {
		t.Fatalf("status ja %d, en %d", ja.Code, en.Code)
	}
	jaErr, enErr := errorBody(t, ja.Body.Bytes()), errorBody(t, en.Body.Bytes())
	if jaErr.Code != "not_found.library" || enErr.Code != jaErr.Code {
		t.Fatalf("codes ja %q, en %q", jaErr.Code, enErr.Code)
	}
	if jaErr.Message != "ライブラリが見つかりません" {
		t.Fatalf("ja message %q", jaErr.Message)
	}
	if enErr.Message != "Library not found" {
		t.Fatalf("en message %q", enErr.Message)
	}

	// Other languages, and no header at all, give Japanese.
	fr := e.admin.do("GET", "/api/v1/libraries/nope", nil, "Accept-Language", "fr, en;q=0.5")
	if got := errorBody(t, fr.Body.Bytes()).Message; got != jaErr.Message {
		t.Fatalf("fr message %q, want Japanese", got)
	}

	// Errors outside the handlers carry codes too: an unknown route, a
	// missing sign-in, and a refused cross-site write.
	route := errorBody(t, e.admin.do("GET", "/api/v1/no-such-route", nil, "Accept-Language", "en").Body.Bytes())
	if route.Code != "not_found.route" || route.Message != "No such API route" {
		t.Fatalf("unknown route: %+v", route)
	}
	anon := errorBody(t, e.anon().do("GET", "/api/v1/libraries", nil, "Accept-Language", "en").Body.Bytes())
	if anon.Code != "unauthorized.sign_in" || anon.Message != "Sign in to continue" {
		t.Fatalf("anonymous: %+v", anon)
	}
	rec := e.anon().do("POST", "/api/v1/auth/login", map[string]any{"username": "x", "password": "y"}, "Origin", "http://evil.example", "Accept-Language", "en")
	if cross := errorBody(t, rec.Body.Bytes()); rec.Code != 403 || cross.Code != "forbidden.cross_site" {
		t.Fatalf("cross-site: status %d, %+v", rec.Code, cross)
	}
}

// fmtVerb matches the printf verbs msg fills (not %%).
var fmtVerb = regexp.MustCompile(`%[-+# 0]*[0-9]*(\.[0-9]+)?[a-zA-Z]`)

// TestCatalogIsComplete: every entry has both texts, and the English keeps
// the verbs of the Japanese.
func TestCatalogIsComplete(t *testing.T) {
	if len(messages) < 50 {
		t.Fatalf("catalog has %d entries", len(messages))
	}
	for code, m := range messages {
		if m.ja == "" || m.en == "" {
			t.Errorf("%s: empty text (ja %q, en %q)", code, m.ja, m.en)
			continue
		}
		if !strings.Contains(code, ".") {
			t.Errorf("%s: code should be kind.what", code)
		}
		jaVerbs, enVerbs := fmtVerb.FindAllString(m.ja, -1), fmtVerb.FindAllString(m.en, -1)
		if strings.Join(jaVerbs, " ") != strings.Join(enVerbs, " ") {
			t.Errorf("%s: verbs ja %v, en %v", code, jaVerbs, enVerbs)
		}
	}
}

// TestHandlersHoldNoJapanese: the handler sources keep their text in the
// catalog. Only string literals count; comments may stay Japanese.
func TestHandlersHoldNoJapanese(t *testing.T) {
	japanese := regexp.MustCompile(`[\p{Hiragana}\p{Katakana}\p{Han}]`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "messages.go" || f == "api.gen.go" {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		af, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		ast.Inspect(af, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err == nil && japanese.MatchString(s) {
				t.Errorf("%s: Japanese text outside messages.go: %s", fset.Position(lit.Pos()), lit.Value)
			}
			return true
		})
	}
	if checked < 10 {
		t.Fatalf("only %d handler files checked", checked)
	}
}

// TestUpdateMeLocale: the language a user chose is stored, returned on every
// read of the user, and only "", "ja" and "en" are accepted.
func TestUpdateMeLocale(t *testing.T) {
	e := newEnv(t)
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "読者", "email": "reader@example.com", "password": "reader pass", "role": "user"}, 201, nil)
	r := e.login("読者", "reader pass")

	var u User
	r.json("GET", "/api/v1/me", nil, 200, &u)
	if u.Locale != "" {
		t.Fatalf("new user locale %q, want empty", u.Locale)
	}
	r.json("PATCH", "/api/v1/me", map[string]any{"name": "読者", "locale": "en"}, 200, &u)
	if u.Locale != "en" {
		t.Fatalf("patched locale %q", u.Locale)
	}
	r.json("GET", "/api/v1/me", nil, 200, &u)
	if u.Locale != "en" {
		t.Fatalf("stored locale %q", u.Locale)
	}
	var list []User
	e.admin.json("GET", "/api/v1/users", nil, 200, &list)
	found := false
	for _, x := range list {
		if x.Name == "読者" {
			found = x.Locale == "en"
		}
	}
	if !found {
		t.Fatalf("listUsers does not show the locale: %+v", list)
	}

	// A name-only update keeps the locale.
	r.json("PATCH", "/api/v1/me", map[string]any{"name": "読者"}, 200, &u)
	if u.Locale != "en" {
		t.Fatalf("name-only update changed locale to %q", u.Locale)
	}
	r.json("PATCH", "/api/v1/me", map[string]any{"name": "読者", "locale": ""}, 200, &u)
	if u.Locale != "" {
		t.Fatalf("cleared locale %q", u.Locale)
	}

	// Anything else is refused, with the error in the request's language.
	bad := r.do("PATCH", "/api/v1/me", map[string]any{"name": "読者", "locale": "fr"}, "Accept-Language", "en")
	if bad.Code != 400 {
		t.Fatalf("locale fr: status %d", bad.Code)
	}
	if got := errorBody(t, bad.Body.Bytes()); got.Code != "bad_request.locale" || got.Message != "Invalid language" {
		t.Fatalf("locale fr error: %+v", got)
	}
	r.json("GET", "/api/v1/me", nil, 200, &u)
	if u.Locale != "" {
		t.Fatalf("refused update stored %q", u.Locale)
	}
}

// TestMediaErrorInEnglish: a book's media reason follows the language. The
// archive's own reasons are translated; anything else is the generic text.
func TestMediaErrorInEnglish(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	red := pngBytes(t, 20, 30, color.RGBA{200, 0, 0, 255})
	writeCBZ(t, filepath.Join(root, "A", "A v01.cbz"), map[string][]byte{"1.png": red})
	l := e.libraryWith("A", root)
	_, books := e.onlySeries(l.Id)
	id := books[0].Id

	cases := map[string]struct{ ja, en string }{
		"archive: unsupported: PDF page 3 is not a single image": {"PDF の 3 ページは1枚の画像ではありません", "PDF page 3 is not a single image"},
		"archive: unsupported: encrypted PDF":                    {"暗号化された PDF です", "This PDF is encrypted"},
		"archive: unsupported: PDF over 512 MB":                  {"PDF が 512 MB を超えています", "This PDF is over 512 MB"},
		"archive: unsupported: something new":                    {"something new", "something new"},
		"open /srv/comics/A/b.cbz: permission denied":            {"ファイルを読み込めませんでした", "The file could not be read"},
	}
	for raw, want := range cases {
		if _, err := e.db.Exec(`UPDATE books SET media_status = 'error', media_error = ? WHERE id = ?`, raw, id); err != nil {
			t.Fatal(err)
		}
		var en, ja Book
		e.admin.json("GET", "/api/v1/books/"+id, nil, 200, &ja)
		rec := e.admin.do("GET", "/api/v1/books/"+id, nil, "Accept-Language", "en")
		if err := json.Unmarshal(rec.Body.Bytes(), &en); err != nil {
			t.Fatal(err)
		}
		if ja.MediaError == nil || *ja.MediaError != want.ja {
			t.Errorf("%q in Japanese: %v, want %q", raw, ja.MediaError, want.ja)
		}
		if en.MediaError == nil || *en.MediaError != want.en {
			t.Errorf("%q in English: %v, want %q", raw, en.MediaError, want.en)
		}
	}
}

// TestWatchReasonsInEnglish: the watcher's reason codes have English text.
func TestWatchReasonsInEnglish(t *testing.T) {
	for _, code := range []string{"watch.limit", "watch.generic"} {
		ja := msg(context.Background(), code)
		if !strings.HasPrefix(ja, "ファイルの監視を始められませんでした") {
			t.Errorf("%s in default (Japanese) context: %q", code, ja)
		}
		enCtx := context.WithValue(context.Background(), localeKey, "en")
		if got := msg(enCtx, code); !strings.HasPrefix(got, "Could not start watching files") || got == ja {
			t.Errorf("%s in English: %q", code, got)
		}
	}
}

// TestTaskLabelsFollowLanguage: a queued scan's label comes from the catalog
// in the request's language: English with Accept-Language en, else Japanese.
func TestTaskLabelsFollowLanguage(t *testing.T) {
	e := newEnv(t)
	l := e.libraryWith("A", t.TempDir())
	e.admin.json("POST", "/api/v1/libraries/"+l.Id+"/scan", nil, 202, nil)
	e.waitTasks()

	scanLabel := func(hdr ...string) string {
		t.Helper()
		var ts []Task
		rec := e.admin.do("GET", "/api/v1/tasks", nil, hdr...)
		if err := json.Unmarshal(rec.Body.Bytes(), &ts); err != nil {
			t.Fatal(err)
		}
		for _, tk := range ts {
			if tk.Kind == Scan {
				return tk.Label
			}
		}
		t.Fatalf("no scan task in %+v", ts)
		return ""
	}
	if got := scanLabel("Accept-Language", "en-US,en;q=0.9"); got != "Scan A" {
		t.Fatalf("English label %q", got)
	}
	if got := scanLabel(); got != "A をスキャン" {
		t.Fatalf("Japanese label %q", got)
	}
}
