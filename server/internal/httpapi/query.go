package httpapi

import (
	"context"
	"database/sql"
	"errors"
)

// Every handler that returns series or books builds its query from
// seriesQuery or bookQuery. Both already apply the caller's library scope
// (libScope), leave out the trash, and fill in the caller's own reading
// progress, so no handler can forget one of the three.
//
// Per-series counts over the caller's progress join books CROSS JOIN
// read_progress: SQLite keeps a CROSS JOIN's order, so the count walks the
// series' few books (books_series) and looks each up in read_progress by
// primary key. With a plain JOIN and no statistics the planner started from
// all of the user's progress rows instead, once per series: about 30 s for
// /series at 2,500 series and 4,700 books read (docs/perf-large-library.md).

const seriesCols = `s.id, s.library_id, s.title, s.title_sort, s.alternate_titles, s.status, s.summary, s.publisher,
	s.reading_direction, s.age_rating, s.language, s.genres, s.tags, s.authors, s.total_book_count, s.locked_fields,
	s.created_at, s.updated_at,
	(SELECT count(*) FROM books b WHERE b.series_id = s.id AND b.deleted_at IS NULL)`

// seriesQuery returns "SELECT ... FROM series s WHERE <visible>" and its
// arguments. Append " AND ..." conditions, then ORDER BY / LIMIT.
func seriesQuery(ctx context.Context) (string, []any) {
	uid := currentUser(ctx).ID
	cond, args := libScope(ctx, "s.library_id")
	return `SELECT ` + seriesCols + `,
	(SELECT count(*) FROM books b CROSS JOIN read_progress rp ON rp.book_id = b.id AND rp.user_id = ?
	  WHERE b.series_id = s.id AND b.deleted_at IS NULL AND rp.completed = 1),
	(SELECT count(*) FROM books b CROSS JOIN read_progress rp ON rp.book_id = b.id AND rp.user_id = ?
	  WHERE b.series_id = s.id AND b.deleted_at IS NULL AND rp.completed = 0)
	FROM series s WHERE s.deleted_at IS NULL AND ` + cond, append([]any{uid, uid}, args...)
}

func scanSeries(r scanner) (Series, error) {
	var x Series
	var alt, status, dir, genres, tags, au, locked string
	var age, total sql.NullInt64
	err := r.Scan(&x.Id, &x.LibraryId, &x.Title, &x.TitleSort, &alt, &status, &x.Summary, &x.Publisher,
		&dir, &age, &x.Language, &genres, &tags, &au, &total, &locked, &x.CreatedAt, &x.UpdatedAt, &x.BookCount,
		&x.BooksRead, &x.BooksInProgress)
	x.AlternateTitles, x.Genres, x.Tags, x.LockedFields = strs(alt), strs(genres), strs(tags), strs(locked)
	x.Status, x.ReadingDirection, x.Authors = SeriesStatus(status), ReadingDirection(dir), authors(au)
	if age.Valid {
		v := int(age.Int64)
		x.AgeRating = &v
	}
	if total.Valid {
		v := int(total.Int64)
		x.TotalBookCount = &v
	}
	x.ThumbnailUrl = "/api/v1/series/" + x.Id + "/thumbnail"
	return x, err
}

// listSeries runs seriesQuery + where (starting with " AND", or "") + tail
// (ORDER BY ...), with args for where and tail in that order.
func (s *server) listSeries(ctx context.Context, where, tail string, args ...any) ([]Series, error) {
	q, base := seriesQuery(ctx)
	rows, err := s.DB.QueryContext(ctx, q+where+" "+tail, append(base, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Series{}
	for rows.Next() {
		x, err := scanSeries(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// oneSeries returns a visible series; ok is false when there is none.
func (s *server) oneSeries(ctx context.Context, id string) (x Series, ok bool, err error) {
	q, args := seriesQuery(ctx)
	x, err = scanSeries(s.DB.QueryRowContext(ctx, q+` AND s.id = ?`, append(args, id)...))
	if errors.Is(err, sql.ErrNoRows) {
		return x, false, nil
	}
	return x, err == nil, err
}

const bookCols = `b.id, b.series_id, b.library_id, b.title, b.number, b.number_sort, b.summary, b.release_date, b.isbn,
	b.authors, b.tags, b.format, b.path, b.size_bytes, b.page_count, b.media_status, b.created_at, b.locked_fields, b.cover_page, b.media_error,
	(SELECT title FROM series WHERE id = b.series_id)`

// bookQuery returns "SELECT ... FROM books b LEFT JOIN read_progress rp ...
// WHERE <visible>" and its arguments, like seriesQuery. rp is the caller's
// progress row (NULL columns when unread).
func bookQuery(ctx context.Context) (string, []any) {
	uid := currentUser(ctx).ID
	cond, args := libScope(ctx, "b.library_id")
	return `SELECT ` + bookCols + `, rp.page, rp.completed, rp.read_at
	FROM books b LEFT JOIN read_progress rp ON rp.book_id = b.id AND rp.user_id = ?
	WHERE b.deleted_at IS NULL AND ` + cond, append([]any{uid}, args...)
}

func scanBook(r scanner) (Book, error) {
	var x Book
	var release, readAt sql.NullString
	var page sql.NullInt64
	var completed sql.NullBool
	var au, tags, format, status, locked, mediaErr string
	err := r.Scan(&x.Id, &x.SeriesId, &x.LibraryId, &x.Title, &x.Number, &x.NumberSort, &x.Summary, &release, &x.Isbn,
		&au, &tags, &format, &x.Path, &x.SizeBytes, &x.PageCount, &status, &x.CreatedAt, &locked, &x.CoverPage, &mediaErr, &x.SeriesTitle,
		&page, &completed, &readAt)
	x.Authors, x.Tags, x.Format, x.MediaStatus = authors(au), strs(tags), BookFormat(format), MediaStatus(status)
	x.LockedFields = strs(locked)
	x.MediaError = &mediaErr
	if release.Valid {
		x.ReleaseDate = &release.String
	}
	if page.Valid {
		x.Progress = &ReadProgress{Page: int(page.Int64), Completed: completed.Bool, ReadAt: readAt.String}
	}
	x.ThumbnailUrl = "/api/v1/books/" + x.Id + "/thumbnail"
	return x, err
}

// listBooks is listSeries for books.
func (s *server) listBooks(ctx context.Context, where, tail string, args ...any) ([]Book, error) {
	q, base := bookQuery(ctx)
	rows, err := s.DB.QueryContext(ctx, q+where+" "+tail, append(base, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Book{}
	for rows.Next() {
		x, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// oneBook returns a visible book; ok is false when there is none.
func (s *server) oneBook(ctx context.Context, id string) (x Book, ok bool, err error) {
	q, args := bookQuery(ctx)
	x, err = scanBook(s.DB.QueryRowContext(ctx, q+` AND b.id = ?`, append(args, id)...))
	if errors.Is(err, sql.ErrNoRows) {
		return x, false, nil
	}
	return x, err == nil, err
}
