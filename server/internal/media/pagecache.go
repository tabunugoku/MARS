package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/archive"
)

var bookIDPattern = regexp.MustCompile(`^[0-9A-Za-z]{1,64}$`)

const (
	keyFileName = "key"
	tmpInfix    = ".tmp-"
	staleTmp    = time.Hour

	defaultExtractions = 2
)

// PageCache keeps extracted pages on disk for archives without cheap random
// access. Layout: Dir/<bookID>/<n> for each page, and Dir/<bookID>/key
// holding the file-version key, written last. Use it through a pointer.
type PageCache struct {
	Dir      string // e.g. DATA/cache/pages
	MaxBytes int64  // total budget; least recently used books are evicted beyond it

	// MaxExtractions limits how many archives are extracted at once, which
	// bounds the temporary disk space in use. Zero means 2.
	MaxExtractions int

	mu    sync.Mutex
	books map[string]*bookLock
	sem   chan struct{} // extraction slots, created on first use under mu
}

// bookLock serialises extraction for one book. refs counts the holders and
// waiters so the entry can be removed from the map when nobody uses it.
type bookLock struct {
	mu   sync.Mutex
	refs int
}

// Open returns page n (1-based) of the book as an open file, extracting the
// whole book on first use. key identifies the file version (e.g. size+mtime);
// a different key re-extracts. The file is opened while the book's lock is
// held, so eviction cannot delete it between validation and open. The caller
// must close it.
func (c *PageCache) Open(ctx context.Context, bookID, key string, archivePath archive.Path, pages []Page, n int) (*os.File, error) {
	if !bookIDPattern.MatchString(bookID) {
		return nil, fmt.Errorf("media: invalid book id %q", bookID)
	}
	if n < 1 || n > len(pages) {
		return nil, fmt.Errorf("media: page %d out of range 1..%d", n, len(pages))
	}
	unlock := c.lockBook(bookID)
	defer unlock()

	bookDir := filepath.Join(c.Dir, bookID)
	if c.valid(bookDir, key) {
		now := time.Now()
		if err := os.Chtimes(filepath.Join(bookDir, keyFileName), now, now); err != nil {
			return nil, err
		}
	} else if err := c.extract(ctx, bookID, key, archivePath, pages); err != nil {
		return nil, err
	}

	f, err := os.Open(filepath.Join(bookDir, strconv.Itoa(n)))
	if err != nil {
		return nil, fmt.Errorf("media: page %d not cached: %w", n, err)
	}
	return f, nil
}

// lockBook takes the per-book lock and returns the function that releases it.
func (c *PageCache) lockBook(id string) func() {
	bl := c.ref(id)
	bl.mu.Lock()
	return c.unlocker(id, bl)
}

// tryLockBook takes the per-book lock without waiting. ok is false when another
// request holds it; in that case there is nothing to release.
func (c *PageCache) tryLockBook(id string) (unlock func(), ok bool) {
	bl := c.ref(id)
	if !bl.mu.TryLock() {
		c.unref(id, bl)
		return nil, false
	}
	return c.unlocker(id, bl), true
}

// ref registers a holder or waiter for the book's lock entry.
func (c *PageCache) ref(id string) *bookLock {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.books == nil {
		c.books = make(map[string]*bookLock)
	}
	bl := c.books[id]
	if bl == nil {
		bl = &bookLock{}
		c.books[id] = bl
	}
	bl.refs++
	return bl
}

// unref drops a holder or waiter and removes the entry when none are left.
func (c *PageCache) unref(id string, bl *bookLock) {
	c.mu.Lock()
	defer c.mu.Unlock()
	bl.refs--
	if bl.refs == 0 {
		delete(c.books, id)
	}
}

func (c *PageCache) unlocker(id string, bl *bookLock) func() {
	return func() {
		bl.mu.Unlock()
		c.unref(id, bl)
	}
}

// semaphore returns the channel that limits concurrent extractions.
func (c *PageCache) semaphore() chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sem == nil {
		n := c.MaxExtractions
		if n <= 0 {
			n = defaultExtractions
		}
		c.sem = make(chan struct{}, n)
	}
	return c.sem
}

// valid reports whether bookDir was extracted from the file version key.
func (c *PageCache) valid(bookDir, key string) bool {
	b, err := os.ReadFile(filepath.Join(bookDir, keyFileName))
	return err == nil && string(b) == key
}

// extract makes room for the book, then writes it. It waits for an extraction
// slot first, and gives up with ctx.Err() if ctx ends while waiting. The
// caller must hold the book's lock.
func (c *PageCache) extract(ctx context.Context, bookID, key string, archivePath archive.Path, pages []Page) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sem := c.semaphore()
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-sem }()

	info, err := archivePath.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return err
	}
	// Reserve the archive size up front, so concurrent extractions do not
	// together overrun the budget. The archive size is only an estimate.
	c.evict(bookID, info.Size())
	if err := c.writeBook(ctx, bookID, key, archivePath, pages); err != nil {
		return err
	}
	c.evict(bookID, 0) // the real size is known now; trim to the budget
	return nil
}

