package httpapi

import (
	"context"
	"database/sql"
	"encoding/xml"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// OPDS 1.2 catalog (see docs/dev/opds-komga-spec.md). Feeds are Atom documents built
// with encoding/xml. Series and books are read only through listSeries,
// listBooks and oneSeries/oneBook, so the caller's libraries decide what
// appears; collections and read lists keep their visibility rule.

const (
	opdsPageSize    = 50
	opdsCatalogPath = "/opds/v1.2/catalog"
	opdsSearchPath  = "/opds/v1.2/search.xml"
	opdsNavType     = "application/atom+xml;profile=opds-catalog;kind=navigation"
	opdsAcqType     = "application/atom+xml;profile=opds-catalog;kind=acquisition"
	opdsSearchType  = "application/opensearchdescription+xml"
	opdsImageType   = "image/jpeg"

	relSubsection = "subsection"
	relStart      = "start"
	relSearch     = "search"
	relSelf       = "self"
	relNext       = "next"
	relPrevious   = "previous"
	relAcquire    = "http://opds-spec.org/acquisition"
	relImage      = "http://opds-spec.org/image"
	relThumbnail  = "http://opds-spec.org/image/thumbnail"
	relPSE        = "http://vaemendis.net/opds-pse/stream"
)

// ─── Atom and OpenSearch documents ───────────────────────────────────────────

// opdsFeed is an Atom feed. The prefixed names (dcterms, pse, opensearch) are
// written as they are, with their namespaces declared on the root.
type opdsFeed struct {
	XMLName xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	NSOPDS  string      `xml:"xmlns:opds,attr"`
	NSDC    string      `xml:"xmlns:dcterms,attr"`
	NSPSE   string      `xml:"xmlns:pse,attr"`
	NSOS    string      `xml:"xmlns:opensearch,attr"`
	ID      string      `xml:"id"`
	Title   string      `xml:"title"`
	Updated string      `xml:"updated"`
	Links   []opdsLink  `xml:"link"`
	Entries []opdsEntry `xml:"entry"`
}

type opdsEntry struct {
	ID      string       `xml:"id"`
	Title   string       `xml:"title"`
	Updated string       `xml:"updated"`
	Issued  string       `xml:"dcterms:issued,omitempty"`
	Authors []opdsPerson `xml:"author"`
	Summary string       `xml:"summary,omitempty"`
	Links   []opdsLink   `xml:"link"`
}

type opdsPerson struct {
	Name string `xml:"name"`
}

type opdsLink struct {
	Rel             string `xml:"rel,attr,omitempty"`
	Href            string `xml:"href,attr"`
	Type            string `xml:"type,attr,omitempty"`
	Length          int64  `xml:"length,attr,omitempty"`
	PSECount        *int   `xml:"pse:count,attr,omitempty"`
	PSELastRead     *int   `xml:"pse:lastRead,attr,omitempty"`
	PSELastReadDate string `xml:"pse:lastReadDate,attr,omitempty"`
}

// opdsSearchDoc is the OpenSearch description behind the search link.
type opdsSearchDoc struct {
	XMLName        xml.Name      `xml:"http://a9.com/-/spec/opensearch/1.1/ OpenSearchDescription"`
	ShortName      string        `xml:"ShortName"`
	Description    string        `xml:"Description"`
	InputEncoding  string        `xml:"InputEncoding"`
	OutputEncoding string        `xml:"OutputEncoding"`
	URL            opdsSearchURL `xml:"Url"`
}

type opdsSearchURL struct {
	Type     string `xml:"type,attr"`
	Template string `xml:"template,attr"`
}

// ─── Routes ──────────────────────────────────────────────────────────────────

// opdsRoutes registers the OPDS 1.2 catalog under /opds/ (full paths).
func (s *server) opdsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /opds/{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, opdsCatalogPath, http.StatusFound)
	})
	mux.HandleFunc("GET /opds/v1.2/catalog", s.opdsCatalog)
	mux.HandleFunc("GET /opds/v1.2/keep-reading", s.opdsKeepReading)
	mux.HandleFunc("GET /opds/v1.2/series/latest", s.opdsLatestSeries)
	mux.HandleFunc("GET /opds/v1.2/books/latest", s.opdsLatestBooks)
	mux.HandleFunc("GET /opds/v1.2/libraries", s.opdsLibraries)
	mux.HandleFunc("GET /opds/v1.2/libraries/{libraryId}", s.opdsLibrary)
	mux.HandleFunc("GET /opds/v1.2/collections", s.opdsCollections)
	mux.HandleFunc("GET /opds/v1.2/collections/{collectionId}", s.opdsCollection)
	mux.HandleFunc("GET /opds/v1.2/readlists", s.opdsReadLists)
	mux.HandleFunc("GET /opds/v1.2/readlists/{readListId}", s.opdsReadList)
	mux.HandleFunc("GET /opds/v1.2/series/{seriesId}", s.opdsSeries)
	mux.HandleFunc("GET /opds/v1.2/search.xml", s.opdsSearchDoc)
	mux.HandleFunc("GET /opds/v1.2/search", s.opdsSearch)
	mux.HandleFunc("GET /opds/v1.2/books/{bookId}/pages/{pageNumber}", s.opdsPage)
	mux.HandleFunc("GET /opds/v1.2/series/{seriesId}/thumbnail", s.serveThumbnail("series", true))
	mux.HandleFunc("GET /opds/v1.2/books/{bookId}/thumbnail", s.serveThumbnail("book", true))
	mux.HandleFunc("GET /opds/v1.2/books/{bookId}/file", s.serveBookFile)
}

