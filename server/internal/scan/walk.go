package scan

import (
	"context"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/archive"
	"github.com/tabunugoku/cominavi/server/internal/natsort"
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
type Listing struct {
	Series     []FoundSeries
	Unreadable []string
}

// Walk lists the series under root. Hidden files and folders (leading '.')
// and the system folders above are skipped, as is anything matching an
// exclude glob (path.Match against the relative path or the base name).
// Symlinks are not followed. Series come back in natural folder order and
// books in natural name order.
func Walk(ctx context.Context, root string, exclude []string) (Listing, error) {
	byDir := map[string]*FoundSeries{}
	var oneshots []FoundSeries
	var unreadable []string

	err :=filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			if p == root {
				return err // the library root itself is unreadable
			}
			// Keep scanning, but remember what could not be read.
			if r, rerr := filepath.Rel(root, p); rerr == nil {
				unreadable = append(unreadable, filepath.ToSlash(r))
			}
			return nil
		}
		if p == root {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if Skip(rel, d.IsDir(), exclude) {
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
			unreadable = append(unreadable, rel)
			return nil
		}
		b := FoundBook{Rel: rel, Format: f, Size: info.Size(), MTime: info.ModTime().UTC()}
		dir := path.Dir(rel)
		if dir == "." {
			oneshots = append(oneshots, FoundSeries{Dir: "", Oneshot: rel, Books: []FoundBook{b}})
			return nil
		}
		s := byDir[dir]
		if s == nil {
			s = &FoundSeries{Dir: dir}
			byDir[dir] = s
		}
		s.Books = append(s.Books, b)
		return nil
	})
	if err != nil {
		return Listing{}, err
	}

	out := make([]FoundSeries, 0, len(byDir)+len(oneshots))
	for _, s := range byDir {
		slices.SortFunc(s.Books, func(a, b FoundBook) int { return cmpNat(a.Rel, b.Rel) })
		out = append(out, *s)
	}
	out = append(out, oneshots...)
	slices.SortFunc(out, func(a, b FoundSeries) int { return cmpNat(a.Dir+a.Oneshot, b.Dir+b.Oneshot) }) // one of the two is always ""
	return Listing{Series: out, Unreadable: unreadable}, nil
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
