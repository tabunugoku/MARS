package httpapi

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/tabunugoku/cominavi/server/internal/library"

	// Decoders for convert=png: the page formats ComiNavi reads.
	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/jpeg"
)

// The Komga-shaped browse API for the Mihon/Tachiyomi Komga extension (see
// docs/opds-komga.md). Every list reads series, books, read lists and
// collections through the same builders as the main API, so a caller only
// ever sees what it may see; anything else is 404 (or left out of a list).
//
// Komga's conventions here: page is 0-based, size defaults to 20, unpaged=true
// returns everything, unknown filter or sort values are ignored, and a book
// list (series books, read list books, /books) returns only books whose media
// is READY unless media_status says otherwise.

const (
	komgaDefaultSize = 20
	// komgaMaxLimit is the largest page the series query takes as LIMIT;
	// bigger pages are cut in memory.
	komgaMaxLimit = 1000
	// komgaMaxInt caps page and size, so page*size cannot overflow.
	komgaMaxInt = 1 << 31
)

// komgaRoutes registers the Komga-shaped API under /komga/ (full paths).
func (s *server) komgaRoutes(mux *http.ServeMux) {
	const api = "GET /komga/api/v1/"
	mux.HandleFunc(api+"libraries", s.komgaLibraries)
	mux.HandleFunc(api+"series", s.komgaSeriesList)
	mux.HandleFunc(api+"series/{seriesId}", s.komgaSeriesOne)
	mux.HandleFunc(api+"series/{seriesId}/books", s.komgaSeriesBooks)
	mux.HandleFunc(api+"series/{seriesId}/thumbnail", s.serveThumbnail("series"))
	mux.HandleFunc(api+"books", s.komgaBookList)
	mux.HandleFunc(api+"books/{bookId}", s.komgaBookOne)
	mux.HandleFunc(api+"books/{bookId}/thumbnail", s.serveThumbnail("book"))
	mux.HandleFunc(api+"books/{bookId}/pages", s.komgaPages)
	mux.HandleFunc(api+"books/{bookId}/pages/{page}", s.komgaPage)
	mux.HandleFunc(api+"readlists", s.komgaReadLists)
	mux.HandleFunc(api+"readlists/{readListId}", s.komgaReadListOne)
	mux.HandleFunc(api+"readlists/{readListId}/books", s.komgaReadListBooks)
	mux.HandleFunc(api+"readlists/{readListId}/thumbnail", s.komgaReadListThumbnail)
	mux.HandleFunc(api+"collections", s.komgaCollections)
	mux.HandleFunc(api+"collections/{collectionId}/series", s.komgaCollectionSeries)
	mux.HandleFunc(api+"genres", s.komgaStrings(`g.value`, `, json_each(s.genres) g`, ``))
	mux.HandleFunc(api+"tags", s.komgaStrings(`t.value`, `, json_each(s.tags) t`, ``))
	mux.HandleFunc(api+"publishers", s.komgaStrings(`s.publisher`, ``, ` AND s.publisher <> ''`))
	mux.HandleFunc(api+"authors", s.komgaAuthorList)
	s.komgaProgressRoutes(mux)
}

// komgaPaging is the page a list asks for.
type komgaPaging struct {
	page    int
	size    int
	unpaged bool
}

func komgaPagingOf(r *http.Request) komgaPaging {
	q := r.URL.Query()
	pg := komgaPaging{size: komgaDefaultSize, unpaged: q.Get("unpaged") == "true"}
	if n, err := strconv.Atoi(q.Get("page")); err == nil && n > 0 {
		pg.page = min(n, komgaMaxInt)
	}
	if n, err := strconv.Atoi(q.Get("size")); err == nil && n > 0 {
		pg.size = min(n, komgaMaxInt)
	}
	return pg
}

// komgaSlice returns the part of all that page pg holds.
func komgaSlice[T any](all []T, pg komgaPaging) []T {
	if pg.unpaged {
		return all
	}
	if pg.page >= len(all)/pg.size+1 {
		return []T{}
	}
	start := pg.page * pg.size
	return all[start:min(start+pg.size, len(all))]
}

