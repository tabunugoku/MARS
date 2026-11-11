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
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/archive"
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
	l, err := s.load(ctx, libraryID)
	if err != nil {
		return task.Task{}, err
	}
	return s.Queue.Submit(ctx, "scan", l.name+" をスキャン", l.id, func(ctx context.Context, progress func(float64)) error {
		l, err := s.load(ctx, libraryID) // settings may have changed while queued
		if err != nil {
			return err
		}
		found, err := scan.Walk(ctx, l.root, l.exclude)
		if err != nil {
			return fmt.Errorf("フォルダを読めません: %w", err)
		}
		progress(0.5)
		st, err := scan.Sync(ctx, s.DB, l.id, l.kind, found, time.Now())
		if err != nil {
			return err
		}
		slog.Info("scan", "library", l.name, "stats", st)
		_, err = s.QueueAnalyze(context.WithoutCancel(ctx), l.id)
		return err
	})
}

// QueueAnalyze queues the analysis of every live book still 'outdated'.
func (s *Service) QueueAnalyze(ctx context.Context, libraryID string) (task.Task, error) {
	l, err := s.load(ctx, libraryID)
	if err != nil {
		return task.Task{}, err
	}
	return s.Queue.Submit(ctx, "analyze", l.name+" の解析", l.id, func(ctx context.Context, progress func(float64)) error {
		l, err := s.load(ctx, libraryID)
		if err != nil {
			return err
		}
		rows, err := s.DB.QueryContext(ctx,
			`SELECT id FROM books WHERE library_id = ? AND deleted_at IS NULL AND media_status = 'outdated' ORDER BY path`, l.id)
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
		for i, id := range todo {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := s.analyzeBook(ctx, l, id); err != nil {
				return err // only database errors stop the task; bad files are recorded per book
			}
			progress(float64(i+1) / float64(len(todo)))
		}
		if len(todo) > 0 {
			tx, err := s.DB.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			defer tx.Rollback()
			if err := scan.RebuildSearch(ctx, tx, l.id); err != nil {
				return err
			}
			return tx.Commit()
		}
		return nil
	})
}

// analyzeBook reads one book and stores pages, thumbnail and metadata. A
// broken or unsupported file is recorded on the book, not returned.
func (s *Service) analyzeBook(ctx context.Context, l lib, bookID string) error {
	var rel, seriesID, lockedJSON string
	if err := s.DB.QueryRowContext(ctx, `SELECT path, series_id, locked_fields FROM books WHERE id = ?`, bookID).
		Scan(&rel, &seriesID, &lockedJSON); err != nil {
		return err
	}
	full, err := resolve(l.root, rel)
	if err != nil {
		return s.markBook(ctx, bookID, "error", err.Error())
	}
	a, err := media.Analyze(full)
	switch {
	case errors.Is(err, archive.ErrUnsupported):
		return s.markBook(ctx, bookID, "unsupported", err.Error())
	case err != nil:
		return s.markBook(ctx, bookID, "error", err.Error())
	}

	thumb := ""
	if jpg, err := media.Thumbnail(a.Cover, thumbW, thumbH); err == nil {
		if err := os.MkdirAll(s.ThumbDir, 0o755); err != nil {
			return err
		}
		name := bookID + ".jpg"
		tmp := filepath.Join(s.ThumbDir, name+".tmp")
		if err := os.WriteFile(tmp, jpg, 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, filepath.Join(s.ThumbDir, name)); err != nil {
			return err
		}
		thumb = name
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
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM pages WHERE book_id = ?`, bookID); err != nil {
		return err
	}
	for _, p := range a.Pages {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO pages (book_id, number, entry, media_type, width, height) VALUES (?, ?, ?, ?, ?, ?)`,
			bookID, p.Number, p.Entry, p.MediaType, p.Width, p.Height); err != nil {
			return err
		}
	}
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx,
		`UPDATE books SET page_count = ?, media_status = 'ready', media_error = '', thumbnail = ?, updated_at = ? WHERE id = ?`,
		len(a.Pages), thumb, ts, bookID); err != nil {
		return err
	}
	if ci != nil {
		if err := applyBookInfo(ctx, tx, bookID, lockedJSON, *ci, ts); err != nil {
			return err
		}
		if err := applySeriesInfo(ctx, tx, seriesID, bookID, *ci, ts); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) markBook(ctx context.Context, bookID, status, msg string) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE books SET media_status = ?, media_error = ?, page_count = 0, updated_at = ? WHERE id = ?`,
		status, msg, time.Now().UTC().Format(time.RFC3339Nano), bookID)
	return err
}

// resolve joins a stored relative path onto the library root and refuses
// anything that would land outside it.
func resolve(root, rel string) (string, error) {
	if !filepath.IsLocal(filepath.FromSlash(rel)) {
		return "", fmt.Errorf("不正なパス: %q", rel)
	}
	return filepath.Join(root, filepath.FromSlash(rel)), nil
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
func applySeriesInfo(ctx context.Context, tx *sql.Tx, seriesID, bookID string, ci scan.ComicInfo, ts string) error {
	var first, lockedJSON string
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM books WHERE series_id = ? AND deleted_at IS NULL ORDER BY number_sort, path LIMIT 1`, seriesID).Scan(&first); err != nil {
		return err
	}
	if first != bookID {
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
		add("title", "title_sort", scan.SortTitle(ci.Series))
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
	_, err := tx.ExecContext(ctx, `UPDATE series SET `+strings.Join(set, ", ")+`, updated_at = ? WHERE id = ?`, args...)
	return err
}

