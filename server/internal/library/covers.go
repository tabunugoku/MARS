package library

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/media"
)

// SeriesCoverSQL is the SQL expression for the file name of a series' cover
// (empty when it has none), over the alias s: the uploaded or local file when
// there is one, else the thumbnail of the cover book when that book is live
// and has one, else the first live book that has one. Every reader of a
// series cover uses it, so the URL version and the bytes always agree.
const SeriesCoverSQL = `CASE WHEN s.thumbnail != '' THEN s.thumbnail ELSE COALESCE(
	(SELECT cb.thumbnail FROM books cb WHERE cb.id = s.cover_book_id AND cb.deleted_at IS NULL AND cb.thumbnail != ''),
	(SELECT fb.thumbnail FROM books fb WHERE fb.series_id = s.id AND fb.deleted_at IS NULL AND fb.thumbnail != ''
	 ORDER BY fb.number_sort, fb.path LIMIT 1), '') END`

// ThumbVersion is the token a thumbnail URL carries (?v=) for a file name.
// Every stored thumbnail has a fresh name, so the token changes with the image.
func ThumbVersion(name string) string { return fmt.Sprintf("%x", hash(name)) }

// ErrImage means uploaded or local bytes are not an image a thumbnail can be
// made from. Other errors are server errors.
var ErrImage = errors.New("library: image")

// newThumbName is a file name under ThumbDir that no earlier thumbnail had.
func newThumbName(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Never expected; the clock still makes the name fresh.
		return fmt.Sprintf("%s-%x.jpg", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b[:]) + ".jpg"
}

// removeThumb deletes a thumbnail file that no row refers to any more. An
// error is ignored: a file left behind is harmless.
func (s *Service) removeThumb(name string) {
	if name != "" {
		_ = os.Remove(filepath.Join(s.ThumbDir, filepath.Base(name)))
	}
}

// seriesArt is what the cover logic needs of a live series.
type seriesArt struct {
	id, libraryID, dir, oneshot, thumb, source, sig string
}

func seriesCoverKey(id string) string { return "series:" + id }

func (s *Service) loadSeriesArt(ctx context.Context, id string) (seriesArt, error) {
	var a seriesArt
	err := s.DB.QueryRowContext(ctx, `SELECT id, library_id, dir, oneshot_file, thumbnail, thumbnail_source, artwork_sig
		FROM series WHERE id = ? AND deleted_at IS NULL`, id).
		Scan(&a.id, &a.libraryID, &a.dir, &a.oneshot, &a.thumb, &a.source, &a.sig)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// SetSeriesUpload makes img (JPEG, PNG or WebP bytes) the series cover. It
// replaces an earlier upload or local artwork file.
func (s *Service) SetSeriesUpload(ctx context.Context, seriesID string, img []byte) error {
	mu := s.coverLock(seriesCoverKey(seriesID))
	if err := mu.Lock(ctx); err != nil {
		return err
	}
	defer mu.Unlock()
	sr, err := s.loadSeriesArt(ctx, seriesID)
	if err != nil {
		return err
	}
	w, h := s.thumbBounds(ctx)
	jpg, err := media.Thumbnail(img, w, h)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrImage, err)
	}
	name, err := s.writeThumb("series-"+seriesID, jpg)
	if err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE series SET thumbnail = ?, thumbnail_source = 'upload', artwork_sig = '', updated_at = ?
		WHERE id = ? AND deleted_at IS NULL`, name, time.Now().UTC().Format(time.RFC3339Nano), seriesID); err != nil {
		s.removeThumb(name)
		return err
	}
	s.removeThumb(sr.thumb)
	return nil
}

// DropSeriesUpload removes an uploaded series cover. Local artwork is picked
// up again at once when the library still has it; otherwise the cover book's
// thumbnail shows. A series whose cover is not an upload is left as it is.
func (s *Service) DropSeriesUpload(ctx context.Context, seriesID string) error {
	mu := s.coverLock(seriesCoverKey(seriesID))
	if err := mu.Lock(ctx); err != nil {
		return err
	}
	defer mu.Unlock()
	sr, err := s.loadSeriesArt(ctx, seriesID)
	if err != nil {
		return err
	}
	if sr.source != "upload" {
		return nil
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE series SET thumbnail = '', thumbnail_source = '', artwork_sig = '', updated_at = ?
		WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), seriesID); err != nil {
		return err
	}
	s.removeThumb(sr.thumb)
	sr.thumb, sr.source, sr.sig = "", "", ""
	l, err := s.load(ctx, sr.libraryID)
	if err != nil {
		return err
	}
	return s.syncSeriesArt(ctx, l, sr)
}

// SetBookUpload makes img the book's thumbnail, until it is regenerated or its
// cover page is chosen again.
func (s *Service) SetBookUpload(ctx context.Context, bookID string, img []byte) error {
	mu := s.coverLock(bookID)
	if err := mu.Lock(ctx); err != nil {
		return err
	}
	defer mu.Unlock()
	var libID, seriesID, status, old string
	err := s.DB.QueryRowContext(ctx, `SELECT library_id, series_id, media_status, thumbnail FROM books
		WHERE id = ? AND deleted_at IS NULL`, bookID).Scan(&libID, &seriesID, &status, &old)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	w, h := s.thumbBounds(ctx)
	jpg, err := media.Thumbnail(img, w, h)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrImage, err)
	}
	name, err := s.writeThumb(bookID, jpg)
	if err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE books SET thumbnail = ?, thumbnail_uploaded = 1, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL`, name, time.Now().UTC().Format(time.RFC3339Nano), bookID); err != nil {
		s.removeThumb(name)
		return err
	}
	s.removeThumb(old)
	s.bookChanged(libID, seriesID, bookID, status)
	return nil
}

