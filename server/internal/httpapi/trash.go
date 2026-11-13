package httpapi

import (
	"context"
	"database/sql"
	"path"
	"slices"
	"time"
)

// purgeCond is a SQL condition over one table, with its arguments. The zero
// value selects nothing.
type purgeCond struct {
	sql  string
	args []any
}

func (c purgeCond) none() bool { return c.sql == "" }

// purgeRows names the rows one purge takes. books and series are deleted
// outright. cascaded is every book that goes, the books of a removed series
// included: their covers and search rows go too.
type purgeRows struct {
	books, series, cascaded purgeCond
}

// purgeSelection is what one purge takes: a trashed series (its books go by
// cascade, so only the series is named), a trashed book, or, when both ids are
// empty, everything in the trash.
func purgeSelection(seriesID, bookID string) purgeRows {
	switch {
	case seriesID != "":
		return purgeRows{
			series:   purgeCond{`id = ? AND deleted_at IS NOT NULL`, []any{seriesID}},
			cascaded: purgeCond{`series_id = ?`, []any{seriesID}},
		}
	case bookID != "":
		b := purgeCond{`id = ? AND deleted_at IS NOT NULL`, []any{bookID}}
		return purgeRows{books: b, cascaded: b}
	}
	return purgeRows{
		books:    purgeCond{`deleted_at IS NOT NULL`, nil},
		series:   purgeCond{`deleted_at IS NOT NULL`, nil},
		cascaded: purgeCond{`deleted_at IS NOT NULL OR series_id IN (SELECT id FROM series WHERE deleted_at IS NOT NULL)`, nil},
	}
}

// purgeTrash hard-deletes the selection (see purgeSelection). found is false
// when the selected series or book is not in the trash; then nothing is removed.
func (s *server) purgeTrash(ctx context.Context, seriesID, bookID string) (TrashResult, bool, error) {
	var res TrashResult
	// A scan restores trashed rows by the ids it read earlier, across
	// several commits: wait for running scans and hold new ones off.
	resume, err := s.Library.PauseScans(ctx)
	if err != nil {
		return res, false, err
	}
	defer resume()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, false, err
	}
	defer tx.Rollback()
	sel := purgeSelection(seriesID, bookID)
	if seriesID != "" || bookID != "" {
		// Checked after the pause, so a scan cannot restore the row first.
		table, named := "series", sel.series
		if bookID != "" {
			table, named = "books", sel.books
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM `+table+` WHERE `+named.sql, named.args...).Scan(&n); err != nil {
			return res, false, err
		}
		if n == 0 {
			return res, false, nil
		}
	}
	// Cover files of every book that goes, including those cascaded away,
	// and of the series that go.
	thumbs, err := thumbNames(ctx, tx, "books", sel.cascaded)
	if err != nil {
		return res, false, err
	}
	seriesThumbs, err := thumbNames(ctx, tx, "series", sel.series)
	if err != nil {
		return res, false, err
	}
	thumbs = append(thumbs, seriesThumbs...)

	var emptied []string
	if !sel.series.none() {
		emptied, err = collectionsLosing(ctx, tx, `SELECT id FROM series WHERE `+sel.series.sql, sel.series.args...)
		if err != nil {
			return res, false, err
		}
	}
	// Search rows of the removed rows go before the rows do, because the
	// subqueries here match the rows that are about to be deleted.
	if !sel.cascaded.none() {
		if _, err := tx.ExecContext(ctx, `DELETE FROM search_rows WHERE kind = 'book' AND ref IN (SELECT id FROM books WHERE `+sel.cascaded.sql+`)`, sel.cascaded.args...); err != nil {
			return res, false, err
		}
	}
	if !sel.series.none() {
		if _, err := tx.ExecContext(ctx, `DELETE FROM search_rows WHERE kind = 'series' AND ref IN (SELECT id FROM series WHERE `+sel.series.sql+`)`, sel.series.args...); err != nil {
			return res, false, err
		}
	}
	if !sel.books.none() {
		if res.Books, err = execCount(ctx, tx, `DELETE FROM books WHERE `+sel.books.sql, sel.books.args...); err != nil {
			return res, false, err
		}
	}
	if !sel.series.none() {
		if res.Series, err = execCount(ctx, tx, `DELETE FROM series WHERE `+sel.series.sql, sel.series.args...); err != nil {
			return res, false, err
		}
	}
	if err := dropEmptied(ctx, tx, emptied); err != nil {
		return res, false, err
	}
	if err := tx.Commit(); err != nil {
		return res, false, err
	}
	// Files go only after the rows are gone. A file left behind is harmless
	// (its name is unique to a removed book), so errors are ignored.
	for _, name := range thumbs {
		s.Library.RemoveThumb(name)
	}
	return res, true, nil
}

// thumbNames returns the non-empty thumbnail names of the rows of table that
// match c. The rows are closed before the caller runs another statement on tx.
func thumbNames(ctx context.Context, tx *sql.Tx, table string, c purgeCond) ([]string, error) {
	if c.none() {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT thumbnail FROM `+table+` WHERE thumbnail != '' AND (`+c.sql+`)`, c.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// trashTime reads a stored deleted_at. A value that does not parse gives the
// zero time, so one bad row does not fail the whole list.
func trashTime(v string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}
	}
	return t
}

