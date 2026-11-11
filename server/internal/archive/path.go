package archive

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Path names a book file as a library root plus a path inside it. Opening
// goes through os.Root, so the file is never resolved outside Root, even if
// a folder in between is swapped for a symlink after it was checked.
type Path struct {
	Root string // library root, an OS path
	Rel  string // file inside Root, with '/' separators
}

// PathOf splits a plain file path into its directory and base name.
func PathOf(file string) Path {
	return Path{Root: filepath.Dir(file), Rel: filepath.Base(file)}
}

func (p Path) String() string { return filepath.Join(p.Root, filepath.FromSlash(p.Rel)) }

// Stat describes the file, resolved inside Root.
func (p Path) Stat() (fs.FileInfo, error) {
	r, err := os.OpenRoot(p.Root)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return r.Stat(filepath.FromSlash(p.Rel))
}

// OpenPath opens the book at p, choosing the reader by extension.
func OpenPath(p Path) (Archive, Format, error) {
	f, ok := formatOf(p.Rel)
	if !ok {
		return nil, "", fmt.Errorf("%w: unknown extension on %q", ErrUnsupported, filepath.Base(p.Rel))
	}
	root, err := os.OpenRoot(p.Root)
	if err != nil {
		return nil, f, err
	}
	var a Archive
	switch f {
	case CBR:
		// rardecode reopens the file (and any further volumes) through
		// the root for every pass, so the archive keeps it.
		a, err = openRar(root, p.Rel)
		if err != nil {
			root.Close()
		}
		return a, f, err
	case CBZ:
		a, err = withFile(root, p.Rel, openZip)
	case CB7:
		a, err = withFile(root, p.Rel, openSevenZip)
	}
	root.Close() // an open file outlives its root
	return a, f, err
}

// withFile opens rel inside root as a regular file and hands it to open,
// which owns it from then on (and must close it on error).
func withFile(root *os.Root, rel string, open func(f *os.File, size int64) (Archive, error)) (Archive, error) {
	f, err := root.Open(filepath.FromSlash(rel))
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !st.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("%w: %q is not a regular file", ErrUnsupported, rel)
	}
	return open(f, st.Size())
}
