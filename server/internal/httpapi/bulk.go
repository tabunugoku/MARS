package httpapi

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/events"
	"github.com/tabunugoku/cominavi/server/internal/scan"
)

// Bulk operations change or add many series, books or list members in one
// transaction. Every id is resolved through the caller's scope first (the
// same rule as seriesQuery, bookQuery and collectionMembers): one id the
// caller cannot see refuses the whole request, and nothing is written.

// maxBulkIDs is the most ids one bulk request may name (the contract's maxItems).
const maxBulkIDs = 1000

// bulkIDs returns the distinct ids of a request, or the catalog code of why
// the list cannot be used.
func bulkIDs(in []string) ([]string, string) {
	switch {
	case len(in) == 0:
		return nil, "bad_request.ids_required"
	case len(in) > maxBulkIDs:
		return nil, "bad_request.ids_too_many"
	}
	return distinct(in), ""
}

// cleanNames trims each name, drops empty ones and repeats, keeping order.
func cleanNames(in *[]string) []string {
	if in == nil {
		return nil
	}
	var out []string
	for _, v := range *in {
		v = strings.TrimSpace(v)
		if v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// editNames removes the names in remove from cur, then appends the names in
// add that are not there yet. Order is kept; a name is never added twice.
func editNames(cur, add, remove []string) []string {
	out := []string{}
	for _, v := range cur {
		if !slices.Contains(remove, v) {
			out = append(out, v)
		}
	}
	for _, v := range add {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// rowEdit collects the column changes of one row and the fields they lock.
type rowEdit struct {
	set   []string
	args  []any
	locks []string
}

func (e *rowEdit) put(lock, col string, v any) {
	e.set, e.args = append(e.set, col+" = ?"), append(e.args, v)
	e.locks = append(e.locks, lock)
}

// text sets a text column when its value differs.
func (e *rowEdit) text(lock, col, cur, next string) {
	if cur != next {
		e.put(lock, col, next)
	}
}

// names sets a JSON list column when its list differs.
func (e *rowEdit) names(lock, col string, cur, next []string) {
	if !slices.Equal(cur, next) {
		e.put(lock, col, jsonText(next))
	}
}

// age sets age_rating when it differs; a nil next clears it.
func (e *rowEdit) age(cur sql.NullInt64, next *int) {
	if cur.Valid == (next != nil) && (next == nil || cur.Int64 == int64(*next)) {
		return
	}
	var v any
	if next != nil {
		v = *next
	}
	e.put("ageRating", "age_rating", v)
}

// update returns the UPDATE that writes the edit to one row. The lock list is
// the current one plus the fields just edited (lockJSON, as UpdateSeries).
func (e *rowEdit) update(table, id, locks string) (string, []any) {
	set := append(slices.Clone(e.set), "locked_fields = ?", "updated_at = ?")
	args := append(slices.Clone(e.args), lockJSON(locks, e.locks, nil), time.Now().UTC().Format(time.RFC3339Nano), id)
	return `UPDATE ` + table + ` SET ` + strings.Join(set, ", ") + ` WHERE id = ?`, args
}

// seriesState is what a bulk edit reads of one series.
type seriesState struct {
	id, libraryID, status, publisher, direction, language, genres, tags, locks string
	age                                                                        sql.NullInt64
}

// seriesForEdit loads the given series as the caller sees them. ok is false
// when one of them is missing, in the trash or outside the caller's libraries.
func seriesForEdit(ctx context.Context, tx *sql.Tx, ids []string) ([]seriesState, bool, error) {
	cond, args := libScope(ctx, "s.library_id")
	rows, err := tx.QueryContext(ctx, `SELECT s.id, s.library_id, s.status, s.publisher, s.reading_direction, s.age_rating,
		s.language, s.genres, s.tags, s.locked_fields
		FROM series s WHERE s.deleted_at IS NULL AND `+cond+` AND s.id IN (`+placeholders(len(ids))+`)`,
		append(args, anyStrings(ids)...)...)
	if err != nil {
		return nil, false, err
	}
	out, err := collect(rows, func(r scanner) (seriesState, error) {
		var x seriesState
		err := r.Scan(&x.id, &x.libraryID, &x.status, &x.publisher, &x.direction, &x.age, &x.language, &x.genres, &x.tags, &x.locks)
		return x, err
	})
	if err != nil {
		return nil, false, err
	}
	return out, len(out) == len(ids), nil
}

// bookState is what a bulk edit or a read-state write reads of one book.
type bookState struct {
	id, seriesID, libraryID, tags, locks string
}

// booksForEdit loads the given books as the caller sees them; ok is false
// when one of them is missing, in the trash or outside the caller's libraries.
func booksForEdit(ctx context.Context, tx *sql.Tx, ids []string) ([]bookState, bool, error) {
	cond, args := libScope(ctx, "b.library_id")
	rows, err := tx.QueryContext(ctx, `SELECT b.id, b.series_id, b.library_id, b.tags, b.locked_fields
		FROM books b WHERE b.deleted_at IS NULL AND `+cond+` AND b.id IN (`+placeholders(len(ids))+`)`,
		append(args, anyStrings(ids)...)...)
	if err != nil {
		return nil, false, err
	}
	out, err := collect(rows, func(r scanner) (bookState, error) {
		var x bookState
		err := r.Scan(&x.id, &x.seriesID, &x.libraryID, &x.tags, &x.locks)
		return x, err
	})
	if err != nil {
		return nil, false, err
	}
	return out, len(out) == len(ids), nil
}

// checkSeriesBulk names the first invalid value of a series bulk edit, or
// "bad_request.nothing_to_change" when the request asks for no change.
func checkSeriesBulk(b *SeriesBulkUpdate) string {
	switch {
	case b.Status != nil && !b.Status.Valid():
		return "bad_request.status"
	case b.ReadingDirection != nil && !b.ReadingDirection.Valid():
		return "bad_request.reading_direction"
	case b.AgeRating.Set && b.AgeRating.Value != nil && (*b.AgeRating.Value < 0 || *b.AgeRating.Value > 99):
		return "bad_request.age_rating"
	}
	none := b.Status == nil && b.Publisher == nil && b.ReadingDirection == nil && b.Language == nil && !b.AgeRating.Set
	if none && len(cleanNames(b.AddGenres))+len(cleanNames(b.RemoveGenres))+
		len(cleanNames(b.AddTags))+len(cleanNames(b.RemoveTags)) == 0 {
		return "bad_request.nothing_to_change"
	}
	return ""
}

// BulkUpdateSeries sets the given fields on many series. A field locks on a
// series only when that series' value actually changes. The response counts
// the series that changed.
func (s *server) BulkUpdateSeries(ctx context.Context, req BulkUpdateSeriesRequestObject) (BulkUpdateSeriesResponseObject, error) {
	b := req.Body
	if b == nil {
		return BulkUpdateSeries400JSONResponse{badRequest(ctx, "bad_request.body_missing")}, nil
	}
	ids, code := bulkIDs(b.SeriesIds)
	if code == "" {
		code = checkSeriesBulk(b)
	}
	if code != "" {
		return BulkUpdateSeries400JSONResponse{badRequest(ctx, code)}, nil
	}
	addG, remG := cleanNames(b.AddGenres), cleanNames(b.RemoveGenres)
	addT, remT := cleanNames(b.AddTags), cleanNames(b.RemoveTags)

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, ok, err := seriesForEdit(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	if !ok {
		return BulkUpdateSeries404JSONResponse{notFound(ctx, "not_found.series")}, nil
	}
	updated := 0
	for _, cur := range rows {
		var e rowEdit
		if b.Status != nil {
			e.text("status", "status", cur.status, string(*b.Status))
		}
		if b.Publisher != nil {
			e.text("publisher", "publisher", cur.publisher, *b.Publisher)
		}
		if b.ReadingDirection != nil {
			e.text("readingDirection", "reading_direction", cur.direction, string(*b.ReadingDirection))
		}
		if b.AgeRating.Set {
			e.age(cur.age, b.AgeRating.Value)
		}
		if b.Language != nil {
			e.text("language", "language", cur.language, *b.Language)
		}
		genres := strs(cur.genres)
		e.names("genres", "genres", genres, editNames(genres, addG, remG))
		tags := strs(cur.tags)
		e.names("tags", "tags", tags, editNames(tags, addT, remT))
		if len(e.set) == 0 {
			continue
		}
		q, args := e.update("series", cur.id, cur.locks)
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return nil, err
		}
		if err := scan.IndexSeries(ctx, tx, cur.id); err != nil {
			return nil, err
		}
		updated++
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return BulkUpdateSeries200JSONResponse{Updated: updated}, nil
}

// BulkUpdateBooks adds and removes tags on many books. Tags lock on a book
// only when its tags change. The response counts the books that changed.
func (s *server) BulkUpdateBooks(ctx context.Context, req BulkUpdateBooksRequestObject) (BulkUpdateBooksResponseObject, error) {
	b := req.Body
	if b == nil {
		return BulkUpdateBooks400JSONResponse{badRequest(ctx, "bad_request.body_missing")}, nil
	}
	ids, code := bulkIDs(b.BookIds)
	addT, remT := cleanNames(b.AddTags), cleanNames(b.RemoveTags)
	if code == "" && len(addT)+len(remT) == 0 {
		code = "bad_request.nothing_to_change"
	}
	if code != "" {
		return BulkUpdateBooks400JSONResponse{badRequest(ctx, code)}, nil
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, ok, err := booksForEdit(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	if !ok {
		return BulkUpdateBooks404JSONResponse{notFound(ctx, "not_found.book")}, nil
	}
	updated := 0
	for _, cur := range rows {
		var e rowEdit
		tags := strs(cur.tags)
		e.names("tags", "tags", tags, editNames(tags, addT, remT))
		if len(e.set) == 0 {
			continue
		}
		q, args := e.update("books", cur.id, cur.locks)
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return nil, err
		}
		if err := scan.IndexBook(ctx, tx, cur.id); err != nil {
			return nil, err
		}
		updated++
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return BulkUpdateBooks200JSONResponse{Updated: updated}, nil
}

// setReadState writes the caller's progress on the live books matching where
// (a condition on alias b with its args). Read puts each book on its last page
// (page 1 when it has none) and only for ready books; unread clears the
// caller's progress on every live book. Like markRead, a write without a
// writer is not ordered, so it always applies.
func setReadState(ctx context.Context, tx *sql.Tx, read bool, where string, args []any) error {
	uid := currentUser(ctx).ID
	if read {
		_, err := tx.ExecContext(ctx, `INSERT INTO read_progress (user_id, book_id, page, completed, read_at)
			SELECT ?, b.id, max(b.page_count, 1), 1, ? FROM books b WHERE b.deleted_at IS NULL AND b.media_status = 'ready' AND `+where+`
			ON CONFLICT(user_id, book_id) DO UPDATE SET page = excluded.page, completed = 1, read_at = excluded.read_at`,
			append([]any{uid, time.Now().UTC().Format(time.RFC3339)}, args...)...)
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM read_progress WHERE user_id = ?
		AND book_id IN (SELECT b.id FROM books b WHERE b.deleted_at IS NULL AND `+where+`)`,
		append([]any{uid}, args...)...)
	return err
}

// SetSeriesReadState marks every book of the given series read or unread for
// the caller, in one transaction.
func (s *server) SetSeriesReadState(ctx context.Context, req SetSeriesReadStateRequestObject) (SetSeriesReadStateResponseObject, error) {
	b := req.Body
	if b == nil {
		return SetSeriesReadState400JSONResponse{badRequest(ctx, "bad_request.body_missing")}, nil
	}
	ids, code := bulkIDs(b.SeriesIds)
	if code != "" {
		return SetSeriesReadState400JSONResponse{badRequest(ctx, code)}, nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, ok, err := seriesForEdit(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	if !ok {
		return SetSeriesReadState404JSONResponse{notFound(ctx, "not_found.series")}, nil
	}
	if err := setReadState(ctx, tx, b.Read, "b.series_id IN ("+placeholders(len(ids))+")", anyStrings(ids)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	for _, r := range rows {
		s.progressChanged(ctx, events.Progress{LibraryID: r.libraryID, SeriesID: r.id})
	}
	return SetSeriesReadState204Response{}, nil
}

// SetBooksReadState marks the given books read or unread for the caller, in
// one transaction.
func (s *server) SetBooksReadState(ctx context.Context, req SetBooksReadStateRequestObject) (SetBooksReadStateResponseObject, error) {
	b := req.Body
	if b == nil {
		return SetBooksReadState400JSONResponse{badRequest(ctx, "bad_request.body_missing")}, nil
	}
	ids, code := bulkIDs(b.BookIds)
	if code != "" {
		return SetBooksReadState400JSONResponse{badRequest(ctx, code)}, nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, ok, err := booksForEdit(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	if !ok {
		return SetBooksReadState404JSONResponse{notFound(ctx, "not_found.book")}, nil
	}
	if err := setReadState(ctx, tx, b.Read, "b.id IN ("+placeholders(len(ids))+")", anyStrings(ids)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	for _, r := range rows {
		s.progressChanged(ctx, events.Progress{LibraryID: r.libraryID, SeriesID: r.seriesID, BookID: r.id})
	}
	return SetBooksReadState204Response{}, nil
}

// addMany appends the members, in the given order, to one list. A member
// already in the list keeps its place, and hidden members are kept. It
// returns the catalog code of the first refusal, or "" when it wrote: a
// missing list, or a member the caller cannot see, refuses everything.
func (s *server) addMany(ctx context.Context, l listLink, listID string, members []string) (string, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	ok, err := l.listExists(ctx, tx, listID)
	if err != nil {
		return "", err
	}
	if !ok {
		return l.notFound, nil
	}
	for _, id := range members {
		ok, err := l.memberVisible(ctx, tx, id)
		if err != nil {
			return "", err
		}
		if !ok {
			return l.noMember, nil
		}
	}
	for _, id := range members {
		if err := l.add(ctx, tx, listID, id); err != nil {
			return "", err
		}
	}
	return "", tx.Commit()
}

// AddCollectionSeriesMany appends several series to a collection, in order.
func (s *server) AddCollectionSeriesMany(ctx context.Context, req AddCollectionSeriesManyRequestObject) (AddCollectionSeriesManyResponseObject, error) {
	l := collectionLink
	var in []string
	if req.Body != nil {
		in = req.Body.SeriesIds
	}
	ids, code := bulkIDs(in)
	if code != "" {
		return AddCollectionSeriesMany400JSONResponse{badRequest(ctx, code)}, nil
	}
	if code, err := s.addMany(ctx, l, req.CollectionId, ids); err != nil {
		return nil, err
	} else if code != "" {
		return AddCollectionSeriesMany404JSONResponse{notFound(ctx, code)}, nil
	}
	cs, err := s.collections(ctx, " WHERE id = ?", req.CollectionId)
	if err != nil {
		return nil, err
	}
	if len(cs) == 0 {
		return AddCollectionSeriesMany404JSONResponse{notFound(ctx, l.notFound)}, nil
	}
	return AddCollectionSeriesMany200JSONResponse(cs[0]), nil
}

// AddReadListBooks appends several books to a read list, in order.
func (s *server) AddReadListBooks(ctx context.Context, req AddReadListBooksRequestObject) (AddReadListBooksResponseObject, error) {
	l := readListLink
	var in []string
	if req.Body != nil {
		in = req.Body.BookIds
	}
	ids, code := bulkIDs(in)
	if code != "" {
		return AddReadListBooks400JSONResponse{badRequest(ctx, code)}, nil
	}
	if code, err := s.addMany(ctx, l, req.ReadListId, ids); err != nil {
		return nil, err
	} else if code != "" {
		return AddReadListBooks404JSONResponse{notFound(ctx, code)}, nil
	}
	ls, err := s.readLists(ctx, " WHERE id = ?", req.ReadListId)
	if err != nil {
		return nil, err
	}
	if len(ls) == 0 {
		return AddReadListBooks404JSONResponse{notFound(ctx, l.notFound)}, nil
	}
	return AddReadListBooks200JSONResponse(ls[0]), nil
}
