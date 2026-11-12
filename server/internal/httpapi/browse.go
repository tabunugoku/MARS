package httpapi

import (
	"context"
	"strings"
	"unicode/utf8"
)

// likePattern turns user text into a LIKE pattern matching it anywhere,
// with % _ and \ taken literally (use with ESCAPE '\').
func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
}

// placeholders returns "?, ?, ?" for n values.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

// Per-user book counts of a series s, for filters and sorting. Each takes
// the caller's user id as its one argument.
const (
	seriesTotalBooks = `(SELECT count(*) FROM books b WHERE b.series_id = s.id AND b.deleted_at IS NULL)`
	seriesReadBooks  = `(SELECT count(*) FROM books b CROSS JOIN read_progress rp ON rp.book_id = b.id AND rp.user_id = ?
	  WHERE b.series_id = s.id AND b.deleted_at IS NULL AND rp.completed = 1)`
	seriesTouched = `EXISTS (SELECT 1 FROM books b CROSS JOIN read_progress rp ON rp.book_id = b.id AND rp.user_id = ?
	  WHERE b.series_id = s.id AND b.deleted_at IS NULL)`
)

func (s *server) ListSeries(ctx context.Context, req ListSeriesRequestObject) (ListSeriesResponseObject, error) {
	p := req.Params
	uid := currentUser(ctx).ID
	var where []string
	var args []any
	if p.LibraryId != nil {
		where, args = append(where, `s.library_id = ?`), append(args, *p.LibraryId)
	}
	if p.Search != nil && strings.TrimSpace(*p.Search) != "" {
		where = append(where, `s.id IN (SELECT ref FROM search_index WHERE kind = 'series' AND text LIKE ? ESCAPE '\')`)
		args = append(args, likePattern(strings.TrimSpace(*p.Search)))
	}
	if p.Status != nil && len(*p.Status) > 0 {
		where = append(where, `s.status IN (`+placeholders(len(*p.Status))+`)`)
		for _, v := range *p.Status {
			args = append(args, string(v))
		}
	}
	if p.Genres != nil && len(*p.Genres) > 0 {
		where = append(where, `EXISTS (SELECT 1 FROM json_each(s.genres) g WHERE g.value IN (`+placeholders(len(*p.Genres))+`))`)
		for _, v := range *p.Genres {
			args = append(args, v)
		}
	}
	if p.ReadState != nil && len(*p.ReadState) > 0 {
		var alts []string
		for _, st := range *p.ReadState {
			switch st {
			case Unread:
				alts, args = append(alts, `NOT `+seriesTouched), append(args, uid)
			case Read:
				alts, args = append(alts, `(`+seriesTotalBooks+` > 0 AND `+seriesReadBooks+` = `+seriesTotalBooks+`)`), append(args, uid)
			case InProgress:
				alts, args = append(alts, `(`+seriesTouched+` AND `+seriesReadBooks+` < `+seriesTotalBooks+`)`), append(args, uid, uid)
			}
		}
		if len(alts) == 0 {
			alts = []string{"0"}
		}
		where = append(where, `(`+strings.Join(alts, ` OR `)+`)`)
	}
	cond := ""
	if len(where) > 0 {
		cond = ` AND ` + strings.Join(where, ` AND `)
	}

	scope, scopeArgs := libScope(ctx, "s.library_id")
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM series s WHERE s.deleted_at IS NULL AND `+scope+cond,
		append(scopeArgs, args...)...).Scan(&total); err != nil {
		return nil, err
	}

	dir := "ASC"
	if p.Desc != nil && *p.Desc {
		dir = "DESC"
	}
	order := `s.title_sort ` + dir
	if p.Sort != nil {
		switch *p.Sort {
		case Updated:
			order = `s.updated_at ` + dir
		case Created:
			order = `s.created_at ` + dir
		case BookCount:
			order = seriesTotalBooks + ` ` + dir
		case ReleaseDate:
			order = `(SELECT min(b.release_date) FROM books b WHERE b.series_id = s.id AND b.deleted_at IS NULL) ` + dir + ` NULLS LAST`
		}
	}
	tail := `ORDER BY ` + order + `, s.title_sort, s.id`
	if p.Limit != nil {
		limit := min(max(*p.Limit, 1), 1000)
		offset := 0
		if p.Offset != nil {
			offset = max(*p.Offset, 0)
		}
		tail += ` LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
	} else if p.Offset != nil && *p.Offset > 0 {
		tail += ` LIMIT -1 OFFSET ?`
		args = append(args, *p.Offset)
	}
	out, err := s.listSeries(ctx, cond, tail, args...)
	if err != nil {
		return nil, err
	}
	return ListSeries200JSONResponse{Body: out, Headers: ListSeries200ResponseHeaders{XTotalCount: &total}}, nil
}