// komgaValues splits comma-joined values (repeated parameters too) and drops
// blanks.
func komgaValues(vals []string) []string {
	var out []string
	for _, v := range vals {
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// komgaDeletedOnly reports deleted=true. Trashed items are never visible, so
// such a list is always empty.
func komgaDeletedOnly(q url.Values) bool {
	return strings.EqualFold(q.Get("deleted"), "true")
}

// komgaSort reads sort=criteria,dir.
func komgaSort(q url.Values) (criteria string, desc bool) {
	crit, dir, _ := strings.Cut(q.Get("sort"), ",")
	return strings.TrimSpace(crit), strings.EqualFold(strings.TrimSpace(dir), "desc")
}

func komgaReadStates(vals []string) []ReadState {
	var out []ReadState
	for _, v := range komgaValues(vals) {
		switch strings.ToUpper(v) {
		case "UNREAD":
			out = append(out, Unread)
		case "IN_PROGRESS":
			out = append(out, InProgress)
		case "READ":
			out = append(out, Read)
		}
	}
	return out
}

var komgaSeriesStatuses = map[string]SeriesStatus{
	"ONGOING": Ongoing, "ENDED": Ended, "HIATUS": Hiatus, "ABANDONED": Abandoned,
}

// komgaFail answers an unexpected error.
func (s *server) komgaFail(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("komga", "path", r.URL.Path, "err", err)
	writeError(r.Context(), w, http.StatusInternalServerError, "server.error")
}

// ─── Libraries ───────────────────────────────────────────────────────────────

func (s *server) komgaLibraries(w http.ResponseWriter, r *http.Request) {
	resp, err := s.ListLibraries(r.Context(), ListLibrariesRequestObject{})
	if err != nil {
		s.komgaFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, komgaMap([]Library(resp.(ListLibraries200JSONResponse)), komgaLibrary))
}

// ─── Series ──────────────────────────────────────────────────────────────────

// komgaSeriesParams turns Komga's series filters into ListSeries parameters,
// and returns the library_id values (more than one is filtered in memory).
func komgaSeriesParams(q url.Values) (ListSeriesParams, []string) {
	var p ListSeriesParams
	if v := strings.TrimSpace(q.Get("search")); v != "" {
		p.Search = &v
	}
	if st := komgaValues(q["status"]); len(st) > 0 {
		var out []SeriesStatus
		for _, v := range st {
			if ss, ok := komgaSeriesStatuses[strings.ToUpper(v)]; ok {
				out = append(out, ss)
			}
		}
		if len(out) > 0 {
			p.Status = &out
		}
	}
	if v := komgaValues(q["genre"]); len(v) > 0 {
		p.Genres = &v
	}
	if v := komgaValues(q["tag"]); len(v) > 0 {
		p.Tags = &v
	}
	if v := komgaValues(q["publisher"]); len(v) > 0 {
		p.Publishers = &v
	}
	// author is name,role; the filter matches the name.
	var authors []string
	for _, raw := range q["author"] {
		name := raw
		if i := strings.LastIndex(raw, ","); i >= 0 {
			name = raw[:i]
		}
		if name = strings.TrimSpace(name); name != "" {
			authors = append(authors, name)
		}
	}
	if len(authors) > 0 {
		p.Authors = &authors
	}
	if rs := komgaReadStates(q["read_status"]); len(rs) > 0 {
		p.ReadState = &rs
	}
	crit, desc := komgaSort(q)
	p.Desc = &desc
	switch crit {
	case "createdDate":
		v := Created
		p.Sort = &v
	case "lastModifiedDate":
		v := Updated
		p.Sort = &v
	}
	return p, komgaValues(q["library_id"])
}

// komgaSeriesQuery runs ListSeries and returns the series with the total
// count (the query's own count, which ignores Limit and Offset).
func (s *server) komgaSeriesQuery(ctx context.Context, p ListSeriesParams) ([]Series, int, error) {
	resp, err := s.ListSeries(ctx, ListSeriesRequestObject{Params: p})
	if err != nil {
		return nil, 0, err
	}
	out := resp.(ListSeries200JSONResponse)
	total := len(out.Body)
	if out.Headers.XTotalCount != nil {
		total = *out.Headers.XTotalCount
	}
	return out.Body, total, nil
}

// komgaSeriesPage returns page pg of the series matching p, and the total.
func (s *server) komgaSeriesPage(ctx context.Context, p ListSeriesParams, libs []string, pg komgaPaging) ([]Series, int, error) {
	if len(libs) > 1 {
		// ListSeries takes one library; filter the others here.
		p.LibraryId, p.Limit, p.Offset = nil, nil, nil
		all, _, err := s.komgaSeriesQuery(ctx, p)
		if err != nil {
			return nil, 0, err
		}
		keep := map[string]bool{}
		for _, l := range libs {
			keep[l] = true
		}
		var in []Series
		for _, x := range all {
			if keep[x.LibraryId] {
				in = append(in, x)
			}
		}
		return komgaSlice(in, pg), len(in), nil
	}
	if len(libs) == 1 {
		p.LibraryId = &libs[0]
	}
	if !pg.unpaged && pg.size <= komgaMaxLimit {
		lim, off := pg.size, pg.page*pg.size
		p.Limit, p.Offset = &lim, &off
		return s.komgaSeriesQuery(ctx, p)
	}
	all, total, err := s.komgaSeriesQuery(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	return komgaSlice(all, pg), total, nil
}

func (s *server) komgaSeriesList(w http.ResponseWriter, r *http.Request) {
	q, pg := r.URL.Query(), komgaPagingOf(r)
	if komgaDeletedOnly(q) {
		writeJSON(w, http.StatusOK, komgaWrap([]KomgaSeriesDto{}, 0, pg))
		return
	}
	p, libs := komgaSeriesParams(q)
	items, total, err := s.komgaSeriesPage(r.Context(), p, libs, pg)
	if err != nil {
		s.komgaFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, komgaWrap(komgaMap(items, komgaSeries), total, pg))
}

func (s *server) komgaSeriesOne(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	x, ok, err := s.oneSeries(ctx, r.PathValue("seriesId"))
	if err != nil {
		s.komgaFail(w, r, err)
		return
	}
	if !ok {
		writeError(ctx, w, http.StatusNotFound, "not_found.series")
		return
	}
	writeJSON(w, http.StatusOK, komgaSeries(x))
}

// komgaMediaFilter is the media_status condition on books b. Without a known
// value it keeps the READY books only.
func komgaMediaFilter(q url.Values) (string, []any) {
	var st []any
	for _, v := range komgaValues(q["media_status"]) {
		switch strings.ToUpper(v) {
		case "READY":
			st = append(st, "ready")
		case "ERROR":
			st = append(st, "error")
		case "UNSUPPORTED":
			st = append(st, "unsupported")
		case "OUTDATED":
			st = append(st, "outdated")
		}
	}
	if len(st) == 0 {
		return ` AND b.media_status = 'ready'`, nil
	}
	return ` AND b.media_status IN (` + placeholders(len(st)) + `)`, st
}

// komgaReadWhere is the read_status condition on books, through the caller's
// progress row rp (see bookQuery).
func komgaReadWhere(states []ReadState) string {
	var alts []string
	for _, st := range states {
		switch st {
		case Unread:
			alts = append(alts, `rp.book_id IS NULL`)
		case InProgress:
			alts = append(alts, `(rp.book_id IS NOT NULL AND rp.completed = 0)`)
		case Read:
			alts = append(alts, `rp.completed = 1`)
		}
	}
	if len(alts) == 0 {
		return ``
	}
	return ` AND (` + strings.Join(alts, ` OR `) + `)`
}

// komgaBooks runs bookQuery + where + tail and returns page pg of the books
// and the total. whereArgs and tailArgs are the placeholders of where and
// tail, in order.
func (s *server) komgaBooks(ctx context.Context, where string, whereArgs []any, tail string, tailArgs []any, pg komgaPaging) ([]Book, int, error) {
	q, base := bookQuery(ctx)
	base = append(base, whereArgs...)
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM (`+q+where+`)`, base...).Scan(&total); err != nil {
		return nil, 0, err
	}
	text := q + where + ` ` + tail
	args := append(append([]any{}, base...), tailArgs...)
	if !pg.unpaged {
		text += ` LIMIT ? OFFSET ?`
		args = append(args, pg.size, pg.page*pg.size)
	}
	rows, err := s.DB.QueryContext(ctx, text, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Book{}
	for rows.Next() {
		x, err := scanBook(ctx, rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, x)
	}
	return out, total, rows.Err()
}

func (s *server) komgaSeriesBooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("seriesId")
	if ok, err := s.visible(ctx, "series", id); err != nil {
		s.komgaFail(w, r, err)
		return
	} else if !ok {
		writeError(ctx, w, http.StatusNotFound, "not_found.series")
		return
	}
	pg := komgaPagingOf(r)
	if komgaDeletedOnly(r.URL.Query()) {
		writeJSON(w, http.StatusOK, komgaWrap([]KomgaBookDto{}, 0, pg))
		return
	}
	mw, margs := komgaMediaFilter(r.URL.Query())
	items, total, err := s.komgaBooks(ctx, ` AND b.series_id = ?`+mw, append([]any{id}, margs...),
		`ORDER BY b.number_sort, b.path`, nil, pg)
	if err != nil {
		s.komgaFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, komgaWrap(komgaMap(items, komgaBook), total, pg))
}

// ─── Books ───────────────────────────────────────────────────────────────────

// komgaBookList is GET /books: the books the caller may see, filtered by
// search, library_id, read_status and media_status.
func (s *server) komgaBookList(w http.ResponseWriter, r *http.Request) {
	q, pg := r.URL.Query(), komgaPagingOf(r)
	if komgaDeletedOnly(q) {
		writeJSON(w, http.StatusOK, komgaWrap([]KomgaBookDto{}, 0, pg))
		return
	}
	var where string
	var args []any
	if v := strings.TrimSpace(q.Get("search")); v != "" {
		where += ` AND b.id IN (SELECT ref FROM search_index WHERE kind = 'book' AND text LIKE ? ESCAPE '\')`
		args = append(args, likePattern(v))
	}
	if libs := komgaValues(q["library_id"]); len(libs) > 0 {
		where += ` AND b.library_id IN (` + placeholders(len(libs)) + `)`
		for _, l := range libs {
			args = append(args, l)
		}
	}
	where += komgaReadWhere(komgaReadStates(q["read_status"]))
	mw, margs := komgaMediaFilter(q)
	where += mw
	args = append(args, margs...)

	order := `b.title`
	dir := ` ASC`
	crit, desc := komgaSort(q)
	if desc {
		dir = ` DESC`
	}
	switch crit {
	case "name":
		order = `b.title`
	case "createdDate":
		order = `b.created_at`
	}
	items, total, err := s.komgaBooks(r.Context(), where, args, `ORDER BY `+order+dir+`, b.number_sort, b.id`, nil, pg)
	if err != nil {
		s.komgaFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, komgaWrap(komgaMap(items, komgaBook), total, pg))
}

func (s *server) komgaBookOne(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	x, ok, err := s.oneBook(ctx, r.PathValue("bookId"))
	if err != nil {
		s.komgaFail(w, r, err)
		return
	}
	if !ok {
		writeError(ctx, w, http.StatusNotFound, "not_found.book")
		return
	}
	writeJSON(w, http.StatusOK, komgaBook(x))
}

// komgaPages is the book's page list, numbered from 1, as a bare array.
func (s *server) komgaPages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("bookId")
	if ok, err := s.visible(ctx, "books", id); err != nil {
		s.komgaFail(w, r, err)
		return
	} else if !ok {
		writeError(ctx, w, http.StatusNotFound, "not_found.book")
		return
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT p.number, p.entry, p.media_type, p.width, p.height
		FROM pages p JOIN books b ON b.id = p.book_id
		WHERE p.book_id = ? AND b.media_status = 'ready' ORDER BY p.number`, id)
	if err != nil {
		s.komgaFail(w, r, err)
		return
	}
	defer rows.Close()
	out := []KomgaPageDto{}
	for rows.Next() {
		var p KomgaPageDto
		var entry string
		if err := rows.Scan(&p.Number, &entry, &p.MediaType, &p.Width, &p.Height); err != nil {
			s.komgaFail(w, r, err)
			return
		}
		p.FileName = path.Base(entry)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		s.komgaFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// komgaPage serves page {page} (1-based) of a book. Without convert=png it is
// the main API's page, unchanged. With it the page is decoded and re-encoded
// as PNG; a page no decoder reads answers 415.
func (s *server) komgaPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("convert") != "png" {
		s.servePage(w, r)
		return
	}
	ctx := r.Context()
	n, ok := s.pageAllowed(w, r)
	if !ok {
		return
	}
	p, err := s.Library.OpenPage(ctx, r.PathValue("bookId"), n)
	if errors.Is(err, library.ErrNotFound) {
		writeError(ctx, w, http.StatusNotFound, "not_found.page")
		return
	}
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.page_unreadable")
		return
	}
	defer p.Close()
	img, _, err := image.Decode(p.Body)
	if err != nil {
		writeError(ctx, w, http.StatusUnsupportedMediaType, "unsupported.page_convert")
		return
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.page_unreadable")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "image/png")
	h.Set("Content-Length", strconv.Itoa(buf.Len()))
	h.Set("Cache-Control", "private, no-cache")
	h.Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(buf.Bytes())
}

// ─── Read lists ──────────────────────────────────────────────────────────────

func (s *server) komgaReadLists(w http.ResponseWriter, r *http.Request) {
	q, pg := r.URL.Query(), komgaPagingOf(r)
	if komgaDeletedOnly(q) {
		writeJSON(w, http.StatusOK, komgaWrap([]KomgaReadListDto{}, 0, pg))
		return
	}
	where, args := "", []any(nil)
	if v := strings.TrimSpace(q.Get("search")); v != "" {
		where, args = ` WHERE name LIKE ? ESCAPE '\'`, []any{likePattern(v)}
	}
	all, err := s.readLists(r.Context(), where, args...)
	if err != nil {
		s.komgaFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, komgaWrap(komgaMap(komgaSlice(all, pg), komgaReadList), len(all), pg))
}

// komgaFindReadList returns the visible read list {id}, or nil.
func (s *server) komgaFindReadList(r *http.Request) (*ReadList, error) {
	ls, err := s.readLists(r.Context(), ` WHERE id = ?`, r.PathValue("readListId"))
	if err != nil || len(ls) == 0 {
		return nil, err
	}
	return &ls[0], nil
}

func (s *server) komgaReadListOne(w http.ResponseWriter, r *http.Request) {
	l, err := s.komgaFindReadList(r)
	if err != nil {
		s.komgaFail(w, r, err)
		return
	}
	if l == nil {
		writeError(r.Context(), w, http.StatusNotFound, "not_found.read_list")
		return
	}
	writeJSON(w, http.StatusOK, komgaReadList(*l))
}

func (s *server) komgaReadListBooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l, err := s.komgaFindReadList(r)
	if err != nil {
		s.komgaFail(w, r, err)
		return
	}
	if l == nil {
		writeError(ctx, w, http.StatusNotFound, "not_found.read_list")
		return
	}
	pg := komgaPagingOf(r)
	if komgaDeletedOnly(r.URL.Query()) {
		writeJSON(w, http.StatusOK, komgaWrap([]KomgaBookDto{}, 0, pg))
		return
	}
	mw, margs := komgaMediaFilter(r.URL.Query())
	where := ` AND b.id IN (SELECT book_id FROM read_list_books WHERE read_list_id = ?)` + mw
	items, total, err := s.komgaBooks(ctx, where, append([]any{l.Id}, margs...),
		`ORDER BY (SELECT position FROM read_list_books WHERE read_list_id = ? AND book_id = b.id)`, []any{l.Id}, pg)
	if err != nil {
		s.komgaFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, komgaWrap(komgaMap(items, komgaBook), total, pg))
}

// komgaReadListThumbnail is the thumbnail of the list's first visible book.
func (s *server) komgaReadListThumbnail(w http.ResponseWriter, r *http.Request) {
	l, err := s.komgaFindReadList(r)
	if err != nil {
		s.komgaFail(w, r, err)
		return
	}
	if l == nil {
		writeError(r.Context(), w, http.StatusNotFound, "not_found.read_list")
		return
	}
	if len(l.BookIds) == 0 {
		writeError(r.Context(), w, http.StatusNotFound, "not_found.cover")
		return
	}
	// The members are already limited to books the caller may see.
	s.sendThumbnail(w, r, "book", l.BookIds[0])
}

// ─── Collections ─────────────────────────────────────────────────────────────

func (s *server) komgaCollections(w http.ResponseWriter, r *http.Request) {
	q, pg := r.URL.Query(), komgaPagingOf(r)
	if komgaDeletedOnly(q) {
		writeJSON(w, http.StatusOK, komgaWrap([]KomgaCollectionDto{}, 0, pg))
		return
	}
	where, args := "", []any(nil)
	if v := strings.TrimSpace(q.Get("search")); v != "" {
		where, args = ` WHERE name LIKE ? ESCAPE '\'`, []any{likePattern(v)}
	}
	all, err := s.collections(r.Context(), where, args...)
	if err != nil {
		s.komgaFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, komgaWrap(komgaMap(komgaSlice(all, pg), komgaCollection), len(all), pg))
}

// komgaCollectionSeries lists a collection's visible series, in its order.
func (s *server) komgaCollectionSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	resp, err := s.ListCollectionSeries(ctx, ListCollectionSeriesRequestObject{CollectionId: r.PathValue("collectionId")})
	if err != nil {
		s.komgaFail(w, r, err)
		return
	}
	all, ok := resp.(ListCollectionSeries200JSONResponse)
	if !ok {
		writeError(ctx, w, http.StatusNotFound, "not_found.collection")
		return
	}
	pg := komgaPagingOf(r)
	writeJSON(w, http.StatusOK, komgaWrap(komgaMap(komgaSlice([]Series(all), pg), komgaSeries), len(all), pg))
}

