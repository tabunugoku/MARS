package httpapi

import (
	"context"
	"database/sql"
)

// ListSeriesFacets returns the filter options of the series list: the
// distinct values over the live series the caller may see (one library when
// libraryId is given). Each list is sorted, and an empty one is [] not null.
func (s *server) ListSeriesFacets(ctx context.Context, req ListSeriesFacetsRequestObject) (ListSeriesFacetsResponseObject, error) {
	lib := req.Params.LibraryId
	var out SeriesFacets
	var err error
	if out.Genres, err = s.facetStrings(ctx, lib, `g.value`, `, json_each(s.genres) g`, ``); err != nil {
		return nil, err
	}
	if out.Tags, err = s.facetStrings(ctx, lib, `t.value`, `, json_each(s.tags) t`, ``); err != nil {
		return nil, err
	}
	if out.Publishers, err = s.facetStrings(ctx, lib, `s.publisher`, ``, ` AND s.publisher <> ''`); err != nil {
		return nil, err
	}
	if out.Languages, err = s.facetStrings(ctx, lib, `s.language`, ``, ` AND s.language <> ''`); err != nil {
		return nil, err
	}
	if out.Authors, err = s.facetStrings(ctx, lib, `json_extract(a.value, '$.name')`, `, json_each(s.authors) a`,
		` AND json_extract(a.value, '$.name') <> ''`); err != nil {
		return nil, err
	}
	if out.AgeRatings, err = s.facetInts(ctx, lib); err != nil {
		return nil, err
	}
	return ListSeriesFacets200JSONResponse(out), nil
}

// facetRows runs SELECT DISTINCT expr over the series the caller may see:
// from adds tables (joined to s), cond adds conditions. The caller's
// library scope applies first, then libraryId when given.
func (s *server) facetRows(ctx context.Context, lib *string, expr, from, cond string) (*sql.Rows, error) {
	scope, args := libScope(ctx, "s.library_id")
	q := `SELECT DISTINCT ` + expr + ` FROM series s` + from +
		` WHERE s.deleted_at IS NULL AND ` + scope + cond
	if lib != nil {
		q += ` AND s.library_id = ?`
		args = append(args, *lib)
	}
	return s.DB.QueryContext(ctx, q+` ORDER BY 1`, args...)
}

func (s *server) facetStrings(ctx context.Context, lib *string, expr, from, cond string) ([]string, error) {
	rows, err := s.facetRows(ctx, lib, expr, from, cond)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *server) facetInts(ctx context.Context, lib *string) ([]int, error) {
	rows, err := s.facetRows(ctx, lib, `s.age_rating`, ``, ` AND s.age_rating IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
