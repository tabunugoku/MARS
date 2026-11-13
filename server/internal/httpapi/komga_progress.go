package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/tabunugoku/mars/server/internal/events"
)

// komgaProgressRoutes registers the read-progress endpoints Mihon's Komga
// tracker calls (see docs/dev/opds-komga-spec.md).
//
// Reads go through the same visibility as the rest of the compat API: a
// series or read list the caller cannot see is 404. Writes go through
// setReadState (the main API's read-state code), so a book marked read here
// is completed on its last page, exactly as in the main API, and each write
// publishes the same progress events.
func (s *server) komgaProgressRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /komga/api/v2/series/{seriesId}/read-progress/tachiyomi", s.komgaTachiSeriesGet)
	mux.HandleFunc("PUT /komga/api/v2/series/{seriesId}/read-progress/tachiyomi", s.komgaTachiSeriesPut)
	mux.HandleFunc("GET /komga/api/v1/readlists/{id}/read-progress/tachiyomi", s.komgaTachiReadListGet)
	mux.HandleFunc("PUT /komga/api/v1/readlists/{id}/read-progress/tachiyomi", s.komgaTachiReadListPut)
	mux.HandleFunc("PATCH /komga/api/v1/books/{bookId}/read-progress", s.komgaBookProgressPatch)
	mux.HandleFunc("DELETE /komga/api/v1/books/{bookId}/read-progress", s.komgaBookProgressDelete)
	mux.HandleFunc("POST /komga/api/v1/series/{seriesId}/read-progress", s.komgaSeriesMarkRead(true))
	mux.HandleFunc("DELETE /komga/api/v1/series/{seriesId}/read-progress", s.komgaSeriesMarkRead(false))
}

// komgaSetRead marks the live books that where selects read or unread, in one
// transaction, through setReadState (the main API's write).
func (s *server) komgaSetRead(ctx context.Context, read bool, where string, args []any) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := setReadState(ctx, tx, read, where, args); err != nil {
		return err
	}
	return tx.Commit()
}

// komgaBookProgressBody is the body of PATCH books/{id}/read-progress. The
// pointers tell a missing field from a false or zero one.
type komgaBookProgressBody struct {
	Page      *int  `json:"page"`
	Completed *bool `json:"completed"`
}

// komgaBookProgressPatch writes the caller's progress on a book. completed:
// true marks it read (on its last page, as setReadState does). Otherwise page
// (1-based, 1 to the page count) is set and the book is completed when page is
// its last page, as the main API does. Neither is 400.
func (s *server) komgaBookProgressPatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	book, ok, err := s.oneBook(ctx, r.PathValue("bookId"))
	if err != nil {
		s.komgaFail(w, r, err)
		return
	}
	if !ok {
		writeError(ctx, w, http.StatusNotFound, "not_found.book")
		return
	}
	var body komgaBookProgressBody
	if !komgaTachiDecode(r, &body) {
		writeError(ctx, w, http.StatusBadRequest, "bad_request.malformed")
		return
	}
	switch {
	case body.Completed != nil && *body.Completed:
		if err := s.komgaSetRead(ctx, true, "b.id = ?", []any{book.Id}); err != nil {
			s.komgaFail(w, r, err)
			return
		}
	case body.Page != nil:
		if *body.Page < 1 || *body.Page > book.PageCount {
			writeError(ctx, w, http.StatusBadRequest, "bad_request.page_number")
			return
		}
		p := ReadProgress{Page: *body.Page, Completed: *body.Page == book.PageCount, ReadAt: time.Now().UTC().Format(time.RFC3339)}
		if _, err := s.DB.ExecContext(ctx, upsertProgress, currentUser(ctx).ID, book.Id, p.Page, p.Completed, p.ReadAt); err != nil {
			s.komgaFail(w, r, err)
			return
		}
	default:
		writeError(ctx, w, http.StatusBadRequest, "bad_request.body_missing")
		return
	}
	s.progressChanged(ctx, events.Progress{LibraryID: book.LibraryId, SeriesID: book.SeriesId, BookID: book.Id})
	w.WriteHeader(http.StatusNoContent)
}

// komgaBookProgressDelete marks a book unread for the caller.
func (s *server) komgaBookProgressDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	book, ok, err := s.oneBook(ctx, r.PathValue("bookId"))
	if err != nil {
		s.komgaFail(w, r, err)
		return
	}
	if !ok {
		writeError(ctx, w, http.StatusNotFound, "not_found.book")
		return
	}
	if err := s.komgaSetRead(ctx, false, "b.id = ?", []any{book.Id}); err != nil {
		s.komgaFail(w, r, err)
		return
	}
	s.progressChanged(ctx, events.Progress{LibraryID: book.LibraryId, SeriesID: book.SeriesId, BookID: book.Id})
	w.WriteHeader(http.StatusNoContent)
}

