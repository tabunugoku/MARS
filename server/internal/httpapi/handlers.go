package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/ids"
	"github.com/tabunugoku/cominavi/server/internal/library"
	"github.com/tabunugoku/cominavi/server/internal/task"
)

// notFound and badRequest build the error bodies of the generated responses:
// a catalog code, and its text in the request's language.
func notFound(ctx context.Context, code string, args ...any) NotFoundJSONResponse {
	return NotFoundJSONResponse{Code: code, Message: msg(ctx, code, args...)}
}

func badRequest(ctx context.Context, code string, args ...any) BadRequestJSONResponse {
	return BadRequestJSONResponse{Code: code, Message: msg(ctx, code, args...)}
}

func strs(s string) []string {
	var out []string
	_ = json.Unmarshal([]byte(s), &out)
	if out == nil {
		out = []string{}
	}
	return out
}

func authors(s string) []Author {
	var out []Author
	_ = json.Unmarshal([]byte(s), &out)
	if out == nil {
		out = []Author{}
	}
	return out
}

func jsonText(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// ─── Libraries ───────────────────────────────────────────────────────────────

const libraryCols = `l.id, l.name, l.root, l.kind, l.scan_interval, l.watch, l.import_comicinfo, l.import_local_artwork, l.exclude, l.last_scan,
	(SELECT count(*) FROM series s WHERE s.library_id = l.id AND s.deleted_at IS NULL),
	(SELECT count(*) FROM books b WHERE b.library_id = l.id AND b.deleted_at IS NULL),
	(SELECT COALESCE(sum(size_bytes), 0) FROM books b WHERE b.library_id = l.id AND b.deleted_at IS NULL)`

type scanner interface{ Scan(...any) error }

func scanLibrary(r scanner) (Library, error) {
	var l Library
	var kind, interval, exclude string
	var last sql.NullString
	err := r.Scan(&l.Id, &l.Name, &l.Root, &kind, &interval, &l.Watch, &l.ImportComicInfo, &l.ImportLocalArtwork, &exclude, &last,
		&l.SeriesCount, &l.BookCount, &l.SizeBytes)
	l.Kind, l.ScanInterval, l.Exclude = LibraryKind(kind), ScanInterval(interval), strs(exclude)
	if last.Valid {
		l.LastScan = &last.String
	}
	return l, err
}

func (s *server) getLibrary(ctx context.Context, id string) (Library, error) {
	cond, args := libScope(ctx, "l.id")
	l, err := scanLibrary(s.DB.QueryRowContext(ctx, `SELECT `+libraryCols+` FROM libraries l WHERE l.id = ? AND `+cond, append([]any{id}, args...)...))
	return s.libraryView(ctx, l), err
}

// libraryView adds the watcher's state and hides where the files live from
// users who cannot manage them.
func (s *server) libraryView(ctx context.Context, l Library) Library {
	// Root, exclusions and watcher state are administrators' business.
	watchErr := ""
	if currentUser(ctx).IsAdmin() {
		if code := s.Library.WatchError(l.Id); code != "" {
			watchErr = msg(ctx, code)
		}
	} else {
		l.Root, l.Exclude = "", []string{}
	}
	l.WatchError = &watchErr
	return l
}

func (s *server) ListLibraries(ctx context.Context, _ ListLibrariesRequestObject) (ListLibrariesResponseObject, error) {
	cond, args := libScope(ctx, "l.id")
	rows, err := s.DB.QueryContext(ctx, `SELECT `+libraryCols+` FROM libraries l WHERE `+cond+` ORDER BY l.name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := ListLibraries200JSONResponse{}
	for rows.Next() {
		l, err := scanLibrary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s.libraryView(ctx, l))
	}
	return out, rows.Err()
}

func (s *server) GetLibrary(ctx context.Context, req GetLibraryRequestObject) (GetLibraryResponseObject, error) {
	l, err := s.getLibrary(ctx, req.LibraryId)
	if errors.Is(err, sql.ErrNoRows) {
		return GetLibrary404JSONResponse{notFound(ctx, "not_found.library")}, nil
	}
	if err != nil {
		return nil, err
	}
	return GetLibrary200JSONResponse(l), nil
}

func validKind(k LibraryKind) bool {
	switch k {
	case LibraryKindManga, LibraryKindWebtoon, LibraryKindComics, LibraryKindBooks:
		return true
	}
	return false
}

func validInterval(i ScanInterval) bool {
	switch i {
	case ScanIntervalOff, ScanIntervalHourly, ScanIntervalDaily, ScanIntervalWeekly:
		return true
	}
	return false
}

// badPattern returns the first exclusion pattern path.Match rejects, or "".
func badPattern(pats []string) string {
	for _, p := range pats {
		if _, err := path.Match(p, ""); err != nil {
			return p
		}
	}
	return ""
}

func (s *server) CreateLibrary(ctx context.Context, req CreateLibraryRequestObject) (CreateLibraryResponseObject, error) {
	b := req.Body
	if b == nil {
		return CreateLibrary400JSONResponse{badRequest(ctx, "bad_request.body_missing")}, nil
	}
	name := strings.TrimSpace(b.Name)
	if name == "" {
		return CreateLibrary400JSONResponse{badRequest(ctx, "bad_request.name_required")}, nil
	}
	if !validKind(b.Kind) {
		return CreateLibrary400JSONResponse{badRequest(ctx, "bad_request.kind")}, nil
	}
	interval := ScanIntervalDaily
	if b.ScanInterval != nil {
		if !validInterval(*b.ScanInterval) {
			return CreateLibrary400JSONResponse{badRequest(ctx, "bad_request.scan_interval")}, nil
		}
		interval = *b.ScanInterval
	}
	exclude := []string{}
	if b.Exclude != nil {
		exclude = *b.Exclude
	}
	if bad := badPattern(exclude); bad != "" {
		return CreateLibrary400JSONResponse{badRequest(ctx, "bad_request.exclude_pattern", bad)}, nil
	}
	if !filepath.IsAbs(b.Root) {
		return CreateLibrary400JSONResponse{badRequest(ctx, "bad_request.root_not_absolute")}, nil
	}
	root := filepath.Clean(b.Root)
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return CreateLibrary400JSONResponse{badRequest(ctx, "bad_request.root_missing")}, nil
	}
	importCI := true
	if b.ImportComicInfo != nil {
		importCI = *b.ImportComicInfo
	}
	watch := false
	if b.Watch != nil {
		watch = *b.Watch
	}
	id := ids.New()
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO libraries (id, name, root, kind, scan_interval, watch, import_comicinfo, exclude, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, root, string(b.Kind), string(interval), watch, importCI, jsonText(exclude), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return CreateLibrary400JSONResponse{badRequest(ctx, "bad_request.root_taken")}, nil
		}
		return nil, err
	}
	if _, err := s.Library.QueueScan(ctx, id); err != nil {
		return nil, err
	}
	l, err := s.getLibrary(ctx, id)
	if err != nil {
		return nil, err
	}
	return CreateLibrary201JSONResponse(l), nil
}

func (s *server) UpdateLibrary(ctx context.Context, req UpdateLibraryRequestObject) (UpdateLibraryResponseObject, error) {
	if _, err := s.getLibrary(ctx, req.LibraryId); errors.Is(err, sql.ErrNoRows) {
		return UpdateLibrary404JSONResponse{notFound(ctx, "not_found.library")}, nil
	} else if err != nil {
		return nil, err
	}
	b := req.Body
	if b == nil {
		return UpdateLibrary400JSONResponse{badRequest(ctx, "bad_request.body_missing")}, nil
	}
	set, args := []string{}, []any{}
	if b.Name != nil {
		n := strings.TrimSpace(*b.Name)
		if n == "" {
			return UpdateLibrary400JSONResponse{badRequest(ctx, "bad_request.name_required")}, nil
		}
		set, args = append(set, "name = ?"), append(args, n)
	}
	if b.Kind != nil {
		if !validKind(*b.Kind) {
			return UpdateLibrary400JSONResponse{badRequest(ctx, "bad_request.kind")}, nil
		}
		set, args = append(set, "kind = ?"), append(args, string(*b.Kind))
	}
	if b.ScanInterval != nil {
		if !validInterval(*b.ScanInterval) {
			return UpdateLibrary400JSONResponse{badRequest(ctx, "bad_request.scan_interval")}, nil
		}
		set, args = append(set, "scan_interval = ?"), append(args, string(*b.ScanInterval))
	}
	if b.Watch != nil {
		set, args = append(set, "watch = ?"), append(args, *b.Watch)
	}
	if b.ImportComicInfo != nil {
		set, args = append(set, "import_comicinfo = ?"), append(args, *b.ImportComicInfo)
	}
	if b.Exclude != nil {
		if bad := badPattern(*b.Exclude); bad != "" {
			return UpdateLibrary400JSONResponse{badRequest(ctx, "bad_request.exclude_pattern", bad)}, nil
		}
		set, args = append(set, "exclude = ?"), append(args, jsonText(*b.Exclude))
	}
	if len(set) > 0 {
		args = append(args, req.LibraryId)
		if _, err := s.DB.ExecContext(ctx, `UPDATE libraries SET `+strings.Join(set, ", ")+` WHERE id = ?`, args...); err != nil {
			return nil, err
		}
	}
	l, err := s.getLibrary(ctx, req.LibraryId)
	if err != nil {
		return nil, err
	}
	return UpdateLibrary200JSONResponse(l), nil
}

func (s *server) DeleteLibrary(ctx context.Context, req DeleteLibraryRequestObject) (DeleteLibraryResponseObject, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM libraries WHERE id = ?`, req.LibraryId)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return DeleteLibrary404JSONResponse{notFound(ctx, "not_found.library")}, nil
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM search_rows WHERE library_id = ?`, req.LibraryId); err != nil {
		return nil, err
	}
	return DeleteLibrary204Response{}, nil
}

func (s *server) ScanLibrary(ctx context.Context, req ScanLibraryRequestObject) (ScanLibraryResponseObject, error) {
	queue := s.Library.QueueScan
	if req.Params.Deep != nil && *req.Params.Deep {
		queue = s.Library.QueueDeepScan
	}
	t, err := queue(ctx, req.LibraryId)
	if errors.Is(err, library.ErrNotFound) {
		return ScanLibrary404JSONResponse{notFound(ctx, "not_found.library")}, nil
	}
	if err != nil {
		return nil, err
	}
	return ScanLibrary202JSONResponse(toTask(ctx, t)), nil
}

// ─── Series and books ────────────────────────────────────────────────────────

// ListSeries is in browse.go; the column lists and scanners in query.go.

func (s *server) GetSeries(ctx context.Context, req GetSeriesRequestObject) (GetSeriesResponseObject, error) {
	x, ok, err := s.oneSeries(ctx, req.SeriesId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return GetSeries404JSONResponse{notFound(ctx, "not_found.series")}, nil
	}
	return GetSeries200JSONResponse(x), nil
}

func (s *server) ListSeriesBooks(ctx context.Context, req ListSeriesBooksRequestObject) (ListSeriesBooksResponseObject, error) {
	if ok, err := s.visible(ctx, "series", req.SeriesId); err != nil {
		return nil, err
	} else if !ok {
		return ListSeriesBooks404JSONResponse{notFound(ctx, "not_found.series")}, nil
	}
	out, err := s.listBooks(ctx, ` AND b.series_id = ?`, `ORDER BY b.number_sort, b.path`, req.SeriesId)
	if err != nil {
		return nil, err
	}
	return ListSeriesBooks200JSONResponse(out), nil
}

func (s *server) GetBook(ctx context.Context, req GetBookRequestObject) (GetBookResponseObject, error) {
	x, ok, err := s.oneBook(ctx, req.BookId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return GetBook404JSONResponse{notFound(ctx, "not_found.book")}, nil
	}
	return GetBook200JSONResponse(x), nil
}

func (s *server) ListBookPages(ctx context.Context, req ListBookPagesRequestObject) (ListBookPagesResponseObject, error) {
	var size int64
	var mtime, status string
	cond, args := libScope(ctx, "b.library_id")
	if err := s.DB.QueryRowContext(ctx, `SELECT size_bytes, mtime, media_status FROM books b WHERE b.id = ? AND b.deleted_at IS NULL AND `+cond,
		append([]any{req.BookId}, args...)...).Scan(&size, &mtime, &status); errors.Is(err, sql.ErrNoRows) {
		return ListBookPages404JSONResponse{notFound(ctx, "not_found.book")}, nil
	} else if err != nil {
		return nil, err
	}
	if status != "ready" { // pages of a failed or outdated analysis are stale
		return ListBookPages200JSONResponse{}, nil
	}
	v := library.Version(size, mtime)
	rows, err := s.DB.QueryContext(ctx, `SELECT number, width, height, media_type FROM pages WHERE book_id = ? ORDER BY number`, req.BookId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := ListBookPages200JSONResponse{}
	for rows.Next() {
		var p Page
		if err := rows.Scan(&p.Number, &p.Width, &p.Height, &p.MediaType); err != nil {
			return nil, err
		}
		p.Url = fmt.Sprintf("/api/v1/books/%s/pages/%d?v=%s", req.BookId, p.Number, v)
		p.ThumbnailUrl = fmt.Sprintf("/api/v1/books/%s/pages/%d/thumbnail?v=%s", req.BookId, p.Number, v)
		out = append(out, p)
	}
	return out, rows.Err()
}

// The image operations are served by servePage / serveThumbnail (see New);
// the strict handlers exist only to satisfy the generated interface.
func (s *server) GetBookPage(ctx context.Context, _ GetBookPageRequestObject) (GetBookPageResponseObject, error) {
	return GetBookPage404JSONResponse{notFound(ctx, "not_found.route")}, nil
}
func (s *server) GetBookPageThumbnail(ctx context.Context, _ GetBookPageThumbnailRequestObject) (GetBookPageThumbnailResponseObject, error) {
	return GetBookPageThumbnail404JSONResponse{notFound(ctx, "not_found.route")}, nil
}
func (s *server) GetBookThumbnail(ctx context.Context, _ GetBookThumbnailRequestObject) (GetBookThumbnailResponseObject, error) {
	return GetBookThumbnail404JSONResponse{notFound(ctx, "not_found.route")}, nil
}
func (s *server) GetSeriesThumbnail(ctx context.Context, _ GetSeriesThumbnailRequestObject) (GetSeriesThumbnailResponseObject, error) {
	return GetSeriesThumbnail404JSONResponse{notFound(ctx, "not_found.route")}, nil
}

// pageAllowed checks the page number, that the caller may see the book and
// may read it, and answers the request itself when not. A thumbnail of a
// page is the page, so both page handlers use it.
func (s *server) pageAllowed(w http.ResponseWriter, r *http.Request) (int, bool) {
	n, err := strconv.Atoi(r.PathValue("page"))
	if err != nil || n < 1 {
		writeError(r.Context(), w, http.StatusNotFound, "not_found.page")
		return 0, false
	}
	if ok, err := s.visible(r.Context(), "books", r.PathValue("bookId")); err != nil {
		writeError(r.Context(), w, http.StatusInternalServerError, "server.page_unreadable")
		return 0, false
	} else if !ok {
		writeError(r.Context(), w, http.StatusNotFound, "not_found.page")
		return 0, false
	}
	if !canStream(r.Context()) {
		writeError(r.Context(), w, http.StatusForbidden, "forbidden.read")
		return 0, false
	}
	return n, true
}

// servePageThumbnail is GET /books/{bookId}/pages/{page}/thumbnail.
func (s *server) servePageThumbnail(w http.ResponseWriter, r *http.Request) {
	n, ok := s.pageAllowed(w, r)
	if !ok {
		return
	}
	t, err := s.Library.PageThumbnail(r.Context(), r.PathValue("bookId"), n)
	if errors.Is(err, library.ErrNotFound) {
		writeError(r.Context(), w, http.StatusNotFound, "not_found.page")
		return
	}
	if err != nil {
		if r.Context().Err() == nil {
			slog.Warn("page thumbnail", "book", r.PathValue("bookId"), "page", n, "err", err)
		}
		writeError(r.Context(), w, http.StatusInternalServerError, "server.page_unreadable")
		return
	}
	f := t.File
	defer f.Close()
	h := w.Header()
	h.Set("ETag", t.ETag)
	if r.URL.Query().Get("v") == t.Version {
		h.Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "private, no-cache")
	}
	h.Set("Content-Type", "image/jpeg")
	h.Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", time.Time{}, f) // answers If-None-Match from the ETag
}

func (s *server) servePage(w http.ResponseWriter, r *http.Request) {
	n, ok := s.pageAllowed(w, r)
	if !ok {
		return
	}
	p, err := s.Library.OpenPage(r.Context(), r.PathValue("bookId"), n)
	if errors.Is(err, library.ErrNotFound) {
		writeError(r.Context(), w, http.StatusNotFound, "not_found.page")
		return
	}
	if err != nil {
		writeError(r.Context(), w, http.StatusInternalServerError, "server.page_unreadable")
		return
	}
	defer p.Close()
	h := w.Header()
	h.Set("ETag", p.ETag)
	// Cache forever only under a versioned URL (?v= from the page list):
	// a replaced file gets new URLs. Anything else must revalidate.
	if r.URL.Query().Get("v") == p.Version {
		h.Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "private, no-cache")
	}
	if inm := r.Header.Get("If-None-Match"); inm != "" && (inm == p.ETag || inm == "*") {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", p.MediaType)
	h.Set("Content-Length", strconv.FormatInt(p.Size, 10))
	h.Set("X-Content-Type-Options", "nosniff")
	if r.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(w, p.Body); err != nil {
		slog.Debug("page write", "err", err) // usually the client went away
	}
}

func (s *server) serveThumbnail(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("bookId")
		if kind == "series" {
			id = r.PathValue("seriesId")
		}
		table := "books"
		if kind == "series" {
			table = "series"
		}
		if ok, err := s.visible(r.Context(), table, id); err != nil {
			writeError(r.Context(), w, http.StatusInternalServerError, "server.cover_unreadable")
			return
		} else if !ok {
			writeError(r.Context(), w, http.StatusNotFound, "not_found.cover")
			return
		}
		s.sendThumbnail(w, r, kind, id)
	}
}

// sendThumbnail writes the cover of a book or series. The caller has
// already decided that the requester may see it.
func (s *server) sendThumbnail(w http.ResponseWriter, r *http.Request, kind, id string) {
	p, err := s.Library.ThumbnailPath(r.Context(), kind, id)
	if errors.Is(err, library.ErrNotFound) {
		writeError(r.Context(), w, http.StatusNotFound, "not_found.cover")
		return
	}
	if err != nil {
		writeError(r.Context(), w, http.StatusInternalServerError, "server.cover_unreadable")
		return
	}
	f, err := os.Open(p)
	if err != nil {
		writeError(r.Context(), w, http.StatusNotFound, "not_found.cover")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		writeError(r.Context(), w, http.StatusInternalServerError, "server.cover_unreadable")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "image/jpeg")
	h.Set("X-Content-Type-Options", "nosniff")
	// Thumbnails are regenerated in place, so revalidate instead of
	// caching forever; ServeContent answers 304 from Last-Modified.
	h.Set("Cache-Control", "private, no-cache")
	http.ServeContent(w, r, "", st.ModTime(), f)
}

// ─── Tasks and media issues ──────────────────────────────────────────────────

// toTask renders a task for the request's language: a coded label or error
// comes from the catalog, anything else is the stored text.
func toTask(ctx context.Context, t task.Task) Task {
	label, errText := t.Label, t.Error
	if t.LabelCode != "" {
		label = msg(ctx, t.LabelCode, t.LabelArg)
	}
	if t.ErrorCode != "" {
		var args []any
		if t.ErrorArg != "" {
			args = append(args, t.ErrorArg)
		}
		errText = msg(ctx, t.ErrorCode, args...)
	}
	out := Task{Id: t.ID, Kind: TaskKind(t.Kind), Label: label, State: TaskState(t.State), Progress: t.Progress, Error: errText}
	if t.LibraryID != "" {
		v := t.LibraryID
		out.LibraryId = &v
	}
	if t.StartedAt != nil {
		v := t.StartedAt.Format(time.RFC3339)
		out.StartedAt = &v
	}
	return out
}

func (s *server) ListTasks(ctx context.Context, _ ListTasksRequestObject) (ListTasksResponseObject, error) {
	ts, err := s.Tasks.List(ctx)
	if err != nil {
		return nil, err
	}
	out := ListTasks200JSONResponse{}
	for _, t := range ts {
		out = append(out, toTask(ctx, t))
	}
	return out, nil
}

func (s *server) CancelTask(ctx context.Context, req CancelTaskRequestObject) (CancelTaskResponseObject, error) {
	err := s.Tasks.Cancel(ctx, req.TaskId)
	if errors.Is(err, task.ErrNotFound) {
		return CancelTask404JSONResponse{notFound(ctx, "not_found.task")}, nil
	}
	if err != nil {
		return nil, err
	}
	return CancelTask204Response{}, nil
}

func (s *server) ListMediaIssues(ctx context.Context, _ ListMediaIssuesRequestObject) (ListMediaIssuesResponseObject, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, CASE WHEN deleted_at IS NOT NULL THEN 'missing' ELSE media_status END,
		  CASE WHEN deleted_at IS NOT NULL THEN path ELSE path || ': ' || media_error END
		FROM books WHERE deleted_at IS NOT NULL OR media_status IN ('error', 'unsupported')
		ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := ListMediaIssues200JSONResponse{}
	for rows.Next() {
		var m MediaIssue
		var kind string
		if err := rows.Scan(&m.BookId, &kind, &m.Detail); err != nil {
			return nil, err
		}
		m.Kind = MediaIssueKind(kind)
		out = append(out, m)
	}
	return out, rows.Err()
}
