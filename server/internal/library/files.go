package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/media"
	"github.com/tabunugoku/cominavi/server/internal/task"
)

// ErrNotReady means the book has not been analysed (or its analysis
// failed), so it has no page list to make a thumbnail from.
var ErrNotReady = errors.New("library: book not analysed")

// ErrBadPage means a cover page outside 1..page count.
var ErrBadPage = errors.New("library: no such page")

// writeThumb stores jpg as the book's thumbnail file and returns its name.
// The temporary file is unique, so an analysis and a regeneration of the
// same book can run at once; the last rename wins.
func (s *Service) writeThumb(bookID string, jpg []byte) (string, error) {
	if err := os.MkdirAll(s.ThumbDir, 0o755); err != nil {
		return "", err
	}
	name := bookID + ".jpg"
	f, err := os.CreateTemp(s.ThumbDir, name+".*.tmp")
	if err != nil {
		return "", err
	}
	_, err = f.Write(jpg)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), filepath.Join(s.ThumbDir, name))
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return name, nil
}

// RegenerateThumbnail makes the book's thumbnail again from its cover page.
func (s *Service) RegenerateThumbnail(ctx context.Context, bookID string) error {
	var page int
	err := s.DB.QueryRowContext(ctx, `SELECT cover_page FROM books WHERE id = ? AND deleted_at IS NULL`, bookID).Scan(&page)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return s.thumbFromPage(ctx, bookID, page)
}

// SetCover makes page n the book's cover: the thumbnail is made from it now,
// and later analyses keep using it. The choice is stored only once the
// thumbnail was made.
func (s *Service) SetCover(ctx context.Context, bookID string, n int) error {
	if err := s.thumbFromPage(ctx, bookID, n); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE books SET cover_page = ?, updated_at = ? WHERE id = ?`,
		n, time.Now().UTC().Format(time.RFC3339Nano), bookID)
	return err
}

func (s *Service) thumbFromPage(ctx context.Context, bookID string, n int) error {
	var root, rel, status string
	var count int
	err := s.DB.QueryRowContext(ctx, `
		SELECT l.root, b.path, b.media_status, b.page_count FROM books b JOIN libraries l ON l.id = b.library_id
		WHERE b.id = ? AND b.deleted_at IS NULL`, bookID).Scan(&root, &rel, &status, &count)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "ready" {
		return ErrNotReady
	}
	if n < 1 || n > count {
		return ErrBadPage
	}
	var entry string
	if err := s.DB.QueryRowContext(ctx, `SELECT entry FROM pages WHERE book_id = ? AND number = ?`, bookID, n).Scan(&entry); errors.Is(err, sql.ErrNoRows) {
		return ErrBadPage
	} else if err != nil {
		return err
	}
	full, err := resolve(root, rel)
	if err != nil {
		return err
	}
	img, err := media.ReadEntry(ctx, full, entry)
	if err != nil {
		return err
	}
	jpg, err := media.Thumbnail(img, thumbW, thumbH)
	if err != nil {
		return err
	}
	name, err := s.writeThumb(bookID, jpg)
	if err != nil {
		return err
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE books SET thumbnail = ? WHERE id = ? AND thumbnail != ?`, name, bookID, name); err != nil {
		return err
	}
	// The file may be new under the same name: tell open screens either way.
	var libID, seriesID string
	if err := s.DB.QueryRowContext(ctx, `SELECT library_id, series_id FROM books WHERE id = ?`, bookID).Scan(&libID, &seriesID); err == nil {
		s.bookChanged(libID, seriesID, bookID, status)
	}
	return nil
}

// QueueThumbnails queues regenerating the thumbnail of every analysed book
// of a library, or of one series of it when seriesID is set. A book that
// fails is logged and counted; the task fails at the end if any did.
func (s *Service) QueueThumbnails(ctx context.Context, libraryID, seriesID string) (task.Task, error) {
	l, err := s.load(ctx, libraryID)
	if err != nil {
		return task.Task{}, err
	}
	key, label, where, arg := "thumbnail", l.name+" の表紙を作り直す", `library_id = ?`, l.id
	if seriesID != "" {
		var title string
		err := s.DB.QueryRowContext(ctx, `SELECT title FROM series WHERE id = ? AND library_id = ? AND deleted_at IS NULL`, seriesID, l.id).Scan(&title)
		if errors.Is(err, sql.ErrNoRows) {
			return task.Task{}, ErrNotFound
		}
		if err != nil {
			return task.Task{}, err
		}
		// Its own key: thumbnails of series B are not folded into series A's.
		key, label, where, arg = "thumbnail:"+seriesID, title+" の表紙を作り直す", `series_id = ?`, seriesID
	}
	return s.Queue.SubmitKey(ctx, "thumbnail", key, label, l.id, func(ctx context.Context, progress func(float64)) error {
		rows, err := s.DB.QueryContext(ctx,
			`SELECT id FROM books WHERE `+where+` AND deleted_at IS NULL AND media_status = 'ready' ORDER BY path`, arg)
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
		if err := rows.Err(); err != nil {
			return err
		}
		failed := 0
		for i, id := range todo {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := s.RegenerateThumbnail(ctx, id); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				failed++
				slog.Warn("thumbnail", "book", id, "err", err)
			}
			progress(float64(i+1) / float64(len(todo)))
		}
		if failed > 0 {
			return fmt.Errorf("%d 冊の表紙を作れませんでした", failed)
		}
		return nil
	})
}

// BookFile is a book's file opened for download.
type BookFile struct {
	*os.File
	Info os.FileInfo
}

// OpenBookFile opens a live book's file inside its library root. It goes
// through os.Root, so nothing outside the root is reachable, symlinks
// included. The caller decides first whether the requester may have it.
func (s *Service) OpenBookFile(ctx context.Context, bookID string) (BookFile, error) {
	var root, rel string
	err := s.DB.QueryRowContext(ctx, `
		SELECT l.root, b.path FROM books b JOIN libraries l ON l.id = b.library_id
		WHERE b.id = ? AND b.deleted_at IS NULL`, bookID).Scan(&root, &rel)
	if errors.Is(err, sql.ErrNoRows) {
		return BookFile{}, ErrNotFound
	}
	if err != nil {
		return BookFile{}, err
	}
	p, err := resolve(root, rel)
	if err != nil {
		return BookFile{}, err
	}
	r, err := os.OpenRoot(p.Root)
	if err != nil {
		return BookFile{}, err
	}
	defer r.Close() // an open file outlives its root
	f, err := r.Open(filepath.FromSlash(p.Rel))
	if errors.Is(err, os.ErrNotExist) {
		return BookFile{}, ErrNotFound
	}
	if err != nil {
		return BookFile{}, err
	}
	st, err := f.Stat()
	if err == nil && !st.Mode().IsRegular() {
		err = ErrNotFound
	}
	if err != nil {
		f.Close()
		return BookFile{}, err
	}
	return BookFile{File: f, Info: st}, nil
}
