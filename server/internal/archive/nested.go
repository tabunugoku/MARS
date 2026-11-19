package archive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	// maxSpillDisk caps the spilled bytes of all books together. It is two
	// wrappers' worth of maxSpillTotal: a few large books spill at once, and the
	// temp dir stays bounded however many books are open.
	maxSpillDisk = 2 * maxSpillTotal
	// maxKeptEntry and maxKeptTotal bound the small plain outer entries that
	// list keeps in memory (a root ComicInfo.xml, a cover). A walk after the
	// listing then need not decompress a solid RAR or 7z archive a second time.
	maxKeptEntry = 1 << 20
	maxKeptTotal = 8 << 20
)

// The spilled bytes of all books together. Each wrapper charges what it spills
// and gives it back in Close. diskWake is closed and replaced on every release,
// so a waiter wakes and checks the budget again.
var (
	diskMu   sync.Mutex
	diskUsed int64
	diskWake = make(chan struct{})
)

// reserveDisk charges n bytes of spill to the budget before a copy. It waits
// while the spills would pass maxSpillDisk, unless nothing is spilled at all,
// so one volume larger than the free space still goes ahead when the disk is
// otherwise idle. It returns ctx's error if ctx ends first.
//
// A budget, not a count of books, is what keeps the disk bounded here: many
// ordinary books, with volumes of tens or hundreds of MB, proceed at once, and
// nothing is reserved for a book that never spills.
func reserveDisk(ctx context.Context, n int64) error {
	for {
		diskMu.Lock()
		if diskUsed == 0 || diskUsed+n <= maxSpillDisk {
			diskUsed += n
			diskMu.Unlock()
			return nil
		}
		wake := diskWake
		diskMu.Unlock()
		select {
		case <-wake:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// releaseDisk gives n bytes back to the budget and wakes the waiters.
func releaseDisk(n int64) { settleDisk(n, 0) }

// settleDisk replaces a reservation of reserved bytes with kept bytes. It never
// waits: a copy that ran past its declared size has to be charged anyway.
func settleDisk(reserved, kept int64) {
	diskMu.Lock()
	defer diskMu.Unlock()
	diskUsed += kept - reserved
	if kept < reserved {
		close(diskWake)
		diskWake = make(chan struct{})
	}
}

// diskBytes returns the bytes charged to the budget now. Tests use it to check
// that a wrapper gives back everything it took.
func diskBytes() int64 {
	diskMu.Lock()
	defer diskMu.Unlock()
	return diskUsed
}

// IgnoredEntry reports whether an entry is never a volume or a page: a macOS
// resource fork under __MACOSX/, or a dot-file such as ._vol1.zip. A directory
// is judged by its own name. media.isIgnored uses the same rule, so the book
// and its listing agree on which entries exist.
func IgnoredEntry(name string) bool {
	name = strings.TrimSuffix(name, "/")
	for _, seg := range strings.Split(name, "/") {
		if seg == "__MACOSX" {
			return true
		}
	}
	return strings.HasPrefix(name[strings.LastIndex(name, "/")+1:], ".")
}

// isVolumeComicInfo reports whether an entry inside a volume is its
// ComicInfo.xml. That file describes one volume (its title, number and page
// count), not the book that spans them, so it is left out of the nested listing
// and walk. The book's own ComicInfo.xml is still read from the outer archive.
func isVolumeComicInfo(name string) bool {
	return strings.EqualFold(name[strings.LastIndex(name, "/")+1:], "comicinfo.xml")
}

// isInnerArchive reports whether an outer entry is a volume that is read as an
// archive. PDF is never one, neither is any other extension, and an ignored
// entry such as a macOS resource fork is an ordinary outer entry.
func isInnerArchive(name string) bool {
	if IgnoredEntry(name) {
		return false
	}
	f, ok := formatOf(name)
	return ok && (f == CBZ || f == CBR || f == CB7)
}

// errBadVolume marks a volume that spilled but does not parse as an archive. A
// listing or walk skips such a volume, so a damaged or encrypted volume costs
// its own pages only. The cause stays in the chain for the book's error when no
// volume can be read at all.
var errBadVolume = errors.New("archive: volume does not parse")

// spilled is one volume after its first use. A parse failure is kept, so each
// later use fails the same way without parsing the volume again.
type spilled struct {
	path string  // temp file, removed by Close; empty once removed
	arc  Archive // nil when err is set
	err  error   // wraps errBadVolume when the volume does not parse
}

// nestedArchive reads the volumes of an outer archive as one book. It is
// returned by OpenPathContext only when the outer archive holds a volume.
type nestedArchive struct {
	outer  Archive
	ctx    context.Context  // stored at open; bounds spilling and listing
	order  []string         // outer entry names of the volumes, in outer order
	volume map[string]bool  // the same names, as a set
	size   map[string]int64 // declared size of each volume, 0 when unknown

	mu      sync.Mutex // guards inner, spilt, charged and kept
	inner   map[string]*spilled
	spilt   int64             // bytes spilled so far, against maxSpillTotal
	charged int64             // disk bytes this wrapper holds in the budget
	kept    map[string][]byte // small plain outer entries, set by list

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
		size:   make(map[string]int64),
		inner:  make(map[string]*spilled),
	}
	for _, e := range a.Entries() {
		if isInnerArchive(e.Name) {
			w.order = append(w.order, e.Name)
			w.volume[e.Name] = true
			w.size[e.Name] = e.Size
		}
	}
	return w, len(w.order) > 0
}

// load returns volume name, spilling r to a temp file on first use. size is the
// volume's declared size, or 0 when unknown. A parse failure is cached. A failed
// copy or a parse cut short by the context is not: it may be the caller's budget
// or context that ran out, which says nothing about the volume, so a later use
// may succeed.
func (w *nestedArchive) load(name string, size int64, r io.Reader) (*spilled, error) {
	w.mu.Lock()
	if sp, ok := w.inner[name]; ok {
		w.mu.Unlock()
		return sp, sp.err
	}
	left := min(maxInnerBytes, maxSpillTotal-w.spilt)
	w.mu.Unlock()
	if left <= 0 {
		return nil, ErrTooMuchData
	}

	// The reservation is the declared size when known, so a small volume
	// charges little, and the cap otherwise. It waits without w.mu held, since
	// Close and other volumes need that lock while this one waits.
	reserve := left
	if size > 0 && size < reserve {
		reserve = size
	}
	if err := reserveDisk(w.ctx, reserve); err != nil {
		return nil, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if sp, ok := w.inner[name]; ok {
		// Another goroutine spilled this volume while we waited.
		releaseDisk(reserve)
		return sp, sp.err
	}
	left = min(maxInnerBytes, maxSpillTotal-w.spilt)
	if left <= 0 {
		releaseDisk(reserve)
		return nil, ErrTooMuchData
	}

	dir := spillDir()
	// The extension is what formatOf reads when the temp file is opened, so the
	// same reader is picked. It is one of the volume extensions, never a
	// pattern character.
	f, err := os.CreateTemp(dir, "nested-*"+strings.ToLower(filepath.Ext(name)))
	if err != nil {
		releaseDisk(reserve)
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
		releaseDisk(reserve)
		return nil, err
	}
	w.spilt += n
	// The bytes written stay charged. The rest of the reservation goes back now,
	// so the budget reflects what is on disk rather than what was hoped for.
	settleDisk(reserve, n)

	// The plain opener, never the nested one: a volume inside a volume is an
	// ordinary entry.
	a, _, err := openPlain(w.ctx, Path{Root: dir, Rel: filepath.Base(f.Name())})
	if err != nil {
		os.Remove(f.Name())
		releaseDisk(n)
		if w.ctx.Err() != nil {
			// The context cut the parse short, so it says nothing about the volume.
			return nil, w.ctx.Err()
		}
		sp := &spilled{err: fmt.Errorf("%w: %w", errBadVolume, headerErr(err))}
		w.inner[name] = sp
		return nil, sp.err
	}
	w.charged += n
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
	return w.load(vol, w.size[vol], rc)
}

// walkStep is one step of a walk over the spills: a volume to walk, or a small
// plain outer entry that list kept (sp is nil and data holds its bytes).
type walkStep struct {
	e    Entry
	sp   *spilled
	data []byte
}

// walkLimited walks the outer entries, and each volume's entries in place of
// the volume, renamed "volume/entry". A SkipRest from fn stops both the volume
// and the outer archive, and walkLimited returns nil. A volume that does not
// parse is skipped; if no volume can be read at all, the first parse error is
// returned, so the book fails with its reason.
func (w *nestedArchive) walkLimited(b *budget, fn func(e Entry, r io.Reader) error) error {
	// A book that list has already read is walked from the spills and the kept
	// entries. The outer archive is solid in RAR and 7z, so reading it again
	// costs a full decompression pass that the listing has already paid for.
	if steps, ok := w.spilledInOrder(); ok {
		volumes, plain := 0, 0
		for _, st := range steps {
			var err error
			if st.sp == nil {
				plain++
				err = fn(st.e, b.reader(bytes.NewReader(st.data)))
			} else {
				volumes++
				err = walkVolume(st.e.Name, st.sp, b, fn)
			}
			if errors.Is(err, SkipRest) {
				return nil
			}
			if err != nil {
				return err
			}
		}
		if volumes == 0 && plain == 0 {
			return w.unreadable()
		}
		return nil
	}

	volumes, plain, stopped := 0, 0, false
	err := walkArchive(w.outer, b, func(e Entry, r io.Reader) error {
		// Ignored entries and directories never hold a page, so they are not read,
		// the same as in the spilled walk and in list.
		if IgnoredEntry(e.Name) || strings.HasSuffix(e.Name, "/") {
			return nil
		}
		var err error
		if w.volume[e.Name] {
			// r is already counted against b by the outer walk, so it is spilled
			// as it is; counting it again would charge the budget twice.
			var sp *spilled
			sp, err = w.load(e.Name, w.size[e.Name], r)
			if errors.Is(err, errBadVolume) {
				return nil
			}
			if err == nil {
				volumes++
				err = walkVolume(e.Name, sp, b, fn)
			}
		} else {
			plain++
			err = fn(e, r)
		}
		if errors.Is(err, SkipRest) {
			stopped = true
		}
		return err
	})
	if err != nil {
		return err
	}
	// Plain outer entries may still be pages, so only a book with nothing but
	// unreadable volumes fails.
	if volumes == 0 && plain == 0 && !stopped {
		return w.unreadable()
	}
	return nil
}

// walkVolume walks the entries of volume sp, each renamed "volume/entry". A
// volume's ComicInfo.xml is skipped (see isVolumeComicInfo). It returns
// SkipRest when fn asked to stop, so the caller stops the whole walk.
func walkVolume(volume string, sp *spilled, b *budget, fn func(e Entry, r io.Reader) error) error {
	skip := false
	err := walkArchive(sp.arc, b, func(ie Entry, ir io.Reader) error {
		if isVolumeComicInfo(ie.Name) {
			return nil
		}
		ie.Name = volume + "/" + ie.Name
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
}

// spilledInOrder returns the walk's steps in outer order, and ok, when a walk
// need not read the outer archive. That holds when every outer entry is a
// volume that is spilled (or unreadable, and so skipped), an ignored or
// directory entry, or a small plain entry that list kept. Otherwise ok is false
// and the caller walks the outer archive.
func (w *nestedArchive) spilledInOrder() (steps []walkStep, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, e := range w.outer.Entries() {
		if IgnoredEntry(e.Name) || strings.HasSuffix(e.Name, "/") {
			continue
		}
		if !w.volume[e.Name] {
			data, kept := w.kept[e.Name]
			if !kept {
				return nil, false
			}
			steps = append(steps, walkStep{e: e, data: data})
			continue
		}
		sp := w.inner[e.Name]
		if sp == nil {
			return nil, false
		}
		if sp.err != nil {
			continue // unreadable: skipped, as the outer walk does
		}
		steps = append(steps, walkStep{e: Entry{Name: e.Name}, sp: sp})
	}
	return steps, true
}

// unreadable returns the parse error of the first volume in outer order when
// no volume could be read, and nil otherwise. A book whose volumes all fail
// gets that reason rather than an empty listing.
func (w *nestedArchive) unreadable() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	var first error
	for _, name := range w.order {
		sp, ok := w.inner[name]
		if !ok {
			continue
		}
		if sp.err == nil {
			return nil
		}
		if first == nil {
			first = sp.err
		}
	}
	return first
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
	//
	// Small plain entries are kept on the same pass. Otherwise a walk after the
	// listing would have to read the outer archive again for them, which costs a
	// full decompression pass on a solid archive.
	kept := make(map[string][]byte)
	var keptTotal int64
	var nested []Entry
	err := walkArchive(w.outer, newBudget(Limit{Ctx: w.ctx, MaxBytes: maxSpillTotal}), func(e Entry, r io.Reader) error {
		if IgnoredEntry(e.Name) || strings.HasSuffix(e.Name, "/") {
			return nil
		}
		if !w.volume[e.Name] {
			// One byte past the cap tells a too-large entry apart without reading it
			// all; such an entry is not kept.
			data, err := io.ReadAll(io.LimitReader(r, maxKeptEntry+1))
			if err != nil {
				return err
			}
			if len(data) <= maxKeptEntry && keptTotal+int64(len(data)) <= maxKeptTotal {
				kept[e.Name] = data
				keptTotal += int64(len(data))
			}
			return nil
		}
		sp, err := w.load(e.Name, w.size[e.Name], r)
		if errors.Is(err, errBadVolume) {
			// A damaged volume is left out, and the rest of the book is listed.
			return nil
		}
		if err != nil {
			return err
		}
		for _, ie := range sp.arc.Entries() {
			if isVolumeComicInfo(ie.Name) {
				continue
			}
			ie.Name = e.Name + "/" + ie.Name
			nested = append(nested, ie)
		}
		return nil
	})
	w.mu.Lock()
	w.kept = kept
	w.mu.Unlock()
	if err != nil {
		return plain, err
	}
	if len(nested) == 0 && !hasPlain(plain) {
		if bad := w.unreadable(); bad != nil {
			return plain, bad
		}
	}
	return append(plain, nested...), nil
}

// hasPlain reports whether any outer entry that is not a volume could be a
// page: one that is neither ignored nor a directory.
func hasPlain(es []Entry) bool {
	for _, e := range es {
		if !IgnoredEntry(e.Name) && !strings.HasSuffix(e.Name, "/") {
			return true
		}
	}
	return false
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

// Close closes the volumes, removes their temp files, gives their bytes back to
// the disk budget, and closes the outer archive. It returns the first error.
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
	releaseDisk(w.charged)
	w.charged = 0
	w.kept = nil
	keep(w.outer.Close())
	return first
}