// ─── Filter options ──────────────────────────────────────────────────────────

// komgaStrings answers the distinct values of one series column (or JSON
// array) that the caller may see, as a bare array.
func (s *server) komgaStrings(expr, from, cond string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vals, err := s.facetStrings(r.Context(), nil, expr, from, cond)
		if err != nil {
			s.komgaFail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, vals)
	}
}

// komgaAuthorList answers the distinct author names with their roles.
func (s *server) komgaAuthorList(w http.ResponseWriter, r *http.Request) {
	scope, args := libScope(r.Context(), "s.library_id")
	rows, err := s.DB.QueryContext(r.Context(), `SELECT DISTINCT json_extract(a.value, '$.name'), json_extract(a.value, '$.role')
		FROM series s, json_each(s.authors) a
		WHERE s.deleted_at IS NULL AND `+scope+` AND json_extract(a.value, '$.name') <> ''
		ORDER BY 1, 2`, args...)
	if err != nil {
		s.komgaFail(w, r, err)
		return
	}
	defer rows.Close()
	out := []KomgaAuthorDto{}
	for rows.Next() {
		var a KomgaAuthorDto
		if err := rows.Scan(&a.Name, &a.Role); err != nil {
			s.komgaFail(w, r, err)
			return
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		s.komgaFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
