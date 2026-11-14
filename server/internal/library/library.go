// Package library ties scanning, analysis and page serving together: it
// queues the background tasks for a library and resolves a book page to
// bytes on disk.
package library

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tabunugoku/mars/server/internal/archive"
	"github.com/tabunugoku/mars/server/internal/db"
	"github.com/tabunugoku/mars/server/internal/events"
	"github.com/tabunugoku/mars/server/internal/media"
	"github.com/tabunugoku/mars/server/internal/scan"
	"github.com/tabunugoku/mars/server/internal/task"
)

var ErrNotFound = errors.New("library: not found")

// thumbBounds returns the box a new thumbnail is fitted into: ThumbSize when
// set, otherwise 400x600.
func (s *Service) thumbBounds(ctx context.Context) (int, int) {
	if s.ThumbSize != nil {
		return s.ThumbSize(ctx)
	}
	return 400, 600
}

// thumbFormat returns the encoding for a new book or series thumbnail:
// ThumbFormat when set, otherwise JPEG.
func (s *Service) thumbFormat(ctx context.Context) media.Format {
	if s.ThumbFormat != nil {
		return s.ThumbFormat(ctx)
	}
	return media.FormatJPEG
}

type Service struct {
	DB       *sql.DB
	Queue    *task.Queue
	ThumbDir string
	Pages    *media.PageCache
	// PageThumbDir keeps page thumbnails (see PageThumbnail), at most
	// PageThumbMax bytes (512 MiB when zero). Empty turns them off.
	PageThumbDir string
	PageThumbMax int64
	pageThumbs   pageThumbs
	Events       *events.Hub // may be nil
	// ThumbSize gives the thumbnail box (width, height) for thumbnails made
	// from now on; nil means 400x600.
	ThumbSize func(ctx context.Context) (w, h int)
	// ThumbFormat gives the encoding (JPEG, WebP or AVIF) of thumbnails made
	// from now on; nil means JPEG. Page thumbnails are always JPEG.
	ThumbFormat func(ctx context.Context) media.Format
	// AnalyzeWorkers is how many books are analysed at once across the whole
	// service, all libraries and tasks together; 0 or less means 1. It is
	// read at the first analysis and fixes the size of the limit.
	AnalyzeWorkers int

	// analysisSlots bounds the books analysed at once (AnalyzeWorkers).
	// pdfSlots bounds the PDF books among them to archive.PDFRenderSlots,
	// so a PDF waits for a renderer outside its analysis timeout.
	analysisSlots slotPool
	pdfSlots      slotPool
	pdfSlotSize   int // test override of archive.PDFRenderSlots; 0 means that
	// analyzeHook, when set, runs as each book's analysis starts, inside its
	// slot and its panic recovery; the func it returns runs when it ends.
	// Tests use it to count books in flight or to make one panic.
	analyzeHook func(bookID string) (done func())

	scanLocks  sync.Map     // library id -> ctxMutex; one walk+sync at a time per library
	coverLocks sync.Map     // book id -> ctxMutex; see coverLock
	scans      scanGate     // every scan sync against PauseScans
	watchDirty watchPending // per library: top-level entries a watcher saw change, not yet scanned

	// deleteBatch is how many books or series one delete transaction removes
	// (see QueueDelete); 0 means deleteBatchSize. deleteHook, when set, runs
	// after each committed batch. Tests use both.
	deleteBatch int
	deleteHook  func()
}

// coverLock orders the writers of a book's cover: an analysis, a cover
// choice and a thumbnail regeneration each read cover_page, write the
// thumbnail file and store the result under it, so a slow analysis cannot
// put back the page an admin just replaced.
// Waiting for it ends with the caller's ctx, so a cancelled request or
// analysis does not sit behind a slow archive read.
func (s *Service) coverLock(bookID string) ctxMutex {
	m, _ := s.coverLocks.LoadOrStore(bookID, newCtxMutex())
	return m.(ctxMutex)
}

func (s *Service) scanLock(id string) ctxMutex {
	m, _ := s.scanLocks.LoadOrStore(id, newCtxMutex())
	return m.(ctxMutex)
}

// slotPool is a counting semaphore. Its size is fixed at the first acquire.
type slotPool struct {
	mu sync.Mutex
	ch chan struct{}
}