// ─── Feeds ───────────────────────────────────────────────────────────────────

// opdsFeedSpec describes the feed being written. query holds the parameters
// the paging links keep (q for search).
type opdsFeedSpec struct {
	id, title, kind, path string
	query                 url.Values
	page                  int
	more                  bool
}

func opdsHref(path string, q url.Values, page int) string {
	v := url.Values{}
	for k, vs := range q {
		v[k] = vs
	}
	if page > 0 {
		v.Set("page", strconv.Itoa(page))
	}
	if len(v) == 0 {
		return path
	}
	return path + "?" + v.Encode()
}

// writeFeed sends a feed with its self, start, search and paging links.
func (s *server) writeFeed(ctx context.Context, w http.ResponseWriter, spec opdsFeedSpec, entries []opdsEntry) {
	typ := opdsNavType
	if spec.kind == "acquisition" {
		typ = opdsAcqType
	}
	links := []opdsLink{
		{Rel: relSelf, Href: opdsHref(spec.path, spec.query, spec.page), Type: typ},
		{Rel: relStart, Href: opdsCatalogPath, Type: opdsNavType},
		{Rel: relSearch, Href: opdsSearchPath, Type: opdsSearchType},
	}
	if spec.page > 0 {
		links = append(links, opdsLink{Rel: relPrevious, Href: opdsHref(spec.path, spec.query, spec.page-1), Type: typ})
	}
	if spec.more {
		links = append(links, opdsLink{Rel: relNext, Href: opdsHref(spec.path, spec.query, spec.page+1), Type: typ})
	}
	feed := opdsFeed{
		NSOPDS: "http://opds-spec.org/2010/catalog", NSDC: "http://purl.org/dc/terms/",
		NSPSE: "http://vaemendis.net/opds-pse/ns", NSOS: "http://a9.com/-/spec/opensearch/1.1/",
		ID: spec.id, Title: spec.title, Updated: opdsNow(),
		Links: links, Entries: entries,
	}
	out, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.error")
		return
	}
	h := w.Header()
	h.Set("Content-Type", typ)
	h.Set("Cache-Control", "private, no-cache")
	h.Set("X-Content-Type-Options", "nosniff")
	w.Write([]byte(xml.Header))
	w.Write(out)
}

// opdsPageOf reads ?page= (0-based). A missing or bad value is the first page.
func opdsPageOf(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || n < 0 {
		return 0
	}
	return min(n, 1<<20)
}

// opdsWindow is the LIMIT/OFFSET pair for one page, plus one row to tell
// whether a next page exists.
func opdsWindow(page int) (limit, offset int) {
	return opdsPageSize + 1, page * opdsPageSize
}

