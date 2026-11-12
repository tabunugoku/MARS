package scan

import (
	"context"
	"database/sql"
	"fmt"
	"path"
	"time"

	cdb "github.com/tabunugoku/cominavi/server/internal/db"
	"github.com/tabunugoku/cominavi/server/internal/ids"
)

// Stats counts what one Sync changed.
type Stats struct {
	SeriesAdded, SeriesRemoved             int
	BooksAdded, BooksChanged, BooksRemoved int
	// Restored counts series and unchanged books brought back from the
	// trash, and unchanged books that moved to another series: rows that
	// reappear in lists although nothing needs analysing.
	Restored int
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

// syncBatch is the most writes Sync puts in one transaction (0: the
// db.Batch default). Tests set it to 1 to cross a commit after every write.
var syncBatch = 0

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// Sync makes the database match what Walk found for one library:
//   - new folders and root files become series; new files become books
//     (media_status 'outdated', so analysis picks them up);
//   - a book whose size or mtime changed goes back to 'outdated';
//   - rows whose files are gone get deleted_at (Komga's trash), and come
//     back if the file reappears at the same path; rows under a path the
//     walk could not read, and rows outside a scoped listing's scope, are left
//     alone;
//   - books with no number in their name are numbered by position, and
//     renumbered when books around them come and go;
//   - the library's search rows are brought up to date.
//
// The current rows are read outside any transaction and the changes are
// written in short transactions (db.Batch), so a reader saving progress
// never waits long behind a scan. Each write is safe to repeat, so a Sync
// that stops part way leaves a state the next one finishes. Only one Sync
// per library runs at a time (the caller holds a lock), so nothing else
// inserts series or books meanwhile.
//
// Metadata from ComicInfo.xml is applied later, by analysis.
func Sync(ctx context.Context, db *sql.DB, libraryID, kind string, listing Listing, now time.Time) (Stats, error) {
	var st Stats
	found := listing.Series
	// Rows outside a scoped listing's scope were not looked for, so they are
	// neither trashed nor restored; rows under an unreadable path are kept too.
	unreachable := func(rel string) bool {
		if !InScope(rel, listing.Scope) {
			return true
		}
		for _, p := range listing.Unreadable {
			if under(rel, p) {
				return true
			}
		}
		return false
	}
	ts := stamp(now)
	b := &cdb.Batch{DB: db, MaxStatements: syncBatch}
	defer b.Close()
	exec := func(query string, args ...any) error {
		if _, err := b.Exec(ctx, query, args...); err != nil {
			return err
		}
		return b.Step(ctx)
	}

	type seriesRow struct {
		id, dir, oneshot string
		deleted, seen    bool
	}
	series := map[string]*seriesRow{}
	rows, err := db.QueryContext(ctx, `SELECT id, dir, oneshot_file, deleted_at IS NOT NULL FROM series WHERE library_id = ?`, libraryID)
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
	if err := rows.Err(); err != nil {
		return st, err
	}

	type bookRow struct {
		id, seriesID, mtime, path, source string
		size                              int64
		number                            float64
		deleted, seen                     bool
	}
	books := map[string]*bookRow{}
	rows, err = db.QueryContext(ctx, `SELECT id, series_id, path, size_bytes, mtime, number,
		  -- a number locked by an edit is never renumbered by position
		  CASE WHEN EXISTS (SELECT 1 FROM json_each(locked_fields) WHERE value = 'number') THEN 'locked' ELSE number_source END,
		  deleted_at IS NOT NULL
		FROM books WHERE library_id = ?`, libraryID)
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
	if err := rows.Err(); err != nil {
		return st, err
	}

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
			if err := exec(`
				INSERT INTO series (id, library_id, dir, oneshot_file, title, title_sort, reading_direction, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				s.id, libraryID, fs.Dir, fs.Oneshot, title, SortTitle(title), DirectionFor(kind), ts, ts); err != nil {
				return st, fmt.Errorf("insert series %q: %w", fs.Dir+fs.Oneshot, err)
			}
			series[key] = s
			st.SeriesAdded++
		case s.deleted:
			if err := exec(`UPDATE series SET deleted_at = NULL, updated_at = ? WHERE id = ?`, ts, s.id); err != nil {
				return st, err
			}
			st.Restored++
		}
		s.seen = true

		for i, fb := range fs.Books {
			mtime := stamp(fb.MTime)
			bk := books[fb.Rel]
			if bk == nil {
				name := path.Base(fb.Rel)
				num, ok := ParseNumber(name)
				source := "file"
				if !ok {
					num, source = float64(i+1), "position"
				}
				id := ids.New()
				if err := exec(`
					INSERT INTO books (id, series_id, library_id, path, format, size_bytes, mtime, title, number, number_sort, number_source, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					id, s.id, libraryID, fb.Rel, string(fb.Format), fb.Size, mtime, BookTitle(name), num, num, source, ts, ts); err != nil {
					return st, fmt.Errorf("insert book %q: %w", fb.Rel, err)
				}
				books[fb.Rel] = &bookRow{id: id, seen: true}
				st.BooksAdded++
				continue
			}
			bk.seen = true
			if bk.source == "position" && bk.number != float64(i+1) {
				// Checked again in SQL: an edit may have locked the number
				// since the rows were read.
				if err := exec(`UPDATE books SET number = ?, number_sort = ? WHERE id = ? AND number_source = 'position'
					AND NOT EXISTS (SELECT 1 FROM json_each(locked_fields) WHERE value = 'number')`, i+1, i+1, bk.id); err != nil {
					return st, err
				}
			}
			changed := bk.size != fb.Size || bk.mtime != mtime
			if !changed && !bk.deleted && bk.seriesID == s.id {
				continue
			}
			status := "media_status"
			if changed {
				status = "'outdated'"
				st.BooksChanged++
			} else {
				st.Restored++
			}
			if err := exec(`
				UPDATE books SET series_id = ?, size_bytes = ?, mtime = ?, deleted_at = NULL,
				  media_status = `+status+`, updated_at = ? WHERE id = ?`,
				s.id, fb.Size, mtime, ts, bk.id); err != nil {
				return st, err
			}
		}
	}

	for _, bk := range books {
		if !bk.seen && !bk.deleted && !unreachable(bk.path) {
			if err := exec(`UPDATE books SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`, ts, bk.id); err != nil {
				return st, err
			}
			st.BooksRemoved++
		}
	}
	for _, s := range series {
		if !s.seen && !s.deleted && !unreachable(s.dir+s.oneshot) {
			if err := exec(`UPDATE series SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`, ts, s.id); err != nil {
				return st, err
			}
			st.SeriesRemoved++
		}
	}

	// The search comparison reads committed rows.
	if err := b.Flush(); err != nil {
		return st, err
	}
	if err := SyncSearch(ctx, db, b, libraryID); err != nil {
		return st, err
	}
	// A scoped sync is not a full scan: the periodic schedule keeps its view
	// of the last full one.
	if len(listing.Scope) == 0 {
		if _, err := b.Exec(ctx, `UPDATE libraries SET last_scan = ? WHERE id = ?`, ts, libraryID); err != nil {
			return st, err
		}
	}
	return st, b.Flush()
}