// ListTrash lists the trashed series and the trashed books whose series is
// live; a book of a trashed series is counted on its series only. Newest
// deletions come first.
func (s *server) ListTrash(ctx context.Context, _ ListTrashRequestObject) (ListTrashResponseObject, error) {
	series, err := s.trashedSeries(ctx)
	if err != nil {
		return nil, err
	}
	books, err := s.trashedBooks(ctx)
	if err != nil {
		return nil, err
	}
	return ListTrash200JSONResponse(TrashList{Books: books, Series: series}), nil
}

func (s *server) trashedSeries(ctx context.Context) ([]TrashedSeries, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT s.id, s.library_id,
		CASE WHEN s.oneshot_file != '' THEN s.oneshot_file ELSE s.dir END, s.title, s.deleted_at,
		(SELECT count(*) FROM books b WHERE b.series_id = s.id)
		FROM series s WHERE s.deleted_at IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrashedSeries{}
	for rows.Next() {
		var x TrashedSeries
		var deleted string
		if err := rows.Scan(&x.Id, &x.LibraryId, &x.Path, &x.Title, &deleted, &x.BookCount); err != nil {
			return nil, err
		}
		x.DeletedAt = trashTime(deleted)
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Sorted on the parsed time: stored RFC 3339 strings do not sort in order.
	slices.SortStableFunc(out, func(a, b TrashedSeries) int { return b.DeletedAt.Compare(a.DeletedAt) })
	return out, nil
}

func (s *server) trashedBooks(ctx context.Context) ([]TrashedBook, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT b.id, b.library_id, b.series_id, sr.title, b.path, b.title, b.deleted_at
		FROM books b JOIN series sr ON sr.id = b.series_id
		WHERE b.deleted_at IS NOT NULL AND sr.deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrashedBook{}
	for rows.Next() {
		var x TrashedBook
		var title, deleted string
		if err := rows.Scan(&x.Id, &x.LibraryId, &x.SeriesId, &x.SeriesTitle, &x.Path, &title, &deleted); err != nil {
			return nil, err
		}
		x.Title = title
		if x.Title == "" {
			x.Title = path.Base(x.Path)
		}
		x.DeletedAt = trashTime(deleted)
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.SortStableFunc(out, func(a, b TrashedBook) int { return b.DeletedAt.Compare(a.DeletedAt) })
	return out, nil
}

// DeleteTrashedSeries removes one trashed series and its books from the
// database. Files are not touched.
func (s *server) DeleteTrashedSeries(ctx context.Context, req DeleteTrashedSeriesRequestObject) (DeleteTrashedSeriesResponseObject, error) {
	_, found, err := s.purgeTrash(ctx, req.SeriesId, "")
	if err != nil {
		return nil, err
	}
	if !found {
		return DeleteTrashedSeries404JSONResponse{notFound(ctx, "not_found.series")}, nil
	}
	return DeleteTrashedSeries204Response{}, nil
}

// DeleteTrashedBook removes one trashed book from the database. Files are not
// touched.
func (s *server) DeleteTrashedBook(ctx context.Context, req DeleteTrashedBookRequestObject) (DeleteTrashedBookResponseObject, error) {
	_, found, err := s.purgeTrash(ctx, "", req.BookId)
	if err != nil {
		return nil, err
	}
	if !found {
		return DeleteTrashedBook404JSONResponse{notFound(ctx, "not_found.book")}, nil
	}
	return DeleteTrashedBook204Response{}, nil
}
