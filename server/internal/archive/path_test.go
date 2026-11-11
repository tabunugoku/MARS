package archive

import (
	"os"
	"path/filepath"
	"testing"
)

// A folder inside the library that is (or becomes) a symlink to somewhere
// outside must not let a book be opened there. Links that stay inside work.
func TestOpenPathStaysInRoot(t *testing.T) {
	base := t.TempDir()
	root, outside := filepath.Join(base, "lib"), filepath.Join(base, "outside")
	for _, d := range []string{filepath.Join(root, "real"), outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	src := zipFixture(t)
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(outside, "book.cbz"), filepath.Join(root, "real", "book.cbz")} {
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}
	// Relative: os.Root treats an absolute link target as leaving the root.
	// The scan never follows links, so stored book paths do not use them.
	if err := os.Symlink("real", filepath.Join(root, "inside")); err != nil {
		t.Fatal(err)
	}

	if a, _, err := OpenPath(Path{Root: root, Rel: "escape/book.cbz"}); err == nil {
		a.Close()
		t.Fatal("opened a book through a symlink that leaves the root")
	}
	if _, err := (Path{Root: root, Rel: "escape/book.cbz"}).Stat(); err == nil {
		t.Fatal("stat through a symlink that leaves the root")
	}
	a, _, err := OpenPath(Path{Root: root, Rel: "inside/book.cbz"})
	if err != nil {
		t.Fatalf("symlink inside the root: %v", err)
	}
	a.Close()
}
