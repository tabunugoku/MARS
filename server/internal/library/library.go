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
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/archive"
	"github.com/tabunugoku/cominavi/server/internal/db"
	"github.com/tabunugoku/cominavi/server/internal/media"
	"github.com/tabunugoku/cominavi/server/internal/scan"
	"github.com/tabunugoku/cominavi/server/internal/task"
)

// Thumbnail bounds. Fixed for now; the thumbnailSize setting arrives with
// the settings API (M3).
const thumbW, thumbH = 400, 600

var ErrNotFound = errors.New("library: not found")

type Service struct {
	DB       *sql.DB
	Queue    *task.Queue
	ThumbDir string
	Pages    *media.PageCache

	scanLocks sync.Map // library id -> *sync.Mutex; one walk+sync at a time per library
}

func (s *Service) scanLock(id string) *sync.Mutex {
	m, _ := s.scanLocks.LoadOrStore(id, &sync.Mutex{})
	return m.(*sync.Mutex)
}

type lib struct {
	id, name, root, kind, interval string
	importComicInfo                bool
	exclude                        []string
	lastScan                       sql.NullString
}

func (s *Service) load(ctx context.Context, id string) (lib, error) {
	var l lib
	var excl string
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, name, root, kind, scan_interval, import_comicinfo, exclude, last_scan FROM libraries WHERE id = ?`, id).
		Scan(&l.id, &l.name, &l.root, &l.kind, &l.interval, &l.importComicInfo, &excl, &l.lastScan)
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
	key, label := "scan", l.name+" をスキャン"
	if deep {
		key, label = "scan:deep", l.name+" を詳細スキャン"
	}
	return s.Queue.SubmitKey(ctx, "scan", key, label, l.id, func(ctx context.Context, progress func(float64)) error {
		l, err := s.load(ctx, libraryID) // settings may have changed while queued
		if err != nil {
			return err
		}
		// A running scan is not deduplicated by the queue, so two can overlap;
		// the lock keeps an older walk from being committed after a newer one.
		mu := s.scanLock(l.id)
		mu.Lock()
		found, err := scan.Walk(ctx, l.root, l.exclude)
		if err != nil {
			mu.Unlock()
			return fmt.Errorf("フォルダを読めません: %w", err)
		}
		progress(0.5)
		st, err := scan.Sync(ctx, s.DB, l.id, l.kind, found, time.Now())
		mu.Unlock()
		if err != nil {
			return err
		}
		if len(found.Unreadable) > 0 {
			slog.Warn("scan: unreadable paths kept as they were", "library", l.name, "paths", found.Unreadable)
		}
		slog.Info("scan", "library", l.name, "stats", st)
		_, err = s.queueAnalyze(context.WithoutCancel(ctx), l.id, deep)
		return err
	})
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
	key, label, which := "analyze", l.name+" の解析", ` AND media_status = 'outdated'`
	if deep {
		key, label, which = "analyze:deep", l.name+" の詳細解析", ""
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
		infos := map[string]map[string]scan.ComicInfo{} // series id -> book id -> ComicInfo
		for i, id := range todo {
			if err := ctx.Err(); err != nil {
				return err
			}
			seriesID, ci, err := s.analyzeBook(ctx, l, id)
			if err != nil {
				return err // only database errors stop the task; bad files are recorded per book
			}
			if ci != nil {
				if infos[seriesID] == nil {
					infos[seriesID] = map[string]scan.ComicInfo{}
				}
				infos[seriesID][id] = *ci
			}
			progress(float64(i+1) / float64(len(todo)))
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
		return b.Flush()
	})
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
	var rel, seriesID, lockedJSON string
	var v version
	var coverPage int
	if err := s.DB.QueryRowContext(ctx, `SELECT path, series_id, locked_fields, size_bytes, mtime, cover_page FROM books WHERE id = ?`, bookID).
		Scan(&rel, &seriesID, &lockedJSON, &v.size, &v.mtime, &coverPage); err != nil {
		return "", nil, err
	}
	full, err := resolve(l.root, rel)
	if err != nil {
		return seriesID, nil, s.markBook(ctx, bookID, v, "error", err.Error())
	}
	a, err := media.AnalyzeCover(ctx, full, coverPage)
	switch {
	case errors.Is(err, archive.ErrUnsupported):
		return seriesID, nil, s.markBook(ctx, bookID, v, "unsupported", err.Error())
	case err != nil:
		return seriesID, nil, s.markBook(ctx, bookID, v, "error", err.Error())
	}

	thumb := ""
	if jpg, err := media.Thumbnail(a.Cover, thumbW, thumbH); err == nil {
		if thumb, err = s.writeThumb(bookID, jpg); err != nil {
			return "", nil, err
		}
	} else {
		slog.Warn("thumbnail", "book", bookID, "err", err)
	}

	var ci *scan.ComicInfo
	if l.importComicInfo && a.ComicInfo != nil {
		if c, err := scan.ParseComicInfo(bytes.NewReader(a.ComicInfo)); err == nil {
			ci = &c
		} else {
			slog.Warn("comicinfo", "book", bookID, "err", err)
		}
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback()
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := tx.ExecContext(ctx, `
		UPDATE books SET page_count = ?, media_status = 'ready', media_error = '', thumbnail = ?, cover_page = ?, updated_at = ?
		WHERE id = ? AND size_bytes = ? AND mtime = ? AND deleted_at IS NULL`,
		len(a.Pages), thumb, a.CoverPage, ts, bookID, v.size, v.mtime)
	if err != nil {
		return "", nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
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
	return seriesID, ci, tx.Commit()
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
		return archive.Path{}, fmt.Errorf("不正なパス: %q", rel)
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

// Version is the token page URLs carry (?v=) so a replaced file gets new
// URLs and old cached responses are never reused for it.
func Version(size int64, mtime string) string {
	return fmt.Sprintf("%x", hash(fmt.Sprintf("%d-%s", size, mtime)))
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
	v := Version(size, mtime)
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
		q = `SELECT thumbnail FROM books WHERE series_id = ? AND deleted_at IS NULL AND thumbnail != '' ORDER BY number_sort, path LIMIT 1`
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

// Schedule scans every library once at startup, then each library again
// when its scan interval has passed. It returns when ctx is done.
func (s *Service) Schedule(ctx context.Context) {
	tick := func(startup bool) {
		rows, err := s.DB.QueryContext(ctx, `SELECT id, scan_interval, last_scan FROM libraries`)
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
			case startup:
				ids = append(ids, id)
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
	tick(true)
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			tick(false)
		}
	}
}