// writeBook writes every page into a temp dir, then swaps it in for Dir/<bookID>.
func (c *PageCache) writeBook(ctx context.Context, bookID, key string, archivePath archive.Path, pages []Page) error {
	tmp, err := os.MkdirTemp(c.Dir, bookID+tmpInfix)
	if err != nil {
		return err
	}
	done := false
	defer func() {
		if !done {
			_ = os.RemoveAll(tmp)
		}
	}()

	written, err := writePages(ctx, tmp, archivePath, pages)
	if err != nil {
		return err
	}
	for _, p := range pages {
		if _, ok := written[p.Entry]; !ok {
			return fmt.Errorf("media: page %d (%q) not found in archive", p.Number, p.Entry)
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, keyFileName), []byte(key), 0o644); err != nil {
		return err
	}
	final := filepath.Join(c.Dir, bookID)
	if err := os.RemoveAll(final); err != nil {
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		return err
	}
	done = true
	return nil
}

// writePages walks the archive once and writes each page entry to dir/<n>.
// Duplicate entry names keep the first page number, and duplicate entries in
// the archive keep the first occurrence. It returns the entry names that were
// written, mapped to their page number. It stops with ctx.Err() once ctx ends.
func writePages(ctx context.Context, dir string, archivePath archive.Path, pages []Page) (map[string]int, error) {
	want := make(map[string]int, len(pages))
	for _, p := range pages {
		if _, dup := want[p.Entry]; !dup {
			want[p.Entry] = p.Number
		}
	}
	written := make(map[string]int, len(want))

	ctx, cancel := context.WithTimeout(ctx, ExtractTimeout)
	defer cancel()
	// OpenPathContext, not OpenPath: parsing a PDF is the slow part and must
	// stop when the timeout or the request ends.
	a, _, err := archive.OpenPathContext(ctx, archivePath)
	if err != nil {
		return nil, err
	}
	defer a.Close()

	var total int64
	err = archive.WalkLimited(a, archive.Limit{Ctx: ctx, MaxBytes: MaxDecompressed}, func(e archive.Entry, r io.Reader) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		num, ok := want[e.Name]
		if !ok {
			return nil
		}
		if _, dup := written[e.Name]; dup {
			return nil
		}
		f, err := os.OpenFile(filepath.Join(dir, strconv.Itoa(num)), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		// Read at most the remaining budget plus one byte, so an oversized
		// book is detected without writing much past the limit.
		n, err := io.Copy(f, io.LimitReader(r, MaxExtractedBook-total+1))
		cerr := f.Close()
		total += n
		written[e.Name] = num
		if err != nil {
			return err
		}
		if cerr != nil {
			return cerr
		}
		if total > MaxExtractedBook {
			return ErrCacheTooBig
		}
		if len(written) == len(want) {
			return archive.SkipRest
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return written, nil
}

// evict removes the least recently used book dirs while the cache, counting
// reserve bytes for an extraction about to start, is over MaxBytes. It never
// removes keep, and it skips any book whose lock is held by another request.
// Temp dirs count toward the total; those older than an hour are removed, and
// younger ones may belong to an extraction in progress and are left alone.
// Failures are ignored: eviction is best effort.
func (c *PageCache) evict(keep string, reserve int64) {
	entries, err := os.ReadDir(c.Dir)
	if err != nil {
		return
	}
	type book struct {
		name string
		size int64
		used time.Time
	}
	var books []book
	total := reserve
	now := time.Now()
	for _, de := range entries {
		if !de.IsDir() {
			continue
		}
		name := de.Name()
		dir := filepath.Join(c.Dir, name)
		if strings.Contains(name, tmpInfix) {
			if info, err := os.Stat(dir); err == nil && now.Sub(info.ModTime()) >= staleTmp {
				if os.RemoveAll(dir) == nil {
					continue
				}
			}
			if size, err := dirSize(dir); err == nil {
				total += size
			}
			continue
		}
		size, err := dirSize(dir)
		if err != nil {
			continue
		}
		var used time.Time // zero: no key file, so this is evicted first
		if info, err := os.Stat(filepath.Join(dir, keyFileName)); err == nil {
			used = info.ModTime()
		}
		books = append(books, book{name, size, used})
		total += size
	}

	sort.Slice(books, func(i, j int) bool { return books[i].used.Before(books[j].used) })
	for _, b := range books {
		if total <= c.MaxBytes {
			break
		}
		if b.name == keep {
			continue
		}
		unlock, ok := c.tryLockBook(b.name)
		if !ok {
			continue // in use by another request; try the next oldest
		}
		// Drop the key first, so a removal that fails part-way cannot leave
		// a dir that still looks valid.
		_ = os.Remove(filepath.Join(c.Dir, b.name, keyFileName))
		err := os.RemoveAll(filepath.Join(c.Dir, b.name))
		unlock()
		if err != nil {
			continue // e.g. Windows: a file is still open; move on
		}
		total -= b.size
	}
}

func dirSize(dir string) (int64, error) {
	var size int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			size += info.Size()
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	return size, err
}