func trimPage[T any](xs []T) ([]T, bool) {
	if len(xs) > opdsPageSize {
		return xs[:opdsPageSize], true
	}
	return xs, false
}

// opdsSeriesWindow lists one page of visible series. tail must end with
// "LIMIT ? OFFSET ?"; args are the placeholders before it.
func (s *server) opdsSeriesWindow(ctx context.Context, r *http.Request, where, tail string, args ...any) ([]Series, bool, error) {
	limit, offset := opdsWindow(opdsPageOf(r))
	out, err := s.listSeries(ctx, where, tail, append(args, limit, offset)...)
	if err != nil {
		return nil, false, err
	}
	out, more := trimPage(out)
	return out, more, nil
}

// opdsBookWindow is opdsSeriesWindow for books.
func (s *server) opdsBookWindow(ctx context.Context, r *http.Request, where, tail string, args ...any) ([]Book, bool, error) {
	limit, offset := opdsWindow(opdsPageOf(r))
	out, err := s.listBooks(ctx, where, tail, append(args, limit, offset)...)
	if err != nil {
		return nil, false, err
	}
	out, more := trimPage(out)
	return out, more, nil
}

// ─── Entries ─────────────────────────────────────────────────────────────────

func opdsNow() string { return time.Now().UTC().Format(time.RFC3339) }

// atomTime turns a stored time into RFC 3339 (UTC), or the current time when
// it cannot be read.
func atomTime(v string) string {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return opdsNow()
	}
	return t.UTC().Format(time.RFC3339)
}

func opdsPeople(as []Author) []opdsPerson {
	out := make([]opdsPerson, 0, len(as))
	for _, a := range as {
		out = append(out, opdsPerson{Name: a.Name})
	}
	return out
}

func opdsNavEntry(id, title, href, typ string) opdsEntry {
	return opdsEntry{ID: id, Title: title, Updated: opdsNow(),
		Links: []opdsLink{{Rel: relSubsection, Href: href, Type: typ}}}
}

func opdsSeriesEntry(x Series) opdsEntry {
	base := "/opds/v1.2/series/" + x.Id
	return opdsEntry{
		ID:      "urn:mars:series:" + x.Id,
		Title:   x.Title,
		Updated: atomTime(x.UpdatedAt),
		Authors: opdsPeople(x.Authors),
		Summary: x.Summary,
		Links: []opdsLink{
			{Rel: relSubsection, Href: base, Type: opdsAcqType},
			{Rel: relImage, Href: base + "/thumbnail", Type: opdsImageType},
			{Rel: relThumbnail, Href: base + "/thumbnail", Type: opdsImageType},
		},
	}
}

// bookMediaType is the type of a book's original file.
func bookMediaType(f BookFormat) string {
	switch f {
	case Cbz:
		return "application/zip"
	case Cbr:
		return "application/x-rar-compressed"
	case Cb7:
		return "application/x-7z-compressed"
	case Pdf:
		return "application/pdf"
	case Epub:
		return "application/epub+zip"
	}
	return "application/octet-stream"
}

// opdsBookEntry is an acquisition entry. The download link appears only for
// callers who may download, the page stream only for those who may read.
func opdsBookEntry(ctx context.Context, b Book) opdsEntry {
	base := "/opds/v1.2/books/" + b.Id
	e := opdsEntry{
		ID:      "urn:mars:book:" + b.Id,
		Title:   b.Title,
		Updated: atomTime(b.CreatedAt),
		Authors: opdsPeople(b.Authors),
		Summary: b.Summary,
	}
	if b.ReleaseDate != nil {
		e.Issued = *b.ReleaseDate
	}
	if canDownload(ctx) {
		e.Links = append(e.Links, opdsLink{Rel: relAcquire, Href: base + "/file",
			Type: bookMediaType(b.Format), Length: b.SizeBytes})
	}
	e.Links = append(e.Links,
		opdsLink{Rel: relImage, Href: base + "/thumbnail", Type: opdsImageType},
		opdsLink{Rel: relThumbnail, Href: base + "/thumbnail", Type: opdsImageType})
	if canStream(ctx) && b.MediaStatus == MediaStatusReady && b.PageCount > 0 {
		count := b.PageCount
		pse := opdsLink{Rel: relPSE, Href: base + "/pages/{pageNumber}?maxWidth={maxWidth}",
			Type: opdsImageType, PSECount: &count}
		if p := b.Progress; p != nil && p.Page >= 1 {
			last := p.Page - 1 // progress counts pages from 1; the stream from 0
			pse.PSELastRead = &last
			if p.ReadAt != "" {
				pse.PSELastReadDate = atomTime(p.ReadAt)
			}
		}
		e.Links = append(e.Links, pse)
	}
	return e
}