// ListGenres lists the distinct genres of the series the caller may see:
// the genre filter's options, without sending every series to the client.
func (s *server) ListGenres(ctx context.Context, req ListGenresRequestObject) (ListGenresResponseObject, error) {
	scope, args := libScope(ctx, "s.library_id")
	q := `SELECT DISTINCT g.value FROM series s, json_each(s.genres) g WHERE s.deleted_at IS NULL AND ` + scope
	if req.Params.LibraryId != nil {
		q += ` AND s.library_id = ?`
		args = append(args, *req.Params.LibraryId)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY g.value`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := ListGenres200JSONResponse{}
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *server) GetAdjacentBooks(ctx context.Context, req GetAdjacentBooksRequestObject) (GetAdjacentBooksResponseObject, error) {
	cur, ok, err := s.oneBook(ctx, req.BookId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return GetAdjacentBooks404JSONResponse{notFound("本が見つかりません")}, nil
	}
	prev, err := s.listBooks(ctx, ` AND b.series_id = ? AND (b.number_sort, b.path) < (?, ?)`,
		`ORDER BY b.number_sort DESC, b.path DESC LIMIT 1`, cur.SeriesId, cur.NumberSort, cur.Path)
	if err != nil {
		return nil, err
	}
	next, err := s.listBooks(ctx, ` AND b.series_id = ? AND (b.number_sort, b.path) > (?, ?)`,
		`ORDER BY b.number_sort, b.path LIMIT 1`, cur.SeriesId, cur.NumberSort, cur.Path)
	if err != nil {
		return nil, err
	}
	var out AdjacentBooks
	if len(prev) > 0 {
		out.Prev = &prev[0]
	}
	if len(next) > 0 {
		out.Next = &next[0]
	}
	return GetAdjacentBooks200JSONResponse(out), nil
}

// onDeckWhere selects the first unread book b of a series when the book
// before it is finished, the series has no book in progress, and no earlier
// book is still unread. With nothing in progress and nothing earlier unread,
// every earlier book is finished, so "the book before it is finished" is
// just "there is an earlier book".
//
// The query starts from the caller's finished books: for each series that
// has one, it takes that series' first unread book, then checks the rest.
// Testing every unread book of the library instead cost 5 s at 37,000 books
// (docs/perf-large-library.md). The caller's user id is bound three times,
// then once more in the ORDER BY (see GetDashboard).
const onDeckWhere = ` AND rp.book_id IS NULL
  AND b.id IN (SELECT (SELECT u.id FROM books u LEFT JOIN read_progress ur ON ur.book_id = u.id AND ur.user_id = ?
      WHERE u.series_id = c.series_id AND u.deleted_at IS NULL AND ur.book_id IS NULL
      ORDER BY u.number_sort, u.path LIMIT 1)
    FROM (SELECT DISTINCT f.series_id FROM read_progress fr CROSS JOIN books f ON f.id = fr.book_id
      WHERE fr.user_id = ? AND fr.completed = 1) c)
  AND NOT EXISTS (SELECT 1 FROM books q CROSS JOIN read_progress qr ON qr.book_id = q.id AND qr.user_id = ?
    WHERE q.series_id = b.series_id AND q.deleted_at IS NULL AND qr.completed = 0)
  AND EXISTS (SELECT 1 FROM books p WHERE p.series_id = b.series_id AND p.deleted_at IS NULL
    AND (p.number_sort, p.path) < (b.number_sort, b.path))`

func (s *server) GetDashboard(ctx context.Context, req GetDashboardRequestObject) (GetDashboardResponseObject, error) {
	uid := currentUser(ctx).ID
	var libArgs []any
	bLib, sLib := "", ""
	if req.Params.LibraryId != nil {
		libArgs = []any{*req.Params.LibraryId}
		bLib, sLib = ` AND b.library_id = ?`, ` AND s.library_id = ?`
	}
	books := func(where, tail string, args ...any) ([]Book, error) {
		return s.listBooks(ctx, bLib+where, tail, append(append([]any{}, libArgs...), args...)...)
	}
	series := func(where, tail string, args ...any) ([]Series, error) {
		return s.listSeries(ctx, sLib+where, tail, append(append([]any{}, libArgs...), args...)...)
	}

	var d Dashboard
	var err error
	if d.KeepReading, err = books(` AND rp.completed = 0`, `ORDER BY rp.read_at DESC, b.id LIMIT 20`); err != nil {
		return nil, err
	}
	if d.OnDeck, err = books(onDeckWhere, `ORDER BY (SELECT max(sr.read_at) FROM books sb
	  CROSS JOIN read_progress sr ON sr.book_id = sb.id AND sr.user_id = ?
	  WHERE sb.series_id = b.series_id AND sb.deleted_at IS NULL) DESC, b.id LIMIT 20`, uid, uid, uid, uid); err != nil {
		return nil, err
	}
	if d.RecentlyAddedBooks, err = books(``, `ORDER BY b.created_at DESC, b.id DESC LIMIT 20`); err != nil {
		return nil, err
	}
	if d.RecentlyAddedSeries, err = series(``, `ORDER BY s.created_at DESC, s.id DESC LIMIT 20`); err != nil {
		return nil, err
	}
	if d.RecentlyRead, err = books(` AND rp.completed = 1`, `ORDER BY rp.read_at DESC, b.id LIMIT 20`); err != nil {
		return nil, err
	}
	if d.RecentlyUpdatedSeries, err = series(` AND s.updated_at > s.created_at`, `ORDER BY s.updated_at DESC, s.id DESC LIMIT 20`); err != nil {
		return nil, err
	}
	return GetDashboard200JSONResponse(d), nil
}

func (s *server) Search(ctx context.Context, req SearchRequestObject) (SearchResponseObject, error) {
	q := strings.TrimSpace(req.Params.Q)
	res := SearchResult{Series: []Series{}, Books: []Book{}, Collections: []Collection{}, ReadLists: []ReadList{}}
	if q == "" {
		return Search200JSONResponse(res), nil
	}
	// trigram needs three characters; shorter queries use LIKE on the index.
	var seriesCond, bookCond string
	var match any
	if utf8.RuneCountInString(q) >= 3 {
		seriesCond = ` AND s.id IN (SELECT ref FROM search_index WHERE kind = 'series' AND text MATCH ?)`
		bookCond = ` AND b.id IN (SELECT ref FROM search_index WHERE kind = 'book' AND text MATCH ?)`
		match = `"` + strings.ReplaceAll(q, `"`, `""`) + `"`
	} else {
		seriesCond = ` AND s.id IN (SELECT ref FROM search_index WHERE kind = 'series' AND text LIKE ? ESCAPE '\')`
		bookCond = ` AND b.id IN (SELECT ref FROM search_index WHERE kind = 'book' AND text LIKE ? ESCAPE '\')`
		match = likePattern(q)
	}
	series, err := s.listSeries(ctx, seriesCond, "ORDER BY s.title_sort LIMIT 20", match)
	if err != nil {
		return nil, err
	}
	books, err := s.listBooks(ctx, bookCond, "ORDER BY b.title, b.number_sort LIMIT 20", match)
	if err != nil {
		return nil, err
	}
	res.Series, res.Books = series, books

	// Collections and read lists match by name, with the visibility rule of
	// lists.go (a normal user sees only those with a member they may see).
	pat := likePattern(q)
	cs, err := s.collections(ctx, ` WHERE name LIKE ? ESCAPE '\'`, pat)
	if err != nil {
		return nil, err
	}
	rl, err := s.readLists(ctx, ` WHERE name LIKE ? ESCAPE '\'`, pat)
	if err != nil {
		return nil, err
	}
	res.Collections, res.ReadLists = cs[:min(len(cs), 20)], rl[:min(len(rl), 20)]
	return Search200JSONResponse(res), nil
}