// komgaSeriesMarkRead marks every live book of a visible series read (read
// true) or unread, for the caller.
func (s *server) komgaSeriesMarkRead(read bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id := r.PathValue("seriesId")
		x, ok, err := s.oneSeries(ctx, id)
		if err != nil {
			s.komgaFail(w, r, err)
			return
		}
		if !ok {
			writeError(ctx, w, http.StatusNotFound, "not_found.series")
			return
		}
		if err := s.komgaSetRead(ctx, read, "b.series_id = ?", []any{id}); err != nil {
			s.komgaFail(w, r, err)
			return
		}
		s.progressChanged(ctx, events.Progress{LibraryID: x.LibraryId, SeriesID: id})
		w.WriteHeader(http.StatusNoContent)
	}
}

// komgaTachiProgressV2 is a series' progress as the tracker reads it.
type komgaTachiProgressV2 struct {
	BooksCount                   int     `json:"booksCount"`
	BooksReadCount               int     `json:"booksReadCount"`
	BooksUnreadCount             int     `json:"booksUnreadCount"`
	BooksInProgressCount         int     `json:"booksInProgressCount"`
	LastReadContinuousNumberSort float64 `json:"lastReadContinuousNumberSort"`
	MaxNumberSort                float64 `json:"maxNumberSort"`
}

// komgaTachiProgressV1 is a read list's progress as the tracker reads it.
type komgaTachiProgressV1 struct {
	BooksCount              int `json:"booksCount"`
	BooksReadCount          int `json:"booksReadCount"`
	BooksUnreadCount        int `json:"booksUnreadCount"`
	BooksInProgressCount    int `json:"booksInProgressCount"`
	LastReadContinuousIndex int `json:"lastReadContinuousIndex"`
}

// komgaTachiSeriesUpdate is the body of a series progress write. The pointer
// tells a missing field from a zero one.
type komgaTachiSeriesUpdate struct {
	LastBookNumberSortRead *float64 `json:"lastBookNumberSortRead"`
}

// komgaTachiReadListUpdate is the body of a read list progress write.
type komgaTachiReadListUpdate struct {
	LastBookRead *int `json:"lastBookRead"`
}

// komgaTachiState classifies a book for the caller: 0 unread, 1 in progress,
// 2 read.
func komgaTachiState(b Book) int {
	switch {
	case b.Progress == nil:
		return 0
	case b.Progress.Completed:
		return 2
	}
	return 1
}

// komgaTachiCounts fills the four counters shared by both DTOs.
func komgaTachiCounts(books []Book) (count, read, unread, inProgress int) {
	for _, b := range books {
		switch komgaTachiState(b) {
		case 0:
			unread++
		case 1:
			inProgress++
		case 2:
			read++
		}
	}
	return len(books), read, unread, inProgress
}

// komgaTachiSeriesProgress takes the series' live books in numberSort order.
// The continuous position is the numberSort of the last book of the leading
// run of read books.
func komgaTachiSeriesProgress(books []Book) komgaTachiProgressV2 {
	var p komgaTachiProgressV2
	p.BooksCount, p.BooksReadCount, p.BooksUnreadCount, p.BooksInProgressCount = komgaTachiCounts(books)
	continuous := true
	for i, b := range books {
		if i == 0 || b.NumberSort > p.MaxNumberSort {
			p.MaxNumberSort = b.NumberSort
		}
		if continuous && komgaTachiState(b) == 2 {
			p.LastReadContinuousNumberSort = b.NumberSort
		} else {
			continuous = false
		}
	}
	return p
}

// komgaTachiReadListProgress takes a read list's visible books in list order.
// The continuous index is the 1-based position of the last book of the
// leading run of read books.
func komgaTachiReadListProgress(books []Book) komgaTachiProgressV1 {
	var p komgaTachiProgressV1
	p.BooksCount, p.BooksReadCount, p.BooksUnreadCount, p.BooksInProgressCount = komgaTachiCounts(books)
	continuous := true
	for i, b := range books {
		if continuous && komgaTachiState(b) == 2 {
			p.LastReadContinuousIndex = i + 1
		} else {
			continuous = false
		}
	}
	return p
}