// acquire takes one of size slots, waiting until one is free or ctx ends. The
// returned func gives the slot back.
func (p *slotPool) acquire(ctx context.Context, size int) (func(), error) {
	p.mu.Lock()
	if p.ch == nil {
		p.ch = make(chan struct{}, max(size, 1))
	}
	ch := p.ch
	p.mu.Unlock()
	select {
	case ch <- struct{}{}:
		return func() { <-ch }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// pdfSlotCount is how many PDF books may be analysed at once.
func (s *Service) pdfSlotCount() int {
	if s.pdfSlotSize > 0 {
		return s.pdfSlotSize
	}
	return archive.PDFRenderSlots
}

type lib struct {
	id, name, root, kind, interval string
	importComicInfo                bool
	importArtwork                  bool
	exclude                        []string
	lastScan                       sql.NullString
}

func (s *Service) load(ctx context.Context, id string) (lib, error) {
	var l lib
	var excl string
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, name, root, kind, scan_interval, import_comicinfo, import_local_artwork, exclude, last_scan FROM libraries WHERE id = ? AND deleting = 0`, id).
		Scan(&l.id, &l.name, &l.root, &l.kind, &l.interval, &l.importComicInfo, &l.importArtwork, &excl, &l.lastScan)
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrNotFound
	}
	if err == nil {
		_ = json.Unmarshal([]byte(excl), &l.exclude)
	}
	return l, err
}

// QueueScan queues a scan of the library; when it finishes it queues the
// analysis of new and changed books.
func (s *Service) QueueScan(ctx context.Context, libraryID string) (task.Task, error) {
	return s.queueScan(ctx, libraryID, false)
}

// QueueDeepScan is QueueScan followed by the analysis of every book of the
// library, changed or not (a deep scan). Books stay readable meanwhile: each
// one is updated in place when its analysis finishes.
func (s *Service) QueueDeepScan(ctx context.Context, libraryID string) (task.Task, error) {
	return s.queueScan(ctx, libraryID, true)
}

func (s *Service) queueScan(ctx context.Context, libraryID string, deep bool) (task.Task, error) {
	l, err := s.load(ctx, libraryID)
	if err != nil {
		return task.Task{}, err
	}
	key, label := "scan", task.Text{Code: "task.scan", Arg: l.name, English: "Scan " + l.name}
	if deep {
		key, label = "scan:deep", task.Text{Code: "task.scan_deep", Arg: l.name, English: "Deep scan " + l.name}
	}
	return s.Queue.SubmitKey(ctx, "scan", key, label, l.id, func(ctx context.Context, progress func(float64)) error {
		return s.scanLibrary(ctx, libraryID, nil, deep, progress)
	})
}

// queueWatchTask queues the scan a file watcher asks for. It has its own queue
// key, so a queued one takes in later watcher requests. When it runs it takes
// the library's pending entries (see addWatchDirty): only those folders are
// scanned, or the whole library when the set is marked full or is empty.
func (s *Service) queueWatchTask(ctx context.Context, libraryID string) (task.Task, error) {
	l, err := s.load(ctx, libraryID)
	if err != nil {
		return task.Task{}, err
	}
	label := task.Text{Code: "task.scan_partial", Arg: l.name, English: "Scan part of " + l.name}
	return s.Queue.SubmitKey(ctx, "scan", "scan:watch", label, l.id, func(ctx context.Context, progress func(float64)) error {
		scope, full := s.takeWatchDirty(libraryID)
		if full || len(scope) == 0 {
			scope = nil
		}
		err := s.scanLibrary(ctx, libraryID, scope, false, progress)
		if err != nil {
			// The taken entries were not scanned: the next watch scan is full.
			s.addWatchDirty(libraryID, nil, true)
		}
		return err
	})
}

// scanLibrary is the body of a scan task. It walks the library (only scope
// when scope is not nil), syncs the rows, then syncs the artwork, announces
// the change and queues the analysis. A scoped scan leaves the rest alone.
func (s *Service) scanLibrary(ctx context.Context, libraryID string, scope []string, deep bool, progress func(float64)) error {
	l, err := s.load(ctx, libraryID) // settings may have changed while queued
	if err != nil {
		return err
	}
	// A running scan is not deduplicated by the queue, so two can overlap;
	// the lock keeps an older walk from being committed after a newer one.
	if err := s.scans.enter(ctx); err != nil {
		return err
	}
	mu := s.scanLock(l.id)
	if err := mu.Lock(ctx); err != nil {
		s.scans.leave()
		return err
	}
	var found scan.Listing
	if scope == nil {
		found, err = scan.Walk(ctx, l.root, l.exclude)
	} else {
		found, err = scan.WalkScope(ctx, l.root, l.exclude, scope)
	}
	if errors.Is(err, scan.ErrNoScope) {
		// Only entries that are skipped or invalid were dirty: nothing to scan.
		mu.Unlock()
		s.scans.leave()
		return nil
	}
	if err != nil {
		mu.Unlock()
		s.scans.leave()
		// The path goes into the English text only: the API's text has none.
		return &task.CodedError{Code: "task.folder_unreadable", Err: fmt.Errorf("cannot read the folder %s: %w", l.root, err)}
	}
	progress(0.5)
	st, err := scan.Sync(ctx, s.DB, l.id, l.kind, found, time.Now())
	mu.Unlock()
	s.scans.leave()
	if err != nil {
		return err
	}
	if len(found.Unreadable) > 0 {
		slog.Warn("scan: unreadable paths kept as they were", "library", l.name, "paths", found.Unreadable)
	}
	slog.Info("scan", "library", l.name, "stats", st, "scoped", scope != nil)
	artChanged, err := s.syncArtwork(ctx, l, scope)
	if err != nil {
		return err
	}
	// An artwork-only change (a cover file added, replaced or removed)
	// is announced too, so open screens refetch the series covers.
	if st != (scan.Stats{}) || artChanged {
		s.Events.Publish(events.Event{Type: "library", LibraryID: l.id, Data: events.Library{
			LibraryID: l.id, SeriesAdded: st.SeriesAdded, SeriesRemoved: st.SeriesRemoved,
			BooksAdded: st.BooksAdded, BooksChanged: st.BooksChanged, BooksRemoved: st.BooksRemoved,
			Restored: st.Restored,
		}})
	}
	_, err = s.queueAnalyze(context.WithoutCancel(ctx), l.id, deep)
	return err
}

// watchPending holds, per library, the top-level entries that watchers saw
// change and that no watch scan has taken yet.
type watchPending struct {
	mu   sync.Mutex
	libs map[string]*dirtyScope
}

// addWatchDirty merges changes into the library's pending set: the entries,
// or the full mark (scope is then ignored).
func (s *Service) addWatchDirty(libraryID string, scope []string, full bool) {
	p := &s.watchDirty
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.libs == nil {
		p.libs = map[string]*dirtyScope{}
	}
	d := p.libs[libraryID]
	if d == nil {
		d = &dirtyScope{}
		p.libs[libraryID] = d
	}
	if full {
		d.markFull()
		return
	}
	for _, e := range scope {
		d.add(e)
	}
}

// takeWatchDirty empties the library's pending set and returns it as
// dirtyScope.take does. A library with nothing pending gives a full scope.
func (s *Service) takeWatchDirty(libraryID string) ([]string, bool) {
	p := &s.watchDirty
	p.mu.Lock()
	defer p.mu.Unlock()
	d := p.libs[libraryID]
	if d == nil {
		return nil, true
	}
	delete(p.libs, libraryID)
	return d.take()
}

// QueueAnalyze queues the analysis of every live book still 'outdated'.
func (s *Service) QueueAnalyze(ctx context.Context, libraryID string) (task.Task, error) {
	return s.queueAnalyze(ctx, libraryID, false)
}

// queueAnalyze with deep analyses every live book, whatever its status. It
// has its own queue key, so it is never folded into a queued normal one.
func (s *Service) queueAnalyze(ctx context.Context, libraryID string, deep bool) (task.Task, error) {
	l, err := s.load(ctx, libraryID)
	if err != nil {
		return task.Task{}, err
	}
	key, label, which := "analyze", task.Text{Code: "task.analyze", Arg: l.name, English: "Analyze " + l.name}, ` AND media_status = 'outdated'`
	if deep {
		key, label, which = "analyze:deep", task.Text{Code: "task.analyze_deep", Arg: l.name, English: "Deep analyze " + l.name}, ""
	}
	return s.Queue.SubmitKey(ctx, "analyze", key, label, l.id, func(ctx context.Context, progress func(float64)) error {
		l, err := s.load(ctx, libraryID)
		if err != nil {
			return err
		}
		rows, err := s.DB.QueryContext(ctx,
			`SELECT id FROM books WHERE library_id = ? AND deleted_at IS NULL`+which+` ORDER BY path`, l.id)
		if err != nil {
			return err
		}
		var todo []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			todo = append(todo, id)
		}
		rows.Close()
		infos, err := s.analyzeBooks(ctx, l, todo, progress)
		if err != nil {
			return err
		}
		if len(todo) == 0 {
			return nil
		}
		// Series fields come from the first book, chosen only now that every
		// book's number (possibly from its own ComicInfo) is final. One
		// series is one unit; the batch commits every few dozen so progress
		// writes never wait behind the whole library.
		b := &db.Batch{DB: s.DB}
		defer b.Close()
		ts := time.Now().UTC().Format(time.RFC3339Nano)
		for seriesID, byBook := range infos {
			tx, err := b.Tx(ctx)
			if err != nil {
				return err
			}
			if err := applySeriesInfo(ctx, tx, seriesID, byBook, ts); err != nil {
				return err
			}
			// Index in the same transaction: a cancel between the two
			// would leave new titles that search cannot find, and a
			// re-run finds nothing left to analyse to repair it.
			if err := scan.IndexSeries(ctx, tx, seriesID); err != nil {
				return err
			}
			if err := b.Step(ctx); err != nil {
				return err
			}
		}
		if err := b.Flush(); err != nil {
			return err
		}
		if err := scan.SyncSearch(ctx, s.DB, b, l.id); err != nil {
			return err
		}
		if err := b.Flush(); err != nil {
			return err
		}
		// Series titles and fields may have changed from ComicInfo.
		s.Events.Publish(events.Event{Type: "library", LibraryID: l.id, Data: events.Library{LibraryID: l.id, BooksChanged: len(todo)}})
		return nil
	})
}

// analyzeBooks analyses the books in todo and returns the ComicInfo of each
// book that has one, by series id and then book id. This run starts up to
// AnalyzeWorkers workers, but every book also takes a slot of the service-wide
// limit (see analyzeOne), so concurrent runs share AnalyzeWorkers between them.
// Only a database error, a panic or a context end stops the run: the first
// such error is returned, and the books still running are cancelled and
// waited for. Bad files are recorded per book by analyzeBook. progress gets
// the share of books done; calls are serialised, so the values rise to 1.
func (s *Service) analyzeBooks(ctx context.Context, l lib, todo []string, progress func(float64)) (map[string]map[string]scan.ComicInfo, error) {
	workers := min(max(s.AnalyzeWorkers, 1), max(len(todo), 1))
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu       sync.Mutex // guards infos, done, firstErr and the progress calls
		infos    = map[string]map[string]scan.ComicInfo{}
		done     int
		firstErr error
		wg       sync.WaitGroup
	)
	jobs := make(chan string)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				seriesID, ci, err := s.analyzeOne(wctx, l, id)
				mu.Lock()
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					cancel()
					continue
				}
				if ci != nil {
					if infos[seriesID] == nil {
						infos[seriesID] = map[string]scan.ComicInfo{}
					}
					infos[seriesID][id] = *ci
				}
				done++
				progress(float64(done) / float64(len(todo)))
				mu.Unlock()
			}
		}()
	}
send:
	for _, id := range todo {
		select {
		case jobs <- id:
		case <-wctx.Done():
			break send
		}
	}
	close(jobs)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if firstErr != nil {
		return nil, firstErr
	}
	if done < len(todo) {
		return nil, ctx.Err()
	}
	return infos, nil
}

// analyzeOne runs analyzeBook for one book inside the service-wide analysis
// limit. A panic in the archive, decoder or thumbnail code becomes an error
// naming the book, so one bad file cannot take the server down; the error
// stops the run like a database error.
func (s *Service) analyzeOne(ctx context.Context, l lib, id string) (seriesID string, ci *scan.ComicInfo, err error) {
	release, err := s.analysisSlots.acquire(ctx, max(s.AnalyzeWorkers, 1))
	if err != nil {
		return "", nil, err
	}
	defer release()
	defer func() {
		if r := recover(); r != nil {
			slog.Error("analyze: book panicked", "book", id, "panic", r, "stack", string(debug.Stack()))
			seriesID, ci, err = "", nil, fmt.Errorf("analyze book %s: panic: %v", id, r)
		}
	}()
	return s.analyzeBook(ctx, l, id)
}

// version is what the last scan recorded for a book file. Analysis results
// are stored only if the row still has the version they were read under, so
// a slow analysis cannot overwrite a newer scan of a replaced file.
type version struct {
	size  int64
	mtime string
}

// analyzeBook reads one book and stores pages, thumbnail and book metadata.
// It returns the book's series and its ComicInfo (nil when absent or the
// result was discarded). A broken or unsupported file is recorded on the
// book, not returned as an error.
func (s *Service) analyzeBook(ctx context.Context, l lib, bookID string) (string, *scan.ComicInfo, error) {
	if h := s.analyzeHook; h != nil {
		defer h(bookID)()
	}
	var rel, seriesID, lockedJSON string
	var v version
	var coverPage int
	if err := s.DB.QueryRowContext(ctx, `SELECT path, series_id, locked_fields, size_bytes, mtime, cover_page FROM books WHERE id = ?`, bookID).
		Scan(&rel, &seriesID, &lockedJSON, &v.size, &v.mtime, &coverPage); err != nil {
		return "", nil, err
	}
	full, err := resolve(l.root, rel)
	if err != nil {
		return seriesID, nil, s.marked(ctx, l.id, seriesID, bookID, v, "error", err.Error())
	}
	var a media.Analysis
	if archive.IsPDF(rel) {
		// Taken before AnalyzeCover starts its timeout, so a wait for a
		// renderer does not use up the analysis time.
		var release func()
		release, err = s.pdfSlots.acquire(ctx, s.pdfSlotCount())
		if err != nil {
			return "", nil, err
		}
		a, err = media.AnalyzeCover(ctx, full, coverPage)
		release()
	} else {
		a, err = media.AnalyzeCover(ctx, full, coverPage)
	}
	switch {
	case errors.Is(err, archive.ErrRendererBusy):
		// Not the file's fault: leave the book as it is (outdated), so a
		// later analysis tries it again.
		slog.Warn("analyze: PDF renderer unavailable, book left for a later analysis", "book", bookID, "err", err)
		return seriesID, nil, nil
	case errors.Is(err, archive.ErrUnsupported):
		return seriesID, nil, s.marked(ctx, l.id, seriesID, bookID, v, "unsupported", err.Error())
	case err != nil:
		return seriesID, nil, s.marked(ctx, l.id, seriesID, bookID, v, "error", err.Error())
	}

	var ci *scan.ComicInfo
	if l.importComicInfo && a.ComicInfo != nil {
		if c, err := scan.ParseComicInfo(bytes.NewReader(a.ComicInfo)); err == nil {
			ci = &c
		} else {
			slog.Warn("comicinfo", "book", bookID, "err", err)
		}
	}

	// From here to the commit the book's cover is ours: a cover choice made
	// while the archive was being read wins over the page we read then.
	mu := s.coverLock(bookID)
	if err := mu.Lock(ctx); err != nil {
		return "", nil, err
	}
	defer mu.Unlock()
	var nowCover, uploaded int
	var oldThumb string
	if err := s.DB.QueryRowContext(ctx, `SELECT cover_page, thumbnail, thumbnail_uploaded FROM books WHERE id = ?`, bookID).
		Scan(&nowCover, &oldThumb, &uploaded); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return seriesID, nil, nil
		}
		return "", nil, err
	}
	coverChanged := nowCover != coverPage && nowCover >= 1 && nowCover <= len(a.Pages)
	// A changed cover and an uploaded thumbnail both keep their image.
	keep := coverChanged || uploaded == 1

	thumb := ""
	if !keep {
		w, h := s.thumbBounds(ctx)
		f := s.thumbFormat(ctx)
		if jpg, err := media.Thumbnail(a.Cover, w, h, f); err == nil {
			if thumb, err = s.writeThumb(bookID, jpg, f); err != nil {
				return "", nil, err
			}
		} else {
			slog.Warn("thumbnail", "book", bookID, "err", err)
		}
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback()
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	// A changed cover keeps its page and the thumbnail its choice made.
	res, err := tx.ExecContext(ctx, `
		UPDATE books SET page_count = ?, media_status = 'ready', media_error = '',
			thumbnail = CASE WHEN ? THEN thumbnail ELSE ? END,
			cover_page = CASE WHEN ? THEN cover_page ELSE ? END, updated_at = ?
		WHERE id = ? AND size_bytes = ? AND mtime = ? AND deleted_at IS NULL`,
		len(a.Pages), keep, thumb, coverChanged, a.CoverPage, ts, bookID, v.size, v.mtime)
	if err != nil {
		s.RemoveThumb(thumb)
		return "", nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		s.RemoveThumb(thumb)
		return seriesID, nil, nil // rescanned meanwhile; the next analysis takes it
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM pages WHERE book_id = ?`, bookID); err != nil {
		return "", nil, err
	}
	ins, err := tx.PrepareContext(ctx, `INSERT INTO pages (book_id, number, entry, media_type, width, height) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return "", nil, err
	}
	defer ins.Close()
	for _, p := range a.Pages {
		if _, err := ins.ExecContext(ctx, bookID, p.Number, p.Entry, p.MediaType, p.Width, p.Height); err != nil {
			return "", nil, err
		}
	}
	if ci != nil {
		if err := applyBookInfo(ctx, tx, bookID, lockedJSON, *ci, ts); err != nil {
			return "", nil, err
		}
		if err := scan.IndexBook(ctx, tx, bookID); err != nil {
			return "", nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		s.RemoveThumb(thumb)
		return "", nil, err
	}
	if !keep {
		s.RemoveThumb(oldThumb) // replaced by thumb (or cleared when none was made)
	}
	s.bookChanged(l.id, seriesID, bookID, "ready")
	return seriesID, ci, nil
}

// marked is markBook followed by a "book" event.
func (s *Service) marked(ctx context.Context, libraryID, seriesID, bookID string, v version, status, msg string) error {
	if err := s.markBook(ctx, bookID, v, status, msg); err != nil {
		return err
	}
	s.bookChanged(libraryID, seriesID, bookID, status)
	return nil
}

// bookChanged announces that a book's analysis or thumbnail changed.
func (s *Service) bookChanged(libraryID, seriesID, bookID, status string) {
	s.Events.Publish(events.Event{Type: "book", LibraryID: libraryID, Data: events.Book{
		BookID: bookID, SeriesID: seriesID, LibraryID: libraryID, MediaStatus: status,
	}})
}

func (s *Service) markBook(ctx context.Context, bookID string, v version, status, msg string) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE books SET media_status = ?, media_error = ?, page_count = 0, updated_at = ?
		WHERE id = ? AND size_bytes = ? AND mtime = ?`,
		status, msg, time.Now().UTC().Format(time.RFC3339Nano), bookID, v.size, v.mtime)
	return err
}

// resolve pairs a stored relative path with the library root, refusing
// '..' and absolute paths.
func resolve(root, rel string) (archive.Path, error) {
	if !filepath.IsLocal(filepath.FromSlash(rel)) {
		return archive.Path{}, fmt.Errorf("invalid path: %q", rel)
	}
	// Containment is enforced when the file is opened: archive.OpenPath
	// goes through os.Root, which refuses anything that resolves outside
	// root at that moment, symlinks included. A check here followed by an
	// open by name would leave a window to swap a folder for a symlink.
	return archive.Path{Root: root, Rel: rel}, nil
}

func locked(lockedJSON string) map[string]bool {
	var l []string
	_ = json.Unmarshal([]byte(lockedJSON), &l)
	m := map[string]bool{}
	for _, f := range l {
		m[f] = true
	}
	return m
}

func jsonText(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func applyBookInfo(ctx context.Context, tx *sql.Tx, bookID, lockedJSON string, ci scan.ComicInfo, ts string) error {
	lk := locked(lockedJSON)
	set := []string{}
	args := []any{}
	add := func(field, col string, v any) {
		if !lk[field] {
			set = append(set, col+" = ?")
			args = append(args, v)
		}
	}
	if ci.Title != "" {
		add("title", "title", ci.Title)
	}
	if n, ok := ci.NumberValue(); ok {
		add("number", "number", n)
		add("number", "number_sort", n)
		add("number", "number_source", "comicinfo")
	}
	if ci.Summary != "" {
		add("summary", "summary", ci.Summary)
	}
	if d := ci.ReleaseDate(); d != "" {
		add("releaseDate", "release_date", d)
	}
	if isbn := ci.ISBN(); isbn != "" {
		add("isbn", "isbn", isbn)
	}
	if au := ci.Authors(); len(au) > 0 {
		add("authors", "authors", jsonText(au))
	}
	if tags := ci.TagList(); len(tags) > 0 {
		add("tags", "tags", jsonText(tags))
	}
	if len(set) == 0 {
		return nil
	}
	args = append(args, ts, bookID)
	_, err := tx.ExecContext(ctx, `UPDATE books SET `+strings.Join(set, ", ")+`, updated_at = ? WHERE id = ?`, args...)
	return err
}

// applySeriesInfo copies series-level fields from the series' first book
// only, so books that disagree cannot make the series flip on every scan.
// byBook holds the ComicInfo of the books analysed in this run; when the
// first book is not among them, the series is left as it is.
func applySeriesInfo(ctx context.Context, tx *sql.Tx, seriesID string, byBook map[string]scan.ComicInfo, ts string) error {
	var first, lockedJSON string
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM books WHERE series_id = ? AND deleted_at IS NULL ORDER BY number_sort, path LIMIT 1`, seriesID).Scan(&first)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	ci, ok := byBook[first]
	if !ok {
		return nil
	}
	if err := tx.QueryRowContext(ctx, `SELECT locked_fields FROM series WHERE id = ?`, seriesID).Scan(&lockedJSON); err != nil {
		return err
	}
	lk := locked(lockedJSON)
	set := []string{}
	args := []any{}
	add := func(field, col string, v any) {
		if !lk[field] {
			set = append(set, col+" = ?")
			args = append(args, v)
		}
	}
	if ci.Series != "" {
		add("title", "title", ci.Series)
		add("titleSort", "title_sort", scan.SortTitle(ci.Series))
	}
	if ci.Publisher != "" {
		add("publisher", "publisher", ci.Publisher)
	}
	if g := ci.GenreList(); len(g) > 0 {
		add("genres", "genres", jsonText(g))
	}
	if ci.LanguageISO != "" {
		add("language", "language", ci.LanguageISO)
	}
	if d := ci.Direction(); d != "" {
		add("readingDirection", "reading_direction", d)
	}
	if ci.Count > 0 {
		add("totalBookCount", "total_book_count", ci.Count)
	}
	if au := ci.Authors(); len(au) > 0 {
		add("authors", "authors", jsonText(au))
	}
	if len(set) == 0 {
		return nil
	}
	args = append(args, ts, seriesID)
	_, err = tx.ExecContext(ctx, `UPDATE series SET `+strings.Join(set, ", ")+`, updated_at = ? WHERE id = ?`, args...)
	return err
}

// Page is a page ready to stream. Body yields at most Size bytes.
type Page struct {
	Body      io.Reader
	Size      int64
	MediaType string
	ETag      string // strong; changes when the file changes
	Version   string // see Version
	Close     func() error
}

// pdfRenderRevision is part of every PDF's version. Bump it when the way PDF
// pages are served changes (which pages are drawn, or how), so pages cached
// by browsers, page thumbnails and the page cache are not reused for the
// same file. Revision 2: pages that are not one image are drawn by pdfium.
const pdfRenderRevision = "2"

// Version is the token page URLs carry (?v=) so a replaced file gets new
// URLs and old cached responses are never reused for it. format is the
// book's format (books.format).
func Version(size int64, mtime, format string) string {
	s := fmt.Sprintf("%d-%s", size, mtime)
	if archive.Format(format) == archive.PDF {
		s += "-pdf" + pdfRenderRevision
	}
	return fmt.Sprintf("%x", hash(s))
}

// OpenPage resolves page n (1-based) of a ready book. Zip pages stream
// straight from the archive (nothing is buffered in memory); RAR and 7z pages
// come from the page cache, which is checked before the archive is touched.
func (s *Service) OpenPage(ctx context.Context, bookID string, n int) (Page, error) {
	var root, rel, format, entry, mediaType, mtime string
	var size int64
	err := s.DB.QueryRowContext(ctx, `
		SELECT l.root, b.path, b.format, b.size_bytes, b.mtime, p.entry, p.media_type
		FROM books b JOIN libraries l ON l.id = b.library_id JOIN pages p ON p.book_id = b.id
		WHERE b.id = ? AND p.number = ? AND b.deleted_at IS NULL AND b.media_status = 'ready'`, bookID, n).
		Scan(&root, &rel, &format, &size, &mtime, &entry, &mediaType)
	if errors.Is(err, sql.ErrNoRows) {
		return Page{}, ErrNotFound
	}
	if err != nil {
		return Page{}, err
	}
	full, err := resolve(root, rel)
	if err != nil {
		return Page{}, err
	}
	v := Version(size, mtime, format)
	etag := fmt.Sprintf(`"%s-%d-%s"`, bookID, n, v)

	if archive.Format(format) == archive.CBZ {
		a, _, err := archive.OpenPath(full)
		if err != nil {
			return Page{}, err
		}
		var entrySize int64 = -1
		for _, e := range a.Entries() {
			if e.Name == entry {
				entrySize = e.Size
				break
			}
		}
		if entrySize < 0 || entrySize > media.MaxEntryBytes {
			a.Close()
			return Page{}, media.ErrTooLarge
		}
		rc, err := a.Open(entry)
		if err != nil {
			a.Close()
			return Page{}, err
		}
		return Page{
			Body: io.LimitReader(rc, entrySize), Size: entrySize, MediaType: mediaType, ETag: etag, Version: v,
			Close: func() error { rc.Close(); return a.Close() },
		}, nil
	}

	pages, err := s.pages(ctx, bookID)
	if err != nil {
		return Page{}, err
	}
	f, err := s.Pages.Open(ctx, bookID, v, full, pages, n)
	if err != nil {
		return Page{}, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return Page{}, err
	}
	return Page{Body: f, Size: st.Size(), MediaType: mediaType, ETag: etag, Version: v, Close: f.Close}, nil
}

func (s *Service) pages(ctx context.Context, bookID string) ([]media.Page, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT number, entry, media_type, width, height FROM pages WHERE book_id = ? ORDER BY number`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []media.Page
	for rows.Next() {
		var p media.Page
		if err := rows.Scan(&p.Number, &p.Entry, &p.MediaType, &p.Width, &p.Height); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func hash(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// ThumbnailPath returns the cover file of a book, or of a series' first
// book that has one.
func (s *Service) ThumbnailPath(ctx context.Context, kind, id string) (string, error) {
	q := `SELECT thumbnail FROM books WHERE id = ? AND deleted_at IS NULL`
	if kind == "series" {
		q = `SELECT ` + SeriesCoverSQL + ` FROM series s WHERE s.id = ?`
	}
	var name string
	err := s.DB.QueryRowContext(ctx, q, id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && name == "") {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return filepath.Join(s.ThumbDir, filepath.Base(name)), nil
}

var intervals = map[string]time.Duration{"hourly": time.Hour, "daily": 24 * time.Hour, "weekly": 7 * 24 * time.Hour}

// Schedule runs the first check when the server starts, then one every
// minute. Each check queues a scan of every library that is due: one never
// scanned, or one whose scan interval has passed since last_scan. A library
// with interval "off" is not scanned. Libraries are not
// scanned just because the server starts. Schedule returns when ctx is done.
func (s *Service) Schedule(ctx context.Context) {
	if _, err := s.ReanalysePDFsOnce(ctx); err != nil {
		slog.Error("reanalyse PDFs", "err", err)
	}
	s.queueDeletes(ctx)
	s.queueDue(ctx)
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.queueDue(ctx)
		}
	}
}

// queueDue queues a scan for each library that is due (see Schedule).
func (s *Service) queueDue(ctx context.Context) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, scan_interval, last_scan FROM libraries WHERE deleting = 0`)
	if err != nil {
		slog.Error("schedule", "err", err)
		return
	}
	var ids []string
	for rows.Next() {
		var id, interval string
		var last sql.NullString
		if err := rows.Scan(&id, &interval, &last); err != nil {
			continue
		}
		every, ok := intervals[interval]
		switch {
		case !ok:
		case !last.Valid:
			ids = append(ids, id)
		default:
			t, err := time.Parse(time.RFC3339Nano, last.String)
			if err != nil || time.Since(t) >= every {
				ids = append(ids, id)
			}
		}
	}
	rows.Close()
	slices.Sort(ids)
	for _, id := range ids {
		if _, err := s.QueueScan(ctx, id); err != nil {
			slog.Error("schedule scan", "library", id, "err", err)
		}
	}
}

// PauseScans waits for every running scan sync to finish and keeps new ones
// from starting, in any library, until the returned func is called. A scan
// commits in batches and remembers the rows it read, so a hard delete in
// between (emptying the trash) would make its later updates miss rows that
// are gone. When ctx ends first it gives up and holds nothing.
func (s *Service) PauseScans(ctx context.Context) (func(), error) {
	return s.scans.pause(ctx)
}
