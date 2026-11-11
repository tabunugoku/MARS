package httpapi

import (
	"image/color"
	"path/filepath"
	"testing"
)

func TestUpdateMe(t *testing.T) {
	e := newEnv(t)
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "読者", "email": "reader@example.com", "password": "reader pass", "role": "user"}, 201, nil)
	r := e.login("読者", "reader pass")
	var u User
	r.json("PATCH", "/api/v1/me", map[string]any{"name": "  新しい名前 "}, 200, &u)
	if u.Name != "新しい名前" {
		t.Fatalf("name %q", u.Name)
	}
	r.json("PATCH", "/api/v1/me", map[string]any{"name": " "}, 400, nil)
	r.json("GET", "/api/v1/me", nil, 200, &u)
	if u.Name != "新しい名前" || u.Role != RoleUser {
		t.Fatalf("me %+v", u)
	}
}

func TestMarkLibraryReadAndAnalyze(t *testing.T) {
	e := newEnv(t)
	red := pngBytes(t, 40, 60, color.RGBA{200, 0, 0, 255})
	rootA, rootB := t.TempDir(), t.TempDir()
	writeCBZ(t, filepath.Join(rootA, "Alpha", "Alpha v01.cbz"), map[string][]byte{"1.png": red})
	writeCBZ(t, filepath.Join(rootA, "Alpha", "Alpha v02.cbz"), map[string][]byte{"1.png": red})
	writeCBZ(t, filepath.Join(rootB, "Beta", "Beta v01.cbz"), map[string][]byte{"1.png": red})
	var a, b Library
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "A", "root": rootA, "kind": "manga"}, 201, &a)
	e.admin.json("POST", "/api/v1/libraries", map[string]any{"name": "B", "root": rootB, "kind": "manga"}, 201, &b)
	e.waitTasks()
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "読者", "email": "reader@example.com", "password": "reader pass", "role": "user",
		"allLibraries": false, "libraryIds": []string{a.Id}}, 201, nil)
	r := e.login("読者", "reader pass")

	r.json("PUT", "/api/v1/libraries/"+a.Id+"/read-progress", map[string]any{"completed": true}, 204, nil)
	r.json("PUT", "/api/v1/libraries/"+b.Id+"/read-progress", map[string]any{"completed": true}, 404, nil)
	var mine, admins []Series
	r.json("GET", "/api/v1/series", nil, 200, &mine)
	if len(mine) != 1 || mine[0].BooksRead != 2 {
		t.Fatalf("after marking read %+v", mine)
	}
	e.admin.json("GET", "/api/v1/series?libraryId="+a.Id, nil, 200, &admins)
	if admins[0].BooksRead != 0 {
		t.Fatalf("admin progress changed: %+v", admins[0])
	}
	r.json("PUT", "/api/v1/libraries/"+a.Id+"/read-progress", map[string]any{"completed": false}, 204, nil)
	r.json("GET", "/api/v1/series", nil, 200, &mine)
	if mine[0].BooksRead != 0 {
		t.Fatalf("after marking unread %+v", mine)
	}

	var books []Book
	e.admin.json("GET", "/api/v1/series/"+mine[0].Id+"/books", nil, 200, &books)
	r.json("POST", "/api/v1/books/"+books[0].Id+"/analyze", nil, 403, nil)
	e.admin.json("POST", "/api/v1/books/nope/analyze", nil, 404, nil)
	var task Task
	e.admin.json("POST", "/api/v1/books/"+books[0].Id+"/analyze", nil, 202, &task)
	e.waitTasks()
	var again Book
	e.admin.json("GET", "/api/v1/books/"+books[0].Id, nil, 200, &again)
	if again.MediaStatus != MediaStatusReady {
		t.Fatalf("after analyze: %s", again.MediaStatus)
	}
}
