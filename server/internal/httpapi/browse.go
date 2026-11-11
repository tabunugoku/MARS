package httpapi

import (
	"context"
	"strings"
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
	seriesReadBooks  = `(SELECT count(*) FROM books b JOIN read_progress rp ON rp.book_id = b.id AND rp.user_id = ?
	  WHERE b.series_id = s.id AND b.deleted_at IS NULL AND rp.completed = 1)`
	seriesTouched = `EXISTS (SELECT 1 FROM books b JOIN read_progress rp ON rp.book_id = b.id AND rp.user_id = ?
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

func (s *server) GetAdjacentBooks(ctx context.Context, req GetAdjacentBooksRequestObject) (GetAdjacentBooksResponseObject, error) {
	return nil, errTODO
}

func (s *server) GetDashboard(ctx context.Context, req GetDashboardRequestObject) (GetDashboardResponseObject, error) {
	return nil, errTODO
}

func (s *server) Search(ctx context.Context, req SearchRequestObject) (SearchResponseObject, error) {
	return nil, errTODO
}
