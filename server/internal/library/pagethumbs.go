package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tabunugoku/mars/server/internal/media"
)

// Page thumbnails are small JPEGs of single pages for page lists. A list
// shows dozens at once and each source page can be several megabytes, so
// they are made once on the server and kept on disk:
// PageThumbDir/<bookID>/<version>-<n>.jpg. The cache is bounded by
// PageThumbMax; the least recently written files go first.
const (
	pageThumbW, pageThumbH = 320, 480
	pageThumbWorkers       = 2 // each decodes a full page: bound memory
	pageThumbTimeout       = time.Minute
	defaultPageThumbMax    = 512 << 20
)

// PageThumb is a page thumbnail ready to serve. File is open (a prune cannot
// take it away mid-response); the caller closes it.
type PageThumb struct {
	File    *os.File
	ETag    string
	Version string
}

// errStale: the book file changed while a thumbnail was being made.
var errStale = errors.New("page thumbnail: book changed")

type pageThumbs struct {
	once     sync.Once
	sem      chan struct{}
	mu       sync.Mutex
	inflight map[string]chan struct{}
	written  int64      // bytes written since the last prune
	publish  sync.Mutex // a new file and the removal of older versions
}

func (p *pageThumbs) init() {
	p.once.Do(func() {
		p.sem = make(chan struct{}, pageThumbWorkers)
		p.inflight = map[string]chan struct{}{}
	})
}

// PageThumbnail returns the thumbnail of page n, making it on first use.
// Requests for the same page wait for one maker; at most pageThumbWorkers
// pages are made at once, and a caller whose ctx ends stops waiting.
func (s *Service) PageThumbnail(ctx context.Context, bookID string, n int) (PageThumb, error) {
	if s.PageThumbDir == "" {
		return PageThumb{}, ErrNotFound
	}
	p := &s.pageThumbs
	p.init()
	// A few rounds: a prune or a changed book file can undo one.
	for range 4 {
		v, err := s.pageVersion(ctx, bookID, n)
		if err != nil {
			return PageThumb{}, err
		}
		path := filepath.Join(s.PageThumbDir, bookID, fmt.Sprintf("%s-%d.jpg", v, n))
		if f, err := os.Open(path); err == nil {
			return PageThumb{File: f, ETag: fmt.Sprintf(`"t-%s-%d-%s"`, bookID, n, v), Version: v}, nil
		}
		p.mu.Lock()
		wait, busy := p.inflight[path]
		if !busy {
			wait = make(chan struct{})
			p.inflight[path] = wait
		}
		p.mu.Unlock()
		if busy {
			select {
			case <-wait:
				continue // made (or failed): look again
			case <-ctx.Done():
				return PageThumb{}, ctx.Err()
			}
		}
		err = s.makePageThumb(ctx, bookID, n, v, path)
		p.mu.Lock()
		delete(p.inflight, path)
		p.mu.Unlock()
		close(wait)
		if err != nil && !errors.Is(err, errStale) {
			return PageThumb{}, err
		}
	}
	return PageThumb{}, errors.New("page thumbnail: gave up after retries")
}

// pageVersion is the version of the book file a page is served from, or
// ErrNotFound when the book is not ready or has no such page.
func (s *Service) pageVersion(ctx context.Context, bookID string, n int) (string, error) {
	var size int64
	var mtime string
	err := s.DB.QueryRowContext(ctx, `
		SELECT b.size_bytes, b.mtime FROM books b JOIN pages p ON p.book_id = b.id
		WHERE b.id = ? AND p.number = ? AND b.deleted_at IS NULL AND b.media_status = 'ready'`, bookID, n).Scan(&size, &mtime)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return Version(size, mtime), nil
}

func (s *Service) makePageThumb(ctx context.Context, bookID string, n int, v, path string) error {
	p := &s.pageThumbs
	select {
	case p.sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-p.sem }()
	// Once started, finish even if this caller leaves: others may wait.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pageThumbTimeout)
	defer cancel()

	pg, err := s.OpenPage(ctx, bookID, n)
	if err != nil {
		return err
	}
	if pg.Version != v { // rescanned while this waited: never file new bytes under the old version
		pg.Close()
		return errStale
	}
	img, err := io.ReadAll(io.LimitReader(pg.Body, media.MaxEntryBytes+1))
	pg.Close()
	if err != nil {
		return err
	}
	if len(img) > media.MaxEntryBytes {
		return media.ErrTooLarge
	}
	jpg, err := media.Thumbnail(img, pageThumbW, pageThumbH)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	_, err = f.Write(jpg)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return err
	}
	if err := s.publishPageThumb(ctx, bookID, n, v, f.Name(), path); err != nil {
		os.Remove(f.Name())
		return err
	}
	p.mu.Lock()
	p.written += int64(len(jpg))
	prune := p.written > s.pageThumbMax()/8
	if prune {
		p.written = 0
	}
	p.mu.Unlock()
	if prune {
		go s.prunePageThumbs()
	}
	return nil
}

// publishPageThumb moves a made thumbnail into place and, when its version is
// still the book's, removes thumbnails of other versions (never asked for
// again). Under one lock, so a maker of an outdated version that finishes
// late cannot remove the files of a newer one.
func (s *Service) publishPageThumb(ctx context.Context, bookID string, n int, v, tmp, path string) error {
	p := &s.pageThumbs
	p.publish.Lock()
	defer p.publish.Unlock()
	cur, err := s.pageVersion(ctx, bookID, n)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if cur != v {
		return errStale
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	old, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.jpg"))
	for _, o := range old {
		if !strings.HasPrefix(filepath.Base(o), v+"-") {
			os.Remove(o)
		}
	}
	return nil
}

func (s *Service) pageThumbMax() int64 {
	if s.PageThumbMax > 0 {
		return s.PageThumbMax
	}
	return defaultPageThumbMax
}

// prunePageThumbs deletes the oldest thumbnails until the cache is at most
// 3/4 of its bound, so it does not run again right away.
func (s *Service) prunePageThumbs() {
	type file struct {
		path string
		size int64
		mod  time.Time
	}
	var files []file
	var total int64
	_ = filepath.WalkDir(s.PageThumbDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jpg") {
			return nil
		}
		if info, err := d.Info(); err == nil {
			files = append(files, file{path, info.Size(), info.ModTime()})
			total += info.Size()
		}
		return nil
	})
	limit := s.pageThumbMax()
	if total <= limit {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	for _, f := range files {
		if total <= limit*3/4 {
			break
		}
		if os.Remove(f.path) == nil {
			total -= f.size
			os.Remove(filepath.Dir(f.path)) // only succeeds once the book's dir is empty
		}
	}
}
