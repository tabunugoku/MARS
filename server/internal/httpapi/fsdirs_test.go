package httpapi

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListDirectories(t *testing.T) {
	e := newEnv(t)
	e.admin.json("POST", "/api/v1/users", map[string]any{"name": "読者", "email": "reader@example.com", "password": "reader pass", "role": "user"}, 201, nil)
	r := e.login("読者", "reader pass")

	root := t.TempDir()
	for _, d := range []string{"b", "a10", "a2", ".hidden", "b/inner", "empty"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "x.cbz"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	list := func(p string) string {
		return "/api/v1/fs/directories?path=" + url.QueryEscape(p)
	}

	var got DirectoryListing
	e.admin.json("GET", list(root), nil, 200, &got)
	if got.Path != filepath.Clean(root) {
		t.Fatalf("path %q, want %q", got.Path, filepath.Clean(root))
	}
	if got.Parent == nil || *got.Parent != filepath.Dir(filepath.Clean(root)) {
		t.Fatalf("parent %v", got.Parent)
	}
	var names []string
	for _, d := range got.Directories {
		names = append(names, d.Name)
		if !d.Readable || d.Path != filepath.Join(root, d.Name) {
			t.Fatalf("entry %+v", d)
		}
	}
	if want := "a2,a10,b,empty"; strings.Join(names, ",") != want {
		t.Fatalf("names %v, want %s", names, want)
	}

	var nested DirectoryListing
	e.admin.json("GET", list(filepath.Join(root, "b")), nil, 200, &nested)
	if len(nested.Directories) != 1 || nested.Directories[0].Name != "inner" {
		t.Fatalf("nested %+v", nested.Directories)
	}
	if nested.Parent == nil || *nested.Parent != filepath.Clean(root) {
		t.Fatalf("nested parent %v", nested.Parent)
	}

	// An empty folder returns a JSON array, not null.
	rec := e.admin.do("GET", list(filepath.Join(root, "empty")), nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"directories":[]`) {
		t.Fatalf("empty: %d %s", rec.Code, rec.Body)
	}

	// A symlink to a folder counts as a folder.
	if err := os.Symlink(filepath.Join(root, "a2"), filepath.Join(root, "link")); err == nil {
		var withLink DirectoryListing
		e.admin.json("GET", list(root), nil, 200, &withLink)
		found := false
		for _, d := range withLink.Directories {
			found = found || d.Name == "link"
		}
		if !found {
			t.Fatalf("symlink to a folder missing: %+v", withLink.Directories)
		}
	}

	e.admin.json("GET", list("relative/path"), nil, 400, nil)
	e.admin.json("GET", list(filepath.Join(root, "missing")), nil, 404, nil)
	e.admin.json("GET", list(filepath.Join(root, "x.cbz")), nil, 404, nil)

	var top DirectoryListing
	e.admin.json("GET", "/api/v1/fs/directories", nil, 200, &top)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.VolumeName(wd) + string(filepath.Separator); top.Path != want {
		t.Fatalf("root path %q, want %q", top.Path, want)
	}
	if top.Parent != nil {
		t.Fatalf("root parent %v, want null", *top.Parent)
	}

	r.json("GET", list(root), nil, 403, nil)
}
