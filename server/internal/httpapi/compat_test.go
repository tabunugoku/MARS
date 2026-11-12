package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tabunugoku/cominavi/server/internal/auth"
)

// compatHandler is the /opds/ and /komga/ handler built the way cmd/cominavi
// builds it, on the test environment's database.
func compatHandler(e *env) http.Handler {
	return NewCompat(Deps{DB: e.db, Version: "test", Auth: auth.Store{DB: e.db},
		Limiter: &auth.Limiter{}, Gate: e.gate, Events: e.hub})
}

// compatEnv starts an environment with a reader (email reader@example.com,
// password "reader pass") and, when on is set, turns both compat APIs on.
func compatEnv(t *testing.T, on bool) (*env, http.Handler, string) {
	t.Helper()
	e := newEnv(t)
	var reader User
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "読者", "email": "reader@example.com", "password": "reader pass", "role": "user"}, 201, &reader)
	if on {
		e.admin.json("PATCH", "/api/v1/settings", map[string]any{"opdsEnabled": true, "komgaApiEnabled": true}, 200, nil)
	}
	return e, compatHandler(e), reader.Id
}

// compatGet sends a GET to path with set applied to the request.
func compatGet(h http.Handler, path string, set ...func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	for _, f := range set {
		f(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func basicAuth(name, pass string) func(*http.Request) {
	return func(r *http.Request) { r.SetBasicAuth(name, pass) }
}

func withCompatCookie(v string) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: compatCookie, Value: v}) }
}

func header(k, v string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set(k, v) }
}

// compatCookieFrom returns the cn_compat value a response set, or "".
func compatCookieFrom(rec *httptest.ResponseRecorder) string {
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == compatCookie {
			return ck.Value
		}
	}
	return ""
}

// Paths that only exist once the feature routes are written; until then the
// authenticated request reaches the mux and gets 404.
var compatPaths = []string{"/opds/v1.2/catalog", "/komga/api/v1/series"}

func TestCompatOffAnswers404(t *testing.T) {
	_, h, _ := compatEnv(t, false)
	for _, p := range compatPaths {
		if rec := compatGet(h, p, basicAuth("reader@example.com", "reader pass")); rec.Code != 404 {
			t.Fatalf("%s off with valid credentials: %d", p, rec.Code)
		}
	}
}

func TestCompatChallenge(t *testing.T) {
	_, h, _ := compatEnv(t, true)
	for _, p := range compatPaths {
		rec := compatGet(h, p)
		if rec.Code != 401 {
			t.Fatalf("%s without credentials: %d", p, rec.Code)
		}
		if wa := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(wa, "Basic") {
			t.Fatalf("%s WWW-Authenticate %q", p, wa)
		}
	}
}

func TestCompatBasicAndCookie(t *testing.T) {
	e, h, _ := compatEnv(t, true)
	p := compatPaths[0]

	rec := compatGet(h, p, basicAuth("読者", "reader pass"))
	if rec.Code == 401 {
		t.Fatalf("basic with user name: %d", rec.Code)
	}
	token := compatCookieFrom(rec)
	if token == "" {
		t.Fatal("basic sign-in set no cn_compat cookie")
	}
	if rec := compatGet(h, p, withCompatCookie(token)); rec.Code == 401 {
		t.Fatalf("cookie alone: %d", rec.Code)
	}
	if rec := compatGet(h, p, basicAuth("reader@example.com", "reader pass")); rec.Code == 401 {
		t.Fatalf("basic with email: %d", rec.Code)
	}

	// The compat cookie is not a web session: /api/v1 refuses it.
	if rec := compatGet(e.h, "/api/v1/me", withCompatCookie(token)); rec.Code != 401 {
		t.Fatalf("cn_compat cookie on /api/v1/me: %d", rec.Code)
	}

	// One wrong password is a 401 (a second failure would not reach the limit).
	if rec := compatGet(h, p, basicAuth("読者", "wrong password")); rec.Code != 401 {
		t.Fatalf("wrong password: %d", rec.Code)
	}
}

func TestCompatAPIKeys(t *testing.T) {
	e, h, _ := compatEnv(t, true)
	p := compatPaths[1]
	reader := e.login("読者", "reader pass")
	var created ApiKeyCreated
	reader.json("POST", "/api/v1/me/api-keys", map[string]any{"name": "Mihon"}, 201, &created)

	for name, set := range map[string]func(*http.Request){
		"X-API-Key": header("X-API-Key", created.Key),
		"Bearer":    header("Authorization", "Bearer "+created.Key),
		"Basic key": basicAuth("anything", created.Key),
	} {
		if rec := compatGet(h, p, set); rec.Code == 401 {
			t.Fatalf("%s: %d", name, rec.Code)
		}
	}
	if rec := compatGet(h, p, header("X-API-Key", "cn_forged")); rec.Code != 401 {
		t.Fatalf("forged key: %d", rec.Code)
	}
}

func TestCompatPasswordChangeEndsCache(t *testing.T) {
	e, h, readerID := compatEnv(t, true)
	p := compatPaths[0]
	rec := compatGet(h, p, basicAuth("読者", "reader pass"))
	if rec.Code == 401 {
		t.Fatalf("basic before change: %d", rec.Code)
	}
	token := compatCookieFrom(rec)

	e.admin.json("PATCH", "/api/v1/users/"+readerID, map[string]any{"password": "new password"}, 200, nil)

	if rec := compatGet(h, p, basicAuth("読者", "reader pass")); rec.Code != 401 {
		t.Fatalf("old password after change: %d", rec.Code)
	}
	if rec := compatGet(h, p, withCompatCookie(token)); rec.Code != 401 {
		t.Fatalf("session after change: %d", rec.Code)
	}
	if rec := compatGet(h, p, basicAuth("読者", "new password")); rec.Code == 401 {
		t.Fatalf("new password: %d", rec.Code)
	}
}
