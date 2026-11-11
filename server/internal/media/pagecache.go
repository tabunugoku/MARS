package media

import (
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
)

// PageCache keeps extracted pages on disk for archives without cheap random
// access. Layout: Dir/<bookID>/<n> for each page, and Dir/<bookID>/key
// holding the file-version key, written last. Use it through a pointer.
type PageCache struct {
	Dir      string // e.g. DATA/cache/pages
	MaxBytes int64  // total budget; least recently used books are evicted beyond it

	mu    sync.Mutex
	books map[string]*bookLock
}

// bookLock serialises extraction for one book. refs counts the holders and
// waiters so the entry can be removed from the map when nobody uses it.
type bookLock struct {
	mu   sync.Mutex
	refs int
}

// Page returns the filesystem path of page n (1-based) of the book,
// extracting the whole book on first use. key identifies the file version
// (e.g. size+mtime); a different key re-extracts.
func (c *PageCache) Page(bookID, key, archivePath string, pages []Page, n int) (string, error) {
	if !bookIDPattern.MatchString(bookID) {
		return "", fmt.Errorf("media: invalid book id %q", bookID)
	}
	if n < 1 || n > len(pages) {
		return "", fmt.Errorf("media: page %d out of range 1..%d", n, len(pages))
	}
	unlock := c.lockBook(bookID)
	defer unlock()

	bookDir := filepath.Join(c.Dir, bookID)
	if c.valid(bookDir, key) {
		now := time.Now()
		if err := os.Chtimes(filepath.Join(bookDir, keyFileName), now, now); err != nil {
			return "", err
		}
	} else {
		if err := c.extract(bookID, key, archivePath, pages); err != nil {
			return "", err
		}
		_ = c.evict(bookID) // best effort: a failed sweep must not fail the read
	}

	page := filepath.Join(bookDir, strconv.Itoa(n))
	if _, err := os.Stat(page); err != nil {
		return "", fmt.Errorf("media: page %d not cached: %w", n, err)
	}
	return page, nil
}

func (c *PageCache) lockBook(id string) func() {
	c.mu.Lock()
	if c.books == nil {
		c.books = make(map[string]*bookLock)
	}
	bl := c.books[id]
	if bl == nil {
		bl = &bookLock{}
		c.books[id] = bl
	}
	bl.refs++
	c.mu.Unlock()

	bl.mu.Lock()
	return func() {
		bl.mu.Unlock()
		c.mu.Lock()
		bl.refs--
		if bl.refs == 0 {
			delete(c.books, id)
		}
		c.mu.Unlock()
	}
}

// valid reports whether bookDir was extracted from the file version key.
func (c *PageCache) valid(bookDir, key string) bool {
	b, err := os.ReadFile(filepath.Join(bookDir, keyFileName))
	return err == nil && string(b) == key
}

// extract writes every page into a temp dir, then swaps it in for Dir/<bookID>.
func (c *PageCache) extract(bookID, key, archivePath string, pages []Page) error {
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return err
	}
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

	written, err := writePages(tmp, archivePath, pages)
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
// written, mapped to their page number.
func writePages(dir, archivePath string, pages []Page) (map[string]int, error) {
	want := make(map[string]int, len(pages))
	for _, p := range pages {
		if _, dup := want[p.Entry]; !dup {
			want[p.Entry] = p.Number
		}
	}
	written := make(map[string]int, len(want))

	a, _, err := archive.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer a.Close()

	var total int64
	err = a.Walk(func(e archive.Entry, r io.Reader) error {
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

// evict removes the least recently used book dirs while the cache is over
// MaxBytes, never removing keep. Temp dirs older than an hour are removed;
// younger ones may belong to an extraction in progress and are left alone.
func (c *PageCache) evict(keep string) error {
	entries, err := os.ReadDir(c.Dir)
	if err != nil {
		return err
	}
	type book struct {
		name string
		size int64
		used time.Time
	}
	var books []book
	var total int64
	now := time.Now()
	for _, de := range entries {
		if !de.IsDir() {
			continue
		}
		dir := filepath.Join(c.Dir, de.Name())
		if strings.Contains(de.Name(), tmpInfix) {
			if info, err := os.Stat(dir); err == nil && now.Sub(info.ModTime()) >= staleTmp {
				_ = os.RemoveAll(dir)
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
		books = append(books, book{de.Name(), size, used})
		total += size
	}

	sort.Slice(books, func(i, j int) bool { return books[i].used.Before(books[j].used) })
	count := len(books)
	for _, b := range books {
		if total <= c.MaxBytes || count <= 1 {
			break
		}
		if b.name == keep {
			continue
		}
		if err := os.RemoveAll(filepath.Join(c.Dir, b.name)); err != nil {
			return err
		}
		total -= b.size
		count--
	}
	return nil
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