func opdsBookEntries(ctx context.Context, books []Book) []opdsEntry {
	out := make([]opdsEntry, 0, len(books))
	for _, b := range books {
		out = append(out, opdsBookEntry(ctx, b))
	}
	return out
}

func opdsSeriesEntries(xs []Series) []opdsEntry {
	out := make([]opdsEntry, 0, len(xs))
	for _, x := range xs {
		out = append(out, opdsSeriesEntry(x))
	}
	return out
}

// ─── Handlers ────────────────────────────────────────────────────────────────

func (s *server) opdsCatalog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	entries := []opdsEntry{
		opdsNavEntry("urn:mars:feed:keep-reading", msg(ctx, "opds.keep_reading"), "/opds/v1.2/keep-reading", opdsAcqType),
		opdsNavEntry("urn:mars:feed:series-latest", msg(ctx, "opds.latest_series"), "/opds/v1.2/series/latest", opdsNavType),
		opdsNavEntry("urn:mars:feed:books-latest", msg(ctx, "opds.latest_books"), "/opds/v1.2/books/latest", opdsAcqType),
		opdsNavEntry("urn:mars:feed:libraries", msg(ctx, "opds.libraries"), "/opds/v1.2/libraries", opdsNavType),
		opdsNavEntry("urn:mars:feed:collections", msg(ctx, "opds.collections"), "/opds/v1.2/collections", opdsNavType),
		opdsNavEntry("urn:mars:feed:readlists", msg(ctx, "opds.readlists"), "/opds/v1.2/readlists", opdsNavType),
	}
	s.writeFeed(ctx, w, opdsFeedSpec{id: "urn:mars:feed:catalog", title: msg(ctx, "opds.catalog_title"),
		kind: "navigation", path: opdsCatalogPath}, entries)
}

func (s *server) opdsKeepReading(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	books, more, err := s.opdsBookWindow(ctx, r, ` AND rp.completed = 0`, `ORDER BY rp.read_at DESC, b.id LIMIT ? OFFSET ?`)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.error")
		return
	}
	s.writeFeed(ctx, w, opdsFeedSpec{id: "urn:mars:feed:keep-reading", title: msg(ctx, "opds.keep_reading"),
		kind: "acquisition", path: "/opds/v1.2/keep-reading", page: opdsPageOf(r), more: more},
		opdsBookEntries(ctx, books))
}

func (s *server) opdsLatestSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	xs, more, err := s.opdsSeriesWindow(ctx, r, ``, `ORDER BY s.created_at DESC, s.id DESC LIMIT ? OFFSET ?`)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.error")
		return
	}
	s.writeFeed(ctx, w, opdsFeedSpec{id: "urn:mars:feed:series-latest", title: msg(ctx, "opds.latest_series"),
		kind: "navigation", path: "/opds/v1.2/series/latest", page: opdsPageOf(r), more: more},
		opdsSeriesEntries(xs))
}

func (s *server) opdsLatestBooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	books, more, err := s.opdsBookWindow(ctx, r, ``, `ORDER BY b.created_at DESC, b.id DESC LIMIT ? OFFSET ?`)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.error")
		return
	}
	s.writeFeed(ctx, w, opdsFeedSpec{id: "urn:mars:feed:books-latest", title: msg(ctx, "opds.latest_books"),
		kind: "acquisition", path: "/opds/v1.2/books/latest", page: opdsPageOf(r), more: more},
		opdsBookEntries(ctx, books))
}

