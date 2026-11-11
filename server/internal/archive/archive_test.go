package archive

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// fixture describes one readable book. files is the expected Entries order
// (directories excluded); content maps each name to its bytes.
type fixture struct {
	name    string
	path    string
	format  Format
	files   []string
	content map[string]string
	random  bool
}

// zipContent holds the page bytes shared by the CBZ and CBR fixtures.
var zipContent = map[string]string{
	"p2.png":        "img-p2",
	"p10.png":       "img-p10",
	"p1.png":        "img-p1",
	"ComicInfo.xml": "<ComicInfo/>",
	"sub/p3.png":    "img-p3",
}

// pageOrder is the archive order used by the CBZ and CBR fixtures.
var pageOrder = []string{"p2.png", "p10.png", "p1.png", "ComicInfo.xml", "sub/p3.png"}

func zipFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "book.cbz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, name := range []string{"p2.png", "p10.png", "p1.png", "ComicInfo.xml"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, zipContent[name]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := zw.Create("sub/"); err != nil { // directory entry
		t.Fatal(err)
	}
	w, err := zw.Create("sub/p3.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, zipContent["sub/p3.png"]); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func rarFixture(t *testing.T) string {
	t.Helper()
	return writeRar4(t, []rarFixtureEntry{
		{name: "p2.png", data: []byte(zipContent["p2.png"])},
		{name: "p10.png", data: []byte(zipContent["p10.png"])},
		{name: "p1.png", data: []byte(zipContent["p1.png"])},
		{name: "ComicInfo.xml", data: []byte(zipContent["ComicInfo.xml"])},
		{name: "sub", dir: true},
		{name: "sub\\p3.png", data: []byte(zipContent["sub/p3.png"])},
	})
}

func fixtures(t *testing.T) []fixture {
	t.Helper()
	cb7Content := map[string]string{"a.txt": "alpha", "b/c.txt": "charlie"}
	return []fixture{
		{
			name:    "cbz",
			path:    zipFixture(t),
			format:  CBZ,
			files:   pageOrder,
			content: zipContent,
			random:  true,
		},
		{
			name:    "cbr",
			path:    rarFixture(t),
			format:  CBR,
			files:   pageOrder,
			content: zipContent,
		},
		{
			name:    "cb7-solid",
			path:    filepath.Join("testdata", "solid.cb7"),
			format:  CB7,
			files:   []string{"a.txt", "b/c.txt"},
			content: cb7Content,
		},
		{
			name:    "cb7-plain",
			path:    filepath.Join("testdata", "plain.cb7"),
			format:  CB7,
			files:   []string{"a.txt", "b/c.txt"},
			content: cb7Content,
		},
	}
}

func TestFormatOf(t *testing.T) {
	cases := map[string]struct {
		want Format
		ok   bool
	}{
		"x.cbz": {CBZ, true}, "X.ZIP": {CBZ, true},
		"x.cbr": {CBR, true}, "x.RAR": {CBR, true},
		"x.cb7": {CB7, true}, "x.7z": {CB7, true},
		"x.txt": {"", false}, "x": {"", false},
	}
	for name, c := range cases {
		got, ok := FormatOf(name)
		if got != c.want || ok != c.ok {
			t.Errorf("FormatOf(%q) = %q, %v; want %q, %v", name, got, ok, c.want, c.ok)
		}
	}
}

func TestEntries(t *testing.T) {
	for _, fx := range fixtures(t) {
		t.Run(fx.name, func(t *testing.T) {
			a, format, err := Open(fx.path)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if format != fx.format {
				t.Errorf("format = %q, want %q", format, fx.format)
			}
			var names []string
			for _, e := range a.Entries() {
				names = append(names, e.Name)
				if want := int64(len(fx.content[e.Name])); e.Size != want {
					t.Errorf("entry %q size = %d, want %d", e.Name, e.Size, want)
				}
			}
			if !reflect.DeepEqual(names, fx.files) {
				t.Errorf("entries = %v, want %v", names, fx.files)
			}
		})
	}
}