// Page is a page ready to serve.
type Page struct {
	Body      io.ReadSeeker
	MediaType string
	ETag      string // strong; changes when the file changes
	Close     func() error
}

// OpenPage resolves page n (1-based) of a ready book. Zip pages are read
// straight from the archive; other formats go through the page cache.
func (s *Service) OpenPage(ctx context.Context, bookID string, n int) (Page, error) {
	var root, rel, entry, mediaType, mtime string
	var size int64
	err := s.DB.QueryRowContext(ctx, `
		SELECT l.root, b.path, b.size_bytes, b.mtime, p.entry, p.media_type
		FROM books b JOIN libraries l ON l.id = b.library_id JOIN pages p ON p.book_id = b.id
		WHERE b.id = ? AND p.number = ? AND b.deleted_at IS NULL AND b.media_status = 'ready'`, bookID, n).
		Scan(&root, &rel, &size, &mtime, &entry, &mediaType)
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
	key := fmt.Sprintf("%d-%s", size, mtime)
	etag := fmt.Sprintf(`"%s-%d-%x"`, bookID, n, hash(key))

	a, _, err := archive.Open(full)
	if err != nil {
		return Page{}, err
	}
	if a.RandomAccess() {
		defer a.Close()
		rc, err := a.Open(entry)
		if err != nil {
			return Page{}, err
		}
		defer rc.Close()
		b, err := io.ReadAll(io.LimitReader(rc, media.MaxEntryBytes+1))
		if err != nil {
			return Page{}, err
		}
		if len(b) > media.MaxEntryBytes {
			return Page{}, media.ErrTooLarge
		}
		return Page{Body: bytes.NewReader(b), MediaType: mediaType, ETag: etag, Close: func() error { return nil }}, nil
	}
	a.Close()

	pages, err := s.pages(ctx, bookID)
	if err != nil {
		return Page{}, err
	}
	p, err := s.Pages.Page(bookID, key, full, pages, n)
	if err != nil {
		return Page{}, err
	}
	f, err := os.Open(p)
	if err != nil {
		return Page{}, err
	}
	return Page{Body: f, MediaType: mediaType, ETag: etag, Close: f.Close}, nil
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
