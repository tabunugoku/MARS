package archive

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// A book file may hold other archives, one per volume (work.zip holding
// vol1.zip and vol2.zip). Read as one book, the pages of every volume follow
// the outer entries, each named "volume/entry". A natural sort of the full
// names then gives volume order and page order.
//
// Only one level is read: an archive inside a volume is an ordinary entry.
// A volume is spilled to a temp file before it is read, because the readers
// need a seekable file and RAR and 7z streams can only be read forward.

var (
	tempMu  sync.Mutex
	tempDir string
)

// SetTempDir sets the directory that volumes are spilled to. An empty dir
// means os.TempDir.
func SetTempDir(dir string) {
	tempMu.Lock()
	defer tempMu.Unlock()
	tempDir = dir
}

func spillDir() string {
	tempMu.Lock()
	defer tempMu.Unlock()
	if tempDir == "" {
		return os.TempDir()
	}
	return tempDir
}

const (
	// maxInnerBytes caps one spilled volume. It matches media.MaxExtractedBook,
	// since the pages of a volume are extracted to the page cache in full.
	maxInnerBytes = 2 << 30
	// maxSpillTotal caps all spills of one wrapper. It matches
	// media.MaxDecompressed, the work one pass over a book may do.
	maxSpillTotal = 4 << 30
)

// isInnerArchive reports whether an outer entry is a volume that is read as an
// archive. PDF is never one, and neither is any other extension.
func isInnerArchive(name string) bool {
	f, ok := formatOf(name)
	return ok && (f == CBZ || f == CBR || f == CB7)
}

// spilled is one volume after its first use. A parse failure is kept, so each
// later use fails the same way without parsing the volume again.
type spilled struct {
	path string  // temp file, removed by Close; empty once removed
	arc  Archive // nil when err is set
	err  error
}

// nestedArchive reads the volumes of an outer archive as one book. It is
// returned by OpenPathContext only when the outer archive holds a volume.
type nestedArchive struct {
	outer  Archive
	ctx    context.Context // stored at open; bounds spilling and listing
	order  []string        // outer entry names of the volumes, in outer order
	volume map[string]bool // the same names, as a set

	mu    sync.Mutex // guards inner and spilt
	inner map[string]*spilled
	spilt int64 // bytes spilled so far, against maxSpillTotal

	listMu  sync.Mutex // guards the listing below
	listed  bool
	entries []Entry
	listErr error
}

// newNestedArchive wraps a. ok is false when a holds no volume, in which case
// the caller keeps a as it is.
func newNestedArchive(ctx context.Context, a Archive) (w *nestedArchive, ok bool) {
	w = &nestedArchive{
		outer:  a,
		ctx:    ctx,
		volume: make(map[string]bool),
		inner:  make(map[string]*spilled),
	}
	for _, e := range a.Entries() {
		if isInnerArchive(e.Name) {
			w.order = append(w.order, e.Name)
			w.volume[e.Name] = true
		}
	}
	return w, len(w.order) > 0
}

// load returns volume name, spilling r to a temp file on first use. A parse
// failure is cached. A failed copy is not: it may be the caller's budget that
// ran out, which says nothing about the volume, so a later walk may succeed.
func (w *nestedArchive) load(name string, r io.Reader) (*spilled, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if sp, ok := w.inner[name]; ok {
		return sp, sp.err
	}
	left := min(maxInnerBytes, maxSpillTotal-w.spilt)
	if left <= 0 {
		return nil, ErrTooMuchData
	}

	dir := spillDir()
	// The extension is what formatOf reads when the temp file is opened, so the
	// same reader is picked. It is one of the volume extensions, never a
	// pattern character.
	f, err := os.CreateTemp(dir, "nested-*"+strings.ToLower(filepath.Ext(name)))
	if err != nil {
		return nil, err
	}
	// The budget reader stops the copy at left bytes and checks w.ctx on every
	// read, so a cancelled request does not sit through a 2 GB copy.
	n, err := io.Copy(f, newBudget(Limit{Ctx: w.ctx, MaxBytes: left}).reader(r))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return nil, err
	}
	w.spilt += n

	// The plain opener, never the nested one: a volume inside a volume is an
	// ordinary entry.
	a, _, err := openPlain(w.ctx, Path{Root: dir, Rel: filepath.Base(f.Name())})
	if err != nil {
		os.Remove(f.Name())
		sp := &spilled{err: headerErr(err)}
		w.inner[name] = sp
		return nil, sp.err
	}
	sp := &spilled{path: f.Name(), arc: a}
	w.inner[name] = sp
	return sp, nil
}

// openVolume returns volume vol, reading it from the outer archive when it is
// not spilled yet.
func (w *nestedArchive) openVolume(vol string) (*spilled, error) {
	w.mu.Lock()
	sp, ok := w.inner[vol]
	w.mu.Unlock()
	if ok {
		if sp.err != nil {
			return nil, sp.err
		}
		return sp, nil
	}
	rc, err := w.outer.Open(vol)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return w.load(vol, rc)
}