// syncArtwork looks at every live series of the library for local artwork.
func (s *Service) syncArtwork(ctx context.Context, l lib) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM series WHERE library_id = ? AND deleted_at IS NULL`, l.id)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.syncSeriesArtwork(ctx, l, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) syncSeriesArtwork(ctx context.Context, l lib, id string) error {
	mu := s.coverLock(seriesCoverKey(id))
	if err := mu.Lock(ctx); err != nil {
		return err
	}
	defer mu.Unlock()
	sr, err := s.loadSeriesArt(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil // trashed meanwhile
	}
	if err != nil {
		return err
	}
	return s.syncSeriesArt(ctx, l, sr)
}

// syncSeriesArt makes the series' local artwork match its folder. The caller
// holds the series' cover lock. An upload is never touched.
func (s *Service) syncSeriesArt(ctx context.Context, l lib, sr seriesArt) error {
	if sr.source == "upload" {
		return nil
	}
	if l.importArtwork && sr.dir != "" && sr.oneshot == "" {
		name, info := findArtwork(l.root, sr.dir)
		if name != "" {
			sig := fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano())
			if sr.source == "local" && sr.sig == sig && sr.thumb != "" {
				return nil
			}
			img, err := readArtwork(l.root, filepath.Join(sr.dir, name))
			var jpg []byte
			if err == nil {
				w, h := s.thumbBounds(ctx)
				jpg, err = media.Thumbnail(img, w, h)
			}
			if err == nil {
				thumb, err := s.writeThumb("series-"+sr.id, jpg)
				if err != nil {
					return err
				}
				if _, err := s.DB.ExecContext(ctx, `UPDATE series SET thumbnail = ?, thumbnail_source = 'local', artwork_sig = ?, updated_at = ?
					WHERE id = ? AND deleted_at IS NULL`, thumb, sig, time.Now().UTC().Format(time.RFC3339Nano), sr.id); err != nil {
					s.removeThumb(thumb)
					return err
				}
				s.removeThumb(sr.thumb)
				return nil
			}
			slog.Warn("local artwork", "series", sr.id, "file", name, "err", err)
		}
	}
	if sr.source != "local" {
		return nil
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE series SET thumbnail = '', thumbnail_source = '', artwork_sig = '', updated_at = ?
		WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), sr.id); err != nil {
		return err
	}
	s.removeThumb(sr.thumb)
	return nil
}

// artworkStems and artworkExts give the order in which a series folder's
// cover files are tried.
var (
	artworkStems = []string{"cover", "folder", "poster"}
	artworkExts  = []string{".jpg", ".jpeg", ".png", ".webp"}
)

// findArtwork returns the first cover file in the folder dir (relative to the
// library root; subfolders are not searched), with its info, or "" when there
// is none. Names compare case-insensitively. A folder that cannot be read has
// none.
func findArtwork(root, dir string) (string, fs.FileInfo) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return "", nil
	}
	defer r.Close()
	d, err := r.Open(filepath.FromSlash(dir))
	if err != nil {
		return "", nil
	}
	defer d.Close()
	ents, err := d.ReadDir(-1)
	if err != nil {
		return "", nil
	}
	byName := map[string]fs.DirEntry{}
	for _, e := range ents {
		if e.Type().IsRegular() {
			byName[strings.ToLower(e.Name())] = e
		}
	}
	for _, stem := range artworkStems {
		for _, ext := range artworkExts {
			e, ok := byName[stem+ext]
			if !ok {
				continue
			}
			if info, err := e.Info(); err == nil {
				return e.Name(), info
			}
		}
	}
	return "", nil
}

// readArtwork reads a cover file inside the library root, at most one entry's
// worth of bytes.
func readArtwork(root, rel string) ([]byte, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	f, err := r.Open(rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, ErrNotFound
	}
	if st.Size() > media.MaxEntryBytes {
		return nil, media.ErrTooLarge
	}
	return io.ReadAll(io.LimitReader(f, media.MaxEntryBytes+1))
}
