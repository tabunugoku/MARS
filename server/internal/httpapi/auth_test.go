package httpapi

import (
	"image/color"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupOnlyOnce(t *testing.T) {
	e := newEnv(t)
	anon := e.anon()
	var ls LoginScreen
	anon.json("GET", "/api/v1/public/login-screen", nil, 200, &ls)
	if ls.NeedsSetup {
		t.Fatal("needsSetup after setup")
	}
	rec := anon.do("POST", "/api/v1/setup", map[string]any{"token": "", "name": "x", "email": "x@example.com", "password": "12345678"})
	if rec.Code != 403 {
		t.Fatalf("second setup: %d", rec.Code)
	}
	var me User
	e.admin.json("GET", "/api/v1/me", nil, 200, &me)
	if me.Role != RoleAdmin || me.Name != "管理人" {
		t.Fatalf("me %+v", me)
	}
}

func TestSetupNeedsToken(t *testing.T) {
	e := newEnv(t)
	// A fresh gate on the same database stays closed: a user exists.
	if e.gate.open() {
		t.Fatal("gate open after setup")
	}
	e.gate.token = "right" // as if no user existed yet
	defer func() { e.gate.token = "" }()
	rec := e.anon().do("POST", "/api/v1/setup", map[string]any{"token": "wrong", "name": "x", "email": "x@example.com", "password": "12345678"})
	if rec.Code != 403 {
		t.Fatalf("wrong token: %d", rec.Code)
	}
}

func TestLoginLogout(t *testing.T) {
	e := newEnv(t)
	anon := e.anon()
	anon.json("GET", "/api/v1/me", nil, 401, nil)
	anon.json("GET", "/api/v1/libraries", nil, 401, nil)
	anon.json("GET", "/api/v1/healthz", nil, 200, nil)

	// By email too, case-insensitively.
	c := e.login("ADMIN@example.com", "correct horse")
	c.json("GET", "/api/v1/me", nil, 200, nil)
	rec := c.do("POST", "/api/v1/auth/logout", nil)
	if rec.Code != 204 {
		t.Fatalf("logout %d", rec.Code)
	}
	c.json("GET", "/api/v1/me", nil, 401, nil)
	e.admin.json("GET", "/api/v1/me", nil, 200, nil) // other sessions survive

	rec = anon.do("POST", "/api/v1/auth/login", map[string]any{"username": "管理人", "password": "correct horse", "remember": true})
	if rec.Code != 200 {
		t.Fatalf("remember login %d", rec.Code)
	}
	ck := rec.Result().Cookies()[0]
	if !ck.HttpOnly || ck.MaxAge != 30*24*3600 || ck.SameSite != 2 /* Lax */ {
		t.Fatalf("cookie %+v", ck)
	}
}

func TestLoginThrottle(t *testing.T) {
	e := newEnv(t)
	anon := e.anon()
	for i := range 5 {
		if rec := anon.do("POST", "/api/v1/auth/login", map[string]any{"username": "管理人", "password": "wrong password"}); rec.Code != 401 {
			t.Fatalf("attempt %d: %d", i, rec.Code)
		}
	}
	rec := anon.do("POST", "/api/v1/auth/login", map[string]any{"username": "管理人", "password": "correct horse"})
	if rec.Code != 429 || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("after 5 failures: %d %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	// Unknown users fail the same way.
	if rec := anon.do("POST", "/api/v1/auth/login", map[string]any{"username": "nobody", "password": "x"}); rec.Code != 429 {
		t.Fatalf("unknown user from a blocked address: %d", rec.Code)
	}
}

func TestCSRF(t *testing.T) {
	e := newEnv(t)
	body := map[string]any{"name": "k"}
	if rec := e.admin.do("POST", "/api/v1/me/api-keys", body, "Origin", ""); rec.Code != 403 {
		t.Fatalf("no Origin: %d", rec.Code)
	}
	if rec := e.admin.do("POST", "/api/v1/me/api-keys", body, "Origin", "https://evil.example"); rec.Code != 403 {
		t.Fatalf("foreign Origin: %d", rec.Code)
	}
	if rec := e.admin.do("POST", "/api/v1/me/api-keys", body, "Origin", "", "Sec-Fetch-Site", "same-origin"); rec.Code != 201 {
		t.Fatalf("Sec-Fetch-Site same-origin: %d", rec.Code)
	}
	if rec := e.anon().do("POST", "/api/v1/auth/login", map[string]any{"username": "管理人", "password": "correct horse"}, "Origin", "https://evil.example"); rec.Code != 403 {
		t.Fatalf("cross-site login: %d", rec.Code)
	}
}

func TestAPIKeys(t *testing.T) {
	e := newEnv(t)
	var created ApiKeyCreated
	e.admin.json("POST", "/api/v1/me/api-keys", map[string]any{"name": "OPDS"}, 201, &created)
	if !strings.HasPrefix(created.Key, "cn_") {
		t.Fatalf("key %q", created.Key)
	}
	bot := client{t: t, h: e.h, bearer: created.Key}
	// No Origin needed with a key: browsers cannot attach it cross-site.
	if rec := bot.do("GET", "/api/v1/me", nil); rec.Code != 200 {
		t.Fatalf("bearer: %d", rec.Code)
	}
	if rec := bot.do("POST", "/api/v1/me/api-keys", map[string]any{"name": "x"}, "Origin", ""); rec.Code != 201 {
		t.Fatalf("bearer write: %d", rec.Code)
	}
	var keys []ApiKey
	e.admin.json("GET", "/api/v1/me/api-keys", nil, 200, &keys)
	if len(keys) != 2 {
		t.Fatalf("keys %+v", keys)
	}
	e.admin.json("DELETE", "/api/v1/me/api-keys/"+created.Id, nil, 204, nil)
	bot.json("GET", "/api/v1/me", nil, 401, nil)
	(client{t: t, h: e.h, bearer: "cn_forged"}).json("GET", "/api/v1/me", nil, 401, nil)
}

// TestLibraryScope checks that a user limited to one library cannot reach
// anything in another, even by id.
func TestLibraryScope(t *testing.T) {
	e := newEnv(t)
	red := pngBytes(t, 40, 60, color.RGBA{200, 0, 0, 255})
	rootA, rootB := t.TempDir(), t.TempDir()
	writeCBZ(t, filepath.Join(rootA, "Alpha", "Alpha v01.cbz"), map[string][]byte{"1.png": red})
	writeCBZ(t, filepath.Join(rootB, "Beta", "Beta v01.cbz"), map[string][]byte{"1.png": red})
	var a, b Library
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "A", "root": rootA, "kind": "manga"}, 201, &a)
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "B", "root": rootB, "kind": "manga"}, 201, &b)
	e.waitTasks()

	var u User
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "読者", "email": "reader@example.com", "password": "reader pass", "role": "user",
		"allLibraries": false, "libraryIds": []string{a.Id}}, 201, &u)
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "other", "email": "READER@example.com", "password": "reader pass", "role": "user"}, 400, nil)
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "a@b", "email": "ab@example.com", "password": "reader pass", "role": "user"}, 400, nil)
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "short", "email": "s@example.com", "password": "short", "role": "user"}, 400, nil)
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "ghost", "email": "g@example.com", "password": "reader pass", "role": "user", "libraryIds": []string{"nope"}}, 400, nil)
	r := e.login("読者", "reader pass")

	var libs []Library
	r.json("GET", "/api/v1/libraries", nil, 200, &libs)
	if len(libs) != 1 || libs[0].Id != a.Id || libs[0].Root != "" {
		t.Fatalf("libraries %+v", libs)
	}
	r.json("GET", "/api/v1/libraries/"+b.Id, nil, 404, nil)

	var all []Series
	e.admin.json("GET", "/api/v1/series", nil, 200, &all)
	var mine []Series
	r.json("GET", "/api/v1/series", nil, 200, &mine)
	if len(all) != 2 || len(mine) != 1 || mine[0].LibraryId != a.Id {
		t.Fatalf("series all=%d mine=%+v", len(all), mine)
	}
	r.json("GET", "/api/v1/series?libraryId="+b.Id, nil, 200, &mine)
	if len(mine) != 0 {
		t.Fatalf("series of B by filter: %+v", mine)
	}
	var hidden, shown Series
	for _, s := range all {
		if s.LibraryId == b.Id {
			hidden = s
		} else {
			shown = s
		}
	}
	var hb, sb []Book
	e.admin.json("GET", "/api/v1/series/"+hidden.Id+"/books", nil, 200, &hb)
	e.admin.json("GET", "/api/v1/series/"+shown.Id+"/books", nil, 200, &sb)
	for _, u := range []string{
		"/api/v1/series/" + hidden.Id,
		"/api/v1/series/" + hidden.Id + "/books",
		"/api/v1/series/" + hidden.Id + "/thumbnail",
		"/api/v1/books/" + hb[0].Id,
		"/api/v1/books/" + hb[0].Id + "/pages",
		"/api/v1/books/" + hb[0].Id + "/pages/1",
		"/api/v1/books/" + hb[0].Id + "/thumbnail",
	} {
		if rec := r.do("GET", u, nil); rec.Code != 404 {
			t.Errorf("%s: %d, want 404", u, rec.Code)
		}
		if rec := e.admin.do("GET", u, nil); rec.Code != 200 {
			t.Errorf("admin %s: %d", u, rec.Code)
		}
	}
	if rec := r.do("GET", "/api/v1/books/"+sb[0].Id+"/pages/1", nil); rec.Code != 200 {
		t.Fatalf("own page: %d", rec.Code)
	}

	// Administrator-only operations.
	for _, op := range [][2]string{{"GET", "/api/v1/users"}, {"GET", "/api/v1/tasks"}, {"GET", "/api/v1/media/issues"},
		{"POST", "/api/v1/libraries/" + a.Id + "/scan"}, {"DELETE", "/api/v1/libraries/" + a.Id}, {"DELETE", "/api/v1/users/" + u.Id}} {
		if rec := r.do(op[0], op[1], nil); rec.Code != 403 {
			t.Errorf("%s %s as user: %d, want 403", op[0], op[1], rec.Code)
		}
	}

	// Changes apply to live sessions at once.
	e.admin.json("PATCH", "/api/v1/users/"+u.Id, map[string]any{"canStream": false}, 200, nil)
	if rec := r.do("GET", "/api/v1/books/"+sb[0].Id+"/pages/1", nil); rec.Code != 403 {
		t.Fatalf("page without canStream: %d", rec.Code)
	}
	e.admin.json("PATCH", "/api/v1/users/"+u.Id, map[string]any{"libraryIds": []string{b.Id}}, 200, nil)
	r.json("GET", "/api/v1/series/"+shown.Id, nil, 404, nil)
	e.admin.json("PATCH", "/api/v1/users/"+u.Id, map[string]any{"password": "new password"}, 200, nil)
	r.json("GET", "/api/v1/me", nil, 401, nil)
	e.login("読者", "new password")
}