func (s *server) opdsLibraries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cond, args := libScope(ctx, "l.id")
	rows, err := s.DB.QueryContext(ctx, `SELECT `+libraryCols+` FROM libraries l WHERE `+cond+` ORDER BY l.name, l.id`, args...)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.error")
		return
	}
	defer rows.Close()
	entries := []opdsEntry{}
	for rows.Next() {
		l, err := scanLibrary(rows)
		if err != nil {
			writeError(ctx, w, http.StatusInternalServerError, "server.error")
			return
		}
		entries = append(entries, opdsNavEntry("urn:mars:library:"+l.Id, l.Name, "/opds/v1.2/libraries/"+l.Id, opdsNavType))
	}
	if rows.Err() != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.error")
		return
	}
	s.writeFeed(ctx, w, opdsFeedSpec{id: "urn:mars:feed:libraries", title: msg(ctx, "opds.libraries"),
		kind: "navigation", path: "/opds/v1.2/libraries"}, entries)
}

func (s *server) opdsLibrary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("libraryId")
	l, err := s.getLibrary(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(ctx, w, http.StatusNotFound, "not_found.library")
		return
	}
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.error")
		return
	}
	xs, more, err := s.opdsSeriesWindow(ctx, r, ` AND s.library_id = ?`, `ORDER BY s.title_sort, s.id LIMIT ? OFFSET ?`, id)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.error")
		return
	}
	s.writeFeed(ctx, w, opdsFeedSpec{id: "urn:mars:library:" + id, title: l.Name,
		kind: "navigation", path: "/opds/v1.2/libraries/" + id, page: opdsPageOf(r), more: more},
		opdsSeriesEntries(xs))
}

func (s *server) opdsCollections(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cs, err := s.collections(ctx, "")
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.error")
		return
	}
	entries := make([]opdsEntry, 0, len(cs))
	for _, c := range cs {
		entries = append(entries, opdsNavEntry("urn:mars:collection:"+c.Id, c.Name, "/opds/v1.2/collections/"+c.Id, opdsNavType))
	}
	s.writeFeed(ctx, w, opdsFeedSpec{id: "urn:mars:feed:collections", title: msg(ctx, "opds.collections"),
		kind: "navigation", path: "/opds/v1.2/collections"}, entries)
}

func (s *server) opdsCollection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("collectionId")
	cs, err := s.collections(ctx, " WHERE id = ?", id)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.error")
		return
	}
	if len(cs) == 0 {
		writeError(ctx, w, http.StatusNotFound, "not_found.collection")
		return
	}
	c := cs[0]
	// Same order as ListCollectionSeries: the saved order when ordered, else by title.
	where := ` AND s.id IN (SELECT series_id FROM collection_series WHERE collection_id = ?)`
	tail := `ORDER BY s.title_sort, s.id LIMIT ? OFFSET ?`
	args := []any{id}
	if c.Ordered {
		tail = `ORDER BY (SELECT position FROM collection_series WHERE collection_id = ? AND series_id = s.id), s.id LIMIT ? OFFSET ?`
		args = append(args, id)
	}
	xs, more, err := s.opdsSeriesWindow(ctx, r, where, tail, args...)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.error")
		return
	}
	s.writeFeed(ctx, w, opdsFeedSpec{id: "urn:mars:collection:" + id, title: c.Name,
		kind: "navigation", path: "/opds/v1.2/collections/" + id, page: opdsPageOf(r), more: more},
		opdsSeriesEntries(xs))
}

func (s *server) opdsReadLists(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ls, err := s.readLists(ctx, "")
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.error")
		return
	}
	entries := make([]opdsEntry, 0, len(ls))
	for _, l := range ls {
		entries = append(entries, opdsNavEntry("urn:mars:readlist:"+l.Id, l.Name, "/opds/v1.2/readlists/"+l.Id, opdsAcqType))
	}
	s.writeFeed(ctx, w, opdsFeedSpec{id: "urn:mars:feed:readlists", title: msg(ctx, "opds.readlists"),
		kind: "navigation", path: "/opds/v1.2/readlists"}, entries)
}

