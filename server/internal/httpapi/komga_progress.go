package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/tabunugoku/cominavi/server/internal/events"
)

// komgaProgressRoutes registers the read-progress endpoints Mihon's Komga
// tracker calls (see docs/opds-komga.md).
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
// its visible books in list order. A list whose members are all hidden is not
// visible to a non-administrator, as in the main API.
func (s *server) komgaTachiReadList(ctx context.Context, id string) (bool, []Book, error) {
	ls, err := s.readLists(ctx, " WHERE id = ?", id)
	if err != nil {
		return false, nil, err
	}
	if len(ls) == 0 {
		return false, nil, nil
	}
	books, err := s.listBooks(ctx,
		` AND b.id IN (SELECT book_id FROM read_list_books WHERE read_list_id = ?)`,
		`ORDER BY (SELECT position FROM read_list_books WHERE read_list_id = ? AND book_id = b.id)`,
		id, id)
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
	books, err := s.listBooks(ctx, " AND b.series_id = ?", "ORDER BY b.number_sort, b.id", id)
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
	if err := setReadState(ctx, tx, true, "b.series_id = ? AND b.number_sort <= ?",
		[]any{id, *body.LastBookNumberSortRead}); err != nil {
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