func TestPasswordAndLastAdmin(t *testing.T) {
	e := newEnv(t)
	var me User
	e.admin.json("GET", "/api/v1/me", nil, 200, &me)
	other := e.login("管理人", "correct horse")

	e.admin.json("PUT", "/api/v1/me/password", map[string]any{"currentPassword": "wrong", "newPassword": "brand new pw"}, 400, nil)
	e.admin.json("PUT", "/api/v1/me/password", map[string]any{"currentPassword": "correct horse", "newPassword": "short"}, 400, nil)
	e.admin.json("PUT", "/api/v1/me/password", map[string]any{"currentPassword": "correct horse", "newPassword": "brand new pw"}, 204, nil)
	e.admin.json("GET", "/api/v1/me", nil, 200, nil) // this session stays
	other.json("GET", "/api/v1/me", nil, 401, nil)   // others end

	e.admin.json("PATCH", "/api/v1/users/"+me.Id, map[string]any{"role": "user"}, 400, nil)
	e.admin.json("DELETE", "/api/v1/users/"+me.Id, nil, 400, nil)
	e.admin.json("PATCH", "/api/v1/users/nope", map[string]any{"role": "user"}, 404, nil)

	var second User
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "二人目", "email": "two@example.com", "password": "second pw!", "role": "admin"}, 201, &second)
	e.admin.json("PATCH", "/api/v1/users/"+me.Id, map[string]any{"role": "user"}, 200, nil)
	e.admin.json("GET", "/api/v1/users", nil, 403, nil) // demoted at once
	two := e.login("二人目", "second pw!")
	two.json("DELETE", "/api/v1/users/"+me.Id, nil, 204, nil)
	e.admin.json("GET", "/api/v1/me", nil, 401, nil)
}