// walkLimited walks the outer entries, and each volume's entries in place of
// the volume, renamed "volume/entry". A SkipRest from fn stops both the volume
// and the outer archive, and walkLimited returns nil.
func (w *nestedArchive) walkLimited(b *budget, fn func(e Entry, r io.Reader) error) error {
	return walkArchive(w.outer, b, func(e Entry, r io.Reader) error {
		if !w.volume[e.Name] {
			return fn(e, r)
		}
		// r is already counted against b by the outer walk, so it is spilled
		// as it is; counting it again would charge the budget twice.
		sp, err := w.load(e.Name, r)
		if err != nil {
			return err
		}
		skip := false
		err = walkArchive(sp.arc, b, func(ie Entry, ir io.Reader) error {
			ie.Name = e.Name + "/" + ie.Name
			err := fn(ie, ir)
			if errors.Is(err, SkipRest) {
				skip = true
			}
			return err
		})
		if err != nil {
			return err
		}
		if skip {
			return SkipRest
		}
		return nil
	})
}

// walkArchive is walkLimited where the archive supports it. Plain archives
// always do, and the outer archive is never a PDF here.
func walkArchive(a Archive, b *budget, fn func(e Entry, r io.Reader) error) error {
	if l, ok := a.(limitedWalker); ok {
		return l.walkLimited(b, fn)
	}
	return a.Walk(fn)
}

// Walk reads every entry once. A volume is read after the outer entries that
// come before it in the outer archive.
func (w *nestedArchive) Walk(fn func(e Entry, r io.Reader) error) error {
	return w.walkLimited(newBudget(Limit{}), fn)
}

// Entries lists the outer entries that are not volumes, then the entries of
// each volume. On a listing error it returns only the first group.
func (w *nestedArchive) Entries() []Entry {
	es, _ := w.listing()
	return es
}

// EntriesContext is Entries that also returns the error. It gives up with
// ctx's error when ctx has already ended.
func (w *nestedArchive) EntriesContext(ctx context.Context) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return w.listing()
}

// listing builds the entry list once and keeps it. A listing that failed
// because the stored context ended is not kept, since a later call may succeed.
func (w *nestedArchive) listing() ([]Entry, error) {
	w.listMu.Lock()
	defer w.listMu.Unlock()
	if w.listed {
		return w.entries, w.listErr
	}
	entries, err := w.list()
	if err != nil && w.ctx.Err() != nil {
		return entries, err
	}
	w.entries, w.listErr, w.listed = entries, err, true
	return entries, err
}

func (w *nestedArchive) list() ([]Entry, error) {
	var plain []Entry
	for _, e := range w.outer.Entries() {
		if !w.volume[e.Name] {
			plain = append(plain, e)
		}
	}
	// Each volume is spilled so its entry list can be read, but its pages are
	// not decompressed. The outer walk is the only pass over the book, and the
	// budget bounds the spilling and the solid-archive skipping.
	var nested []Entry
	err := walkArchive(w.outer, newBudget(Limit{Ctx: w.ctx, MaxBytes: maxSpillTotal}), func(e Entry, r io.Reader) error {
		if !w.volume[e.Name] {
			return nil
		}
		sp, err := w.load(e.Name, r)
		if err != nil {
			return err
		}
		for _, ie := range sp.arc.Entries() {
			ie.Name = e.Name + "/" + ie.Name
			nested = append(nested, ie)
		}
		return nil
	})
	if err != nil {
		return plain, err
	}
	return append(plain, nested...), nil
}

// volumeOf splits name into a volume and the entry inside it. The longest
// matching volume name wins.
func (w *nestedArchive) volumeOf(name string) (vol, rest string, ok bool) {
	for _, v := range w.order {
		if strings.HasPrefix(name, v+"/") && len(v) > len(vol) {
			vol, rest, ok = v, name[len(v)+1:], true
		}
	}
	return vol, rest, ok
}

// Open returns an entry by its name, which for a volume's entry is
// "volume/entry". Other names are read from the outer archive.
func (w *nestedArchive) Open(name string) (io.ReadCloser, error) {
	if vol, rest, ok := w.volumeOf(name); ok {
		sp, err := w.openVolume(vol)
		if err != nil {
			return nil, err
		}
		return sp.arc.Open(rest)
	}
	return w.outer.Open(name)
}

// RandomAccess is false: a volume is spilled, and reading it is not a seek
// into the book file.
func (w *nestedArchive) RandomAccess() bool { return false }

// Close closes the volumes, removes their temp files and closes the outer
// archive. It returns the first error.
func (w *nestedArchive) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	var first error
	keep := func(err error) {
		if first == nil && err != nil {
			first = err
		}
	}
	for _, sp := range w.inner {
		// A volume is closed before its file is removed, which Windows requires.
		if sp.arc != nil {
			keep(sp.arc.Close())
		}
		if sp.path != "" {
			if err := os.Remove(sp.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				keep(err)
			}
		}
	}
	keep(w.outer.Close())
	return first
}