func (s *server) opdsReadList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("readListId")
	ls, err := s.readLists(ctx, " WHERE id = ?", id)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.error")
		return
	}
	if len(ls) == 0 {
		writeError(ctx, w, http.StatusNotFound, "not_found.read_list")
		return
	}
	books, more, err := s.opdsBookWindow(ctx, r,
		` AND b.id IN (SELECT book_id FROM read_list_books WHERE read_list_id = ?)`,
		`ORDER BY (SELECT position FROM read_list_books WHERE read_list_id = ? AND book_id = b.id), b.id LIMIT ? OFFSET ?`,
		id, id)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.error")
		return
	}
	s.writeFeed(ctx, w, opdsFeedSpec{id: "urn:mars:readlist:" + id, title: ls[0].Name,
		kind: "acquisition", path: "/opds/v1.2/readlists/" + id, page: opdsPageOf(r), more: more},
		opdsBookEntries(ctx, books))
}

func (s *server) opdsSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("seriesId")
	x, ok, err := s.oneSeries(ctx, id)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.error")
		return
	}
	if !ok {
		writeError(ctx, w, http.StatusNotFound, "not_found.series")
		return
	}
	books, more, err := s.opdsBookWindow(ctx, r, ` AND b.series_id = ?`, `ORDER BY b.number_sort, b.path LIMIT ? OFFSET ?`, id)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.error")
		return
	}
	s.writeFeed(ctx, w, opdsFeedSpec{id: "urn:mars:series:" + id, title: x.Title,
		kind: "acquisition", path: "/opds/v1.2/series/" + id, page: opdsPageOf(r), more: more},
		opdsBookEntries(ctx, books))
}

func (s *server) opdsSearchDoc(w http.ResponseWriter, r *http.Request) {
	doc := opdsSearchDoc{
		ShortName:      "MARS",
		Description:    msg(r.Context(), "opds.search_description"),
		InputEncoding:  "UTF-8",
		OutputEncoding: "UTF-8",
		URL:            opdsSearchURL{Type: opdsNavType, Template: "/opds/v1.2/search?q={searchTerms}"},
	}
	out, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		writeError(r.Context(), w, http.StatusInternalServerError, "server.error")
		return
	}
	w.Header().Set("Content-Type", opdsSearchType)
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Write([]byte(xml.Header))
	w.Write(out)
}

// opdsSeriesMatch is the search condition of Search (browse.go): trigram
// matching from three characters, LIKE below that.
func opdsSeriesMatch(q string) (string, any) {
	if utf8.RuneCountInString(q) >= 3 {
		return ` AND s.id IN (SELECT ref FROM search_index WHERE kind = 'series' AND text MATCH ?)`,
			`"` + strings.ReplaceAll(q, `"`, `""`) + `"`
	}
	return ` AND s.id IN (SELECT ref FROM search_index WHERE kind = 'series' AND text LIKE ? ESCAPE '\')`, likePattern(q)
}

func (s *server) opdsSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	spec := opdsFeedSpec{id: "urn:mars:feed:search", title: msg(ctx, "opds.search_results", q),
		kind: "navigation", path: "/opds/v1.2/search", query: url.Values{"q": {q}}, page: opdsPageOf(r)}
	if q == "" {
		s.writeFeed(ctx, w, spec, []opdsEntry{})
		return
	}
	where, match := opdsSeriesMatch(q)
	xs, more, err := s.opdsSeriesWindow(ctx, r, where, `ORDER BY s.title_sort, s.id LIMIT ? OFFSET ?`, match)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "server.error")
		return
	}
	spec.more = more
	s.writeFeed(ctx, w, spec, opdsSeriesEntries(xs))
}

// opdsPage serves one page for OPDS-PSE. The stream is 0-based; the page
// handler (servePage) is 1-based, so the number is shifted by one. maxWidth
// is ignored.
func (s *server) opdsPage(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("pageNumber"))
	if err != nil || n < 0 {
		writeError(r.Context(), w, http.StatusNotFound, "not_found.page")
		return
	}
	r.SetPathValue("page", strconv.Itoa(n+1))
	s.servePage(w, r)
}
