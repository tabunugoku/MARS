package scan

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tabunugoku/mars/server/internal/archive"
	"github.com/tabunugoku/mars/server/internal/natsort"
)

// FoundBook is a book file seen on disk.
type FoundBook struct {
	Rel    string // path relative to the library root, '/' separated
	Format archive.Format
	Size   int64
	MTime  time.Time
}

// FoundSeries is a folder that directly contains book files, or a single
// book file at the library root (Komga's rule: one folder, one series).
type FoundSeries struct {
	Dir     string // folder relative to the root, '/' separated; "" for root files
	Oneshot string // for a root file: its Rel; "" for folder series
	Books   []FoundBook
}

// Folders that NAS systems and OSes create; never part of a library.
var skipDirs = map[string]bool{
	"@eaDir": true, "#recycle": true, "#snapshot": true, "$RECYCLE.BIN": true,
	"System Volume Information": true, "lost+found": true, "__MACOSX": true,
}

// Skip reports whether Walk leaves out the entry at rel (relative to the
// library root, '/' separated): a hidden name, a system folder (isDir only),
// or a match of an exclude glob (path.Match against the relative path or the
// base name). The file watcher uses it too, so both agree on what is a book.
func Skip(rel string, isDir bool, exclude []string) bool {
	base := path.Base(rel)
	if strings.HasPrefix(base, ".") || (isDir && skipDirs[base]) {
		return true
	}
	for _, pat := range exclude {
		if ok, _ := path.Match(pat, rel); ok {
			return true
		}
		if ok, _ := path.Match(pat, base); ok {
			return true
		}
	}
	return false
}

// Listing is what Walk saw. Unreadable holds paths (relative, '/'
// separated) that could not be read completely: books under them must not
// be treated as deleted, since they may only be out of reach for a moment.
// Scope is nil for a listing of the whole library. For a WalkScope listing it
// holds the cleaned scope entries: Sync leaves rows outside them alone.
type Listing struct {
	Series     []FoundSeries
	Unreadable []string
	Scope      []string
}

// ErrNoScope is what WalkScope returns when no entry of its scope is usable
// (all are invalid or skipped): there is nothing to scan.
var ErrNoScope = errors.New("scan: no usable scope entry")

// InScope reports whether the path rel (relative to the root, '/' separated)
// is inside the scope: an empty scope is the whole library.
func InScope(rel string, scope []string) bool {
	if len(scope) == 0 {
		return true
	}
	for _, e := range scope {
		if under(rel, e) {
			return true
		}
	}
	return false
}

// Walk lists the series under root. Hidden files and folders (leading '.')
// and the system folders above are skipped, as is anything matching an
// exclude glob (path.Match against the relative path or the base name).
// Symlinks are not followed. Series come back in natural folder order and
// books in natural name order.
func Walk(ctx context.Context, root string, exclude []string) (Listing, error) {
	w := newWalker(root, exclude)
	if err := w.walk(ctx, root); err != nil {
		return Listing{}, err
	}
	return w.listing(), nil
}

// WalkScope is Walk limited to scope: each entry is one top-level folder or
// root-level file, named by a single path segment. A folder is walked exactly
// as Walk walks the root; a book file counts as a oneshot series, as a root
// file does in Walk. A missing entry contributes nothing, so Sync trashes its
// rows. Entries that are invalid or skipped are dropped; when none is left
// ErrNoScope is returned. The root must be readable, as for Walk.
func WalkScope(ctx context.Context, root string, exclude, scope []string) (Listing, error) {
	if _, err := os.ReadDir(root); err != nil {
		return Listing{}, err
	}
	var names []string
	for _, e := range scope {
		if validScopeEntry(e) {
			names = append(names, e)
		}
	}
	slices.Sort(names)
	names = slices.Compact(names)

	w := newWalker(root, exclude)
	var clean []string
	for _, e := range names {
		full := filepath.Join(root, filepath.FromSlash(e))
		info, err := os.Lstat(full)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			if !Skip(e, false, exclude) {
				clean = append(clean, e) // gone: its rows are trashed
			}
			continue
		case err != nil:
			w.unreadable = append(w.unreadable, e)
			clean = append(clean, e)
			continue
		}
		if Skip(e, info.IsDir(), exclude) {
			continue
		}
		clean = append(clean, e)
		if err := w.walk(ctx, full); err != nil {
			return Listing{}, err
		}
	}
	if len(clean) == 0 {
		return Listing{}, ErrNoScope
	}
	l := w.listing()
	l.Scope = clean
	return l, nil
}

// validScopeEntry reports whether e is one path segment naming an entry of the
// root: not empty, '.' or '..', and without a separator or a volume name.
func validScopeEntry(e string) bool {
	return e != "" && e != "." && e != ".." && !strings.ContainsAny(e, `/\`) &&
		!filepath.IsAbs(e) && filepath.VolumeName(e) == ""
}

// walker collects what a walk sees, grouped into series.
type walker struct {
	root       string
	exclude    []string
	byDir      map[string]*FoundSeries
	oneshots   []FoundSeries
	unreadable []string
}

func newWalker(root string, exclude []string) *walker {
	return &walker{root: root, exclude: exclude, byDir: map[string]*FoundSeries{}}
}

// walk visits start (the root, a folder under it or a file) with the rules of
// Walk. An unreadable root is an error; any other unreadable path is recorded
// and the walk goes on.
func (w *walker) walk(ctx context.Context, start string) error {
	return filepath.WalkDir(start, func(p string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			if p == w.root {
				return err // the library root itself is unreadable
			}
			// Keep scanning, but remember what could not be read.
			if r, rerr := filepath.Rel(w.root, p); rerr == nil {
				w.unreadable = append(w.unreadable, filepath.ToSlash(r))
			}
			return nil
		}
		if p == w.root {
			return nil
		}
		rel, rerr := filepath.Rel(w.root, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if Skip(rel, d.IsDir(), w.exclude) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		f, ok := archive.FormatOf(d.Name())
		if !ok {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			w.unreadable = append(w.unreadable, rel)
			return nil
		}
		b := FoundBook{Rel: rel, Format: f, Size: info.Size(), MTime: info.ModTime().UTC()}
		dir := path.Dir(rel)
		if dir == "." {
			w.oneshots = append(w.oneshots, FoundSeries{Dir: "", Oneshot: rel, Books: []FoundBook{b}})
			return nil
		}
		s := w.byDir[dir]
		if s == nil {
			s = &FoundSeries{Dir: dir}
			w.byDir[dir] = s
		}
		s.Books = append(s.Books, b)
		return nil
	})
}

// listing returns what the walks so far found, in sort order.
func (w *walker) listing() Listing {
	out := make([]FoundSeries, 0, len(w.byDir)+len(w.oneshots))
	for _, s := range w.byDir {
		slices.SortFunc(s.Books, func(a, b FoundBook) int { return cmpNat(a.Rel, b.Rel) })
		out = append(out, *s)
	}
	out = append(out, w.oneshots...)
	slices.SortFunc(out, func(a, b FoundSeries) int { return cmpNat(a.Dir+a.Oneshot, b.Dir+b.Oneshot) }) // one of the two is always ""
	return Listing{Series: out, Unreadable: w.unreadable}
}

// under reports whether rel is folder p or inside it ("" or "." is the root).
func under(rel, p string) bool {
	return p == "" || p == "." || rel == p || strings.HasPrefix(rel, p+"/")
}

func cmpNat(a, b string) int {
	switch {
	case natsort.Less(a, b):
		return -1
	case natsort.Less(b, a):
		return 1
	}
	return 0
}