func TestOpen(t *testing.T) {
	for _, fx := range fixtures(t) {
		t.Run(fx.name, func(t *testing.T) {
			a, _, err := Open(fx.path)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()

			for _, name := range fx.files {
				rc, err := a.Open(name)
				if err != nil {
					t.Fatalf("Open(%q): %v", name, err)
				}
				got, err := io.ReadAll(rc)
				rc.Close()
				if err != nil {
					t.Fatalf("read %q: %v", name, err)
				}
				if string(got) != fx.content[name] {
					t.Errorf("Open(%q) = %q, want %q", name, got, fx.content[name])
				}
			}

			if _, err := a.Open("missing.png"); !errors.Is(err, ErrNotExist) {
				t.Errorf("Open(missing) err = %v, want ErrNotExist", err)
			}
			if _, err := a.Open("sub"); !errors.Is(err, ErrNotExist) {
				t.Errorf("Open(directory) err = %v, want ErrNotExist", err)
			}
		})
	}
}

func TestWalk(t *testing.T) {
	for _, fx := range fixtures(t) {
		t.Run(fx.name, func(t *testing.T) {
			a, _, err := Open(fx.path)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()

			var names, bodies []string
			err = a.Walk(func(e Entry, r io.Reader) error {
				b, err := io.ReadAll(r)
				if err != nil {
					return err
				}
				names = append(names, e.Name)
				bodies = append(bodies, string(b))
				return nil
			})
			if err != nil {
				t.Fatalf("Walk: %v", err)
			}
			if !reflect.DeepEqual(names, fx.files) {
				t.Errorf("walked names = %v, want %v", names, fx.files)
			}
			for i, name := range names {
				if bodies[i] != fx.content[name] {
					t.Errorf("walk %q = %q, want %q", name, bodies[i], fx.content[name])
				}
			}
		})
	}
}

func TestWalkSkipRest(t *testing.T) {
	for _, fx := range fixtures(t) {
		t.Run(fx.name, func(t *testing.T) {
			a, _, err := Open(fx.path)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()

			calls := 0
			err = a.Walk(func(e Entry, r io.Reader) error {
				calls++
				return SkipRest
			})
			if err != nil {
				t.Errorf("Walk with SkipRest returned %v, want nil", err)
			}
			if calls != 1 {
				t.Errorf("fn called %d times, want 1", calls)
			}
		})
	}
}

func TestWalkPropagatesError(t *testing.T) {
	fx := fixtures(t)[0]
	a, _, err := Open(fx.path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	boom := errors.New("boom")
	if err := a.Walk(func(Entry, io.Reader) error { return boom }); !errors.Is(err, boom) {
		t.Errorf("Walk err = %v, want boom", err)
	}
}

func TestRandomAccess(t *testing.T) {
	for _, fx := range fixtures(t) {
		t.Run(fx.name, func(t *testing.T) {
			a, _, err := Open(fx.path)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if got := a.RandomAccess(); got != fx.random {
				t.Errorf("RandomAccess() = %v, want %v", got, fx.random)
			}
		})
	}
}

func TestOpenUnsupported(t *testing.T) {
	dir := t.TempDir()

	txt := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txt, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Open(txt); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Open(.txt) err = %v, want ErrUnsupported", err)
	}

	garbage := filepath.Join(dir, "garbage.cbz")
	if err := os.WriteFile(garbage, []byte("this is not a zip file at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Open(garbage); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Open(garbage .cbz) err = %v, want ErrUnsupported", err)
	}

	missing := filepath.Join(dir, "missing.cbz")
	if _, _, err := Open(missing); errors.Is(err, ErrUnsupported) || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open(missing file) err = %v, want fs.ErrNotExist and not ErrUnsupported", err)
	}
}

func TestZipEncryptedEntry(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "locked.png", Method: zip.Store, Flags: 0x1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "secret"); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "locked.cbz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	a, _, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err := a.Open("locked.png"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Open(encrypted) err = %v, want ErrUnsupported", err)
	}
	if err := a.Walk(func(Entry, io.Reader) error { return nil }); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Walk(encrypted) err = %v, want ErrUnsupported", err)
	}
}