// komgaTachiReadList reports whether the caller may see the read list, and
// its books in list order, exactly the books the browse endpoint lists by
// default (READY only), so Mihon's chapter numbers match the tracker's. A list
// whose members are all hidden is not visible to a non-administrator, as in
// the main API.
func (s *server) komgaTachiReadList(ctx context.Context, id string) (bool, []Book, error) {
	ls, err := s.readLists(ctx, " WHERE id = ?", id)
	if err != nil {
		return false, nil, err
	}
	if len(ls) == 0 {
		return false, nil, nil
	}
	where, args, tail, tailArgs := komgaReadListBookQuery(id, nil)
	books, _, err := s.komgaBooks(ctx, where, args, tail, tailArgs, komgaPaging{unpaged: true})
	return true, books, err
}

// komgaTachiFail answers an unexpected error.
func komgaTachiFail(ctx context.Context, w http.ResponseWriter, err error) {
	slog.Error("komga progress", "err", err)
	writeError(ctx, w, http.StatusInternalServerError, "server.error")
}

// komgaTachiDecode reads a JSON body into v; a body that does not parse
// reports false.
func komgaTachiDecode(r *http.Request, v any) bool {
	return json.NewDecoder(r.Body).Decode(v) == nil
}

func (s *server) komgaTachiSeriesGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("seriesId")
	_, ok, err := s.oneSeries(ctx, id)
	if err != nil {
		komgaTachiFail(ctx, w, err)
		return
	}
	if !ok {
		writeError(ctx, w, http.StatusNotFound, "not_found.series")
		return
	}
	where, args, tail, tailArgs := komgaSeriesBookQuery(id, nil)
	books, _, err := s.komgaBooks(ctx, where, args, tail, tailArgs, komgaPaging{unpaged: true})
	if err != nil {
		komgaTachiFail(ctx, w, err)
		return
	}
	writeJSON(w, http.StatusOK, komgaTachiSeriesProgress(books))
}

func (s *server) komgaTachiSeriesPut(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("seriesId")
	x, ok, err := s.oneSeries(ctx, id)
	if err != nil {
		komgaTachiFail(ctx, w, err)
		return
	}
	if !ok {
		writeError(ctx, w, http.StatusNotFound, "not_found.series")
		return
	}
	var body komgaTachiSeriesUpdate
	if !komgaTachiDecode(r, &body) || body.LastBookNumberSortRead == nil {
		writeError(ctx, w, http.StatusBadRequest, "bad_request.malformed")
		return
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		komgaTachiFail(ctx, w, err)
		return
	}
	defer tx.Rollback()
	// The books are the ones the browse and GET endpoints count (the same
	// builder), up to and including the number written.
	where, args, _, _ := komgaSeriesBookQuery(id, nil)
	cond := strings.TrimPrefix(where, " AND ") + " AND b.number_sort <= ?"
	if err := setReadState(ctx, tx, true, cond, append(args, *body.LastBookNumberSortRead)); err != nil {
		komgaTachiFail(ctx, w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		komgaTachiFail(ctx, w, err)
		return
	}
	s.progressChanged(ctx, events.Progress{LibraryID: x.LibraryId, SeriesID: id})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) komgaTachiReadListGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ok, books, err := s.komgaTachiReadList(ctx, r.PathValue("id"))
	if err != nil {
		komgaTachiFail(ctx, w, err)
		return
	}
	if !ok {
		writeError(ctx, w, http.StatusNotFound, "not_found.read_list")
		return
	}
	writeJSON(w, http.StatusOK, komgaTachiReadListProgress(books))
}

// komgaTachiReadListPut marks the first lastBookRead visible books of the
// list read. Books after them keep their progress.
func (s *server) komgaTachiReadListPut(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ok, books, err := s.komgaTachiReadList(ctx, r.PathValue("id"))
	if err != nil {
		komgaTachiFail(ctx, w, err)
		return
	}
	if !ok {
		writeError(ctx, w, http.StatusNotFound, "not_found.read_list")
		return
	}
	var body komgaTachiReadListUpdate
	if !komgaTachiDecode(r, &body) || body.LastBookRead == nil || *body.LastBookRead < 0 {
		writeError(ctx, w, http.StatusBadRequest, "bad_request.malformed")
		return
	}
	n := min(*body.LastBookRead, len(books))
	if n > 0 {
		first := books[:n]
		ids := make([]string, n)
		for i, b := range first {
			ids[i] = b.Id
		}
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			komgaTachiFail(ctx, w, err)
			return
		}
		defer tx.Rollback()
		if err := setReadState(ctx, tx, true, "b.id IN ("+placeholders(n)+")", anyStrings(ids)); err != nil {
			komgaTachiFail(ctx, w, err)
			return
		}
		if err := tx.Commit(); err != nil {
			komgaTachiFail(ctx, w, err)
			return
		}
		for _, b := range first {
			s.progressChanged(ctx, events.Progress{LibraryID: b.LibraryId, SeriesID: b.SeriesId, BookID: b.Id})
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
