package scan

import (
	"context"
	"database/sql"
	"fmt"
	"path"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/ids"
)

// Stats counts what one Sync changed.
type Stats struct {
	SeriesAdded, SeriesRemoved             int
	BooksAdded, BooksChanged, BooksRemoved int
}

// DirectionFor is the reading direction a new series gets from its
// library's kind, before any ComicInfo.xml says otherwise.
func DirectionFor(kind string) string {
	switch kind {
	case "manga":
		return "rtl"
	case "webtoon":
		return "webtoon"
	}
	return "ltr"
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// Sync makes the database match what Walk found for one library, in one
// transaction:
//   - new folders and root files become series; new files become books
//     (media_status 'outdated', so analysis picks them up);
//   - a book whose size or mtime changed goes back to 'outdated';
//   - rows whose files are gone get deleted_at (Komga's trash), and come
//     back if the file reappears at the same path; rows under a path the
//     walk could not read are left alone;
//   - books with no number in their name are numbered by position, and
//     renumbered when books around them come and go;
//   - the search index for the library is rebuilt from the live rows.
//
// Metadata from ComicInfo.xml is applied later, by analysis.
func Sync(ctx context.Context, db *sql.DB, libraryID, kind string, listing Listing, now time.Time) (Stats, error) {
	var st Stats
	found := listing.Series
	unreachable := func(rel string) bool {
		for _, p := range listing.Unreadable {
			if under(rel, p) {
				return true
			}
		}
		return false
	}
	ts := stamp(now)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return st, err
	}
	defer tx.Rollback()

	type seriesRow struct {
		id, dir, oneshot string
		deleted, seen    bool
	}
	series := map[string]*seriesRow{}
	rows, err := tx.QueryContext(ctx, `SELECT id, dir, oneshot_file, deleted_at IS NOT NULL FROM series WHERE library_id = ?`, libraryID)
	if err != nil {
		return st, err
	}
	for rows.Next() {
		var r seriesRow
		if err := rows.Scan(&r.id, &r.dir, &r.oneshot, &r.deleted); err != nil {
			rows.Close()
			return st, err
		}
		series[r.dir+"\x00"+r.oneshot] = &r
	}
	rows.Close()

	type bookRow struct {
		id, seriesID, mtime, path, source string
		size                              int64
		number                            float64
		deleted, seen                     bool
	}
	books := map[string]*bookRow{}
	rows, err = tx.QueryContext(ctx, `SELECT id, series_id, path, size_bytes, mtime, number, number_source, deleted_at IS NOT NULL FROM books WHERE library_id = ?`, libraryID)
	if err != nil {
		return st, err
	}
	for rows.Next() {
		var r bookRow
		if err := rows.Scan(&r.id, &r.seriesID, &r.path, &r.size, &r.mtime, &r.number, &r.source, &r.deleted); err != nil {
			rows.Close()
			return st, err
		}
		books[r.path] = &r
	}
	rows.Close()

	for _, fs := range found {
		key := fs.Dir + "\x00" + fs.Oneshot
		s := series[key]
		switch {
		case s == nil:
			title := SeriesTitle(path.Base(fs.Dir))
			if fs.Oneshot != "" {
				title = BookTitle(path.Base(fs.Oneshot))
			}
			s = &seriesRow{id: ids.New()}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO series (id, library_id, dir, oneshot_file, title, title_sort, reading_direction, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				s.id, libraryID, fs.Dir, fs.Oneshot, title, SortTitle(title), DirectionFor(kind), ts, ts); err != nil {
				return st, fmt.Errorf("insert series %q: %w", fs.Dir+fs.Oneshot, err)
			}
			series[key] = s
			st.SeriesAdded++
		case s.deleted:
			if _, err := tx.ExecContext(ctx, `UPDATE series SET deleted_at = NULL, updated_at = ? WHERE id = ?`, ts, s.id); err != nil {
				return st, err
			}
		}
		s.seen = true

		for i, fb := range fs.Books {
			mtime := stamp(fb.MTime)
			b := books[fb.Rel]
			if b == nil {
				name := path.Base(fb.Rel)
				num, ok := ParseNumber(name)
				source := "file"
				if !ok {
					num, source = float64(i+1), "position"
				}
				id := ids.New()
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO books (id, series_id, library_id, path, format, size_bytes, mtime, title, number, number_sort, number_source, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					id, s.id, libraryID, fb.Rel, string(fb.Format), fb.Size, mtime, BookTitle(name), num, num, source, ts, ts); err != nil {
					return st, fmt.Errorf("insert book %q: %w", fb.Rel, err)
				}
				books[fb.Rel] = &bookRow{id: id, seen: true}
				st.BooksAdded++
				continue
			}
			b.seen = true
			if b.source == "position" && b.number != float64(i+1) {
				if _, err := tx.ExecContext(ctx, `UPDATE books SET number = ?, number_sort = ? WHERE id = ?`, i+1, i+1, b.id); err != nil {
					return st, err
				}
			}
			changed := b.size != fb.Size || b.mtime != mtime
			if !changed && !b.deleted && b.seriesID == s.id {
				continue
			}
			status := "media_status"
			if changed {
				status = "'outdated'"
				st.BooksChanged++
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE books SET series_id = ?, size_bytes = ?, mtime = ?, deleted_at = NULL,
				  media_status = `+status+`, updated_at = ? WHERE id = ?`,
				s.id, fb.Size, mtime, ts, b.id); err != nil {
				return st, err
			}
		}
	}

	for _, b := range books {
		if !b.seen && !b.deleted && !unreachable(b.path) {
			if _, err := tx.ExecContext(ctx, `UPDATE books SET deleted_at = ? WHERE id = ?`, ts, b.id); err != nil {
				return st, err
			}
			st.BooksRemoved++
		}
	}
	for _, s := range series {
		if !s.seen && !s.deleted && !unreachable(s.dir+s.oneshot) {
			if _, err := tx.ExecContext(ctx, `UPDATE series SET deleted_at = ? WHERE id = ?`, ts, s.id); err != nil {
				return st, err
			}
			st.SeriesRemoved++
		}
	}

	if err := RebuildSearch(ctx, tx, libraryID); err != nil {
		return st, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE libraries SET last_scan = ? WHERE id = ?`, ts, libraryID); err != nil {
		return st, err
	}
	return st, tx.Commit()
}

// RebuildSearch replaces the library's search rows with its live series and
// books (title plus author names).
func RebuildSearch(ctx context.Context, tx *sql.Tx, libraryID string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM search_index WHERE library_id = ?`, libraryID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO search_index (kind, ref, library_id, text)
		SELECT 'series', s.id, s.library_id,
		  s.title || ' ' || s.alternate_titles || ' ' || COALESCE((SELECT group_concat(json_extract(a.value, '$.name'), ' ') FROM json_each(s.authors) a), '')
		FROM series s WHERE s.library_id = ? AND s.deleted_at IS NULL`, libraryID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO search_index (kind, ref, library_id, text)
		SELECT 'book', b.id, b.library_id,
		  b.title || ' ' || COALESCE((SELECT group_concat(json_extract(a.value, '$.name'), ' ') FROM json_each(b.authors) a), '')
		FROM books b WHERE b.library_id = ? AND b.deleted_at IS NULL`, libraryID)
	return err
}
