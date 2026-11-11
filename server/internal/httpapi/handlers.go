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

func notFound(msg string) NotFoundJSONResponse     { return NotFoundJSONResponse{Message: msg} }
func badRequest(msg string) BadRequestJSONResponse { return BadRequestJSONResponse{Message: msg} }

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
	return redactLibrary(ctx, l), err
}

// redactLibrary hides where the files live from users who cannot manage them.
func redactLibrary(ctx context.Context, l Library) Library {
	if !currentUser(ctx).IsAdmin() {
		l.Root, l.Exclude = "", []string{}
	}
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
		out = append(out, redactLibrary(ctx, l))
	}
	return out, rows.Err()
}

func (s *server) GetLibrary(ctx context.Context, req GetLibraryRequestObject) (GetLibraryResponseObject, error) {
	l, err := s.getLibrary(ctx, req.LibraryId)
	if errors.Is(err, sql.ErrNoRows) {
		return GetLibrary404JSONResponse{notFound("ライブラリが見つかりません")}, nil
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
	case Off, Hourly, Daily, Weekly:
		return true
	}
	return false
}

func validExclude(pats []string) error {
	for _, p := range pats {
		if _, err := path.Match(p, ""); err != nil {
			return fmt.Errorf("除外パターンが正しくありません: %q", p)
		}
	}
	return nil
}

func (s *server) CreateLibrary(ctx context.Context, req CreateLibraryRequestObject) (CreateLibraryResponseObject, error) {
	b := req.Body
	if b == nil {
		return CreateLibrary400JSONResponse{badRequest("本文がありません")}, nil
	}
	name := strings.TrimSpace(b.Name)
	if name == "" {
		return CreateLibrary400JSONResponse{badRequest("名前を入力してください")}, nil
	}
	if !validKind(b.Kind) {
		return CreateLibrary400JSONResponse{badRequest("種類が正しくありません")}, nil
	}
	interval := Daily
	if b.ScanInterval != nil {
		if !validInterval(*b.ScanInterval) {
			return CreateLibrary400JSONResponse{badRequest("スキャン間隔が正しくありません")}, nil
		}
		interval = *b.ScanInterval
	}
	exclude := []string{}
	if b.Exclude != nil {
		exclude = *b.Exclude
	}
	if err := validExclude(exclude); err != nil {
		return CreateLibrary400JSONResponse{badRequest(err.Error())}, nil
	}
	if !filepath.IsAbs(b.Root) {
		return CreateLibrary400JSONResponse{badRequest("フォルダは絶対パスで指定してください")}, nil
	}
	root := filepath.Clean(b.Root)
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return CreateLibrary400JSONResponse{badRequest("フォルダが見つかりません")}, nil
	}
	importCI := true
	if b.ImportComicInfo != nil {
		importCI = *b.ImportComicInfo
	}
	id := ids.New()
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO libraries (id, name, root, kind, scan_interval, import_comicinfo, exclude, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, root, string(b.Kind), string(interval), importCI, jsonText(exclude), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return CreateLibrary400JSONResponse{badRequest("このフォルダは別のライブラリに登録済みです")}, nil
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
		return UpdateLibrary404JSONResponse{notFound("ライブラリが見つかりません")}, nil
	} else if err != nil {
		return nil, err
	}
	b := req.Body
	if b == nil {
		return UpdateLibrary400JSONResponse{badRequest("本文がありません")}, nil
	}
	set, args := []string{}, []any{}
	if b.Name != nil {
		n := strings.TrimSpace(*b.Name)
		if n == "" {
			return UpdateLibrary400JSONResponse{badRequest("名前を入力してください")}, nil
		}
		set, args = append(set, "name = ?"), append(args, n)
	}
	if b.Kind != nil {
		if !validKind(*b.Kind) {
			return UpdateLibrary400JSONResponse{badRequest("種類が正しくありません")}, nil
		}
		set, args = append(set, "kind = ?"), append(args, string(*b.Kind))
	}
	if b.ScanInterval != nil {
		if !validInterval(*b.ScanInterval) {
			return UpdateLibrary400JSONResponse{badRequest("スキャン間隔が正しくありません")}, nil
		}
		set, args = append(set, "scan_interval = ?"), append(args, string(*b.ScanInterval))
	}
	if b.ImportComicInfo != nil {
		set, args = append(set, "import_comicinfo = ?"), append(args, *b.ImportComicInfo)
	}
	if b.Exclude != nil {
		if err := validExclude(*b.Exclude); err != nil {
			return UpdateLibrary400JSONResponse{badRequest(err.Error())}, nil
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
		return DeleteLibrary404JSONResponse{notFound("ライブラリが見つかりません")}, nil
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM search_index WHERE library_id = ?`, req.LibraryId); err != nil {
		return nil, err
	}
	return DeleteLibrary204Response{}, nil
}

func (s *server) ScanLibrary(ctx context.Context, req ScanLibraryRequestObject) (ScanLibraryResponseObject, error) {
	t, err := s.Library.QueueScan(ctx, req.LibraryId)
	if errors.Is(err, library.ErrNotFound) {
		return ScanLibrary404JSONResponse{notFound("ライブラリが見つかりません")}, nil
	}
	if err != nil {
		return nil, err
	}
	return ScanLibrary202JSONResponse(toTask(t)), nil
}

// ─── Series and books ────────────────────────────────────────────────────────

const seriesCols = `s.id, s.library_id, s.title, s.title_sort, s.alternate_titles, s.status, s.summary, s.publisher,
	s.reading_direction, s.age_rating, s.language, s.genres, s.tags, s.authors, s.total_book_count, s.locked_fields,
	s.created_at, s.updated_at,
	(SELECT count(*) FROM books b WHERE b.series_id = s.id AND b.deleted_at IS NULL)`

func scanSeries(r scanner) (Series, error) {
	var x Series
	var alt, status, dir, genres, tags, au, locked string
	var age, total sql.NullInt64
	err := r.Scan(&x.Id, &x.LibraryId, &x.Title, &x.TitleSort, &alt, &status, &x.Summary, &x.Publisher,
		&dir, &age, &x.Language, &genres, &tags, &au, &total, &locked, &x.CreatedAt, &x.UpdatedAt, &x.BookCount)
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

func (s *server) ListSeries(ctx context.Context, req ListSeriesRequestObject) (ListSeriesResponseObject, error) {
	cond, args := libScope(ctx, "s.library_id")
	q := `SELECT ` + seriesCols + ` FROM series s WHERE s.deleted_at IS NULL AND ` + cond
	if req.Params.LibraryId != nil {
		q += ` AND s.library_id = ?`
		args = append(args, *req.Params.LibraryId)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY s.title_sort, s.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := ListSeries200JSONResponse{}
	for rows.Next() {
		x, err := scanSeries(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *server) GetSeries(ctx context.Context, req GetSeriesRequestObject) (GetSeriesResponseObject, error) {
	cond, args := libScope(ctx, "s.library_id")
	x, err := scanSeries(s.DB.QueryRowContext(ctx, `SELECT `+seriesCols+` FROM series s WHERE s.id = ? AND s.deleted_at IS NULL AND `+cond, append([]any{req.SeriesId}, args...)...))
	if errors.Is(err, sql.ErrNoRows) {
		return GetSeries404JSONResponse{notFound("シリーズが見つかりません")}, nil
	}
	if err != nil {
		return nil, err
	}
	return GetSeries200JSONResponse(x), nil
}

const bookCols = `b.id, b.series_id, b.library_id, b.title, b.number, b.number_sort, b.summary, b.release_date, b.isbn,
	b.authors, b.tags, b.format, b.path, b.size_bytes, b.page_count, b.media_status, b.created_at`

func scanBook(r scanner) (Book, error) {
	var x Book
	var release sql.NullString
	var au, tags, format, status string
	err := r.Scan(&x.Id, &x.SeriesId, &x.LibraryId, &x.Title, &x.Number, &x.NumberSort, &x.Summary, &release, &x.Isbn,
		&au, &tags, &format, &x.Path, &x.SizeBytes, &x.PageCount, &status, &x.CreatedAt)
	x.Authors, x.Tags, x.Format, x.MediaStatus = authors(au), strs(tags), BookFormat(format), MediaStatus(status)
	if release.Valid {
		x.ReleaseDate = &release.String
	}
	x.ThumbnailUrl = "/api/v1/books/" + x.Id + "/thumbnail"
	return x, err
}

func (s *server) ListSeriesBooks(ctx context.Context, req ListSeriesBooksRequestObject) (ListSeriesBooksResponseObject, error) {
	if ok, err := s.visible(ctx, "series", req.SeriesId); err != nil {
		return nil, err
	} else if !ok {
		return ListSeriesBooks404JSONResponse{notFound("シリーズが見つかりません")}, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+bookCols+` FROM books b WHERE b.series_id = ? AND b.deleted_at IS NULL ORDER BY b.number_sort, b.path`, req.SeriesId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := ListSeriesBooks200JSONResponse{}
	for rows.Next() {
		x, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *server) GetBook(ctx context.Context, req GetBookRequestObject) (GetBookResponseObject, error) {
	cond, args := libScope(ctx, "b.library_id")
	x, err := scanBook(s.DB.QueryRowContext(ctx, `SELECT `+bookCols+` FROM books b WHERE b.id = ? AND b.deleted_at IS NULL AND `+cond, append([]any{req.BookId}, args...)...))
	if errors.Is(err, sql.ErrNoRows) {
		return GetBook404JSONResponse{notFound("本が見つかりません")}, nil
	}
	if err != nil {
		return nil, err
	}
	return GetBook200JSONResponse(x), nil
}

func (s *server) ListBookPages(ctx context.Context, req ListBookPagesRequestObject) (ListBookPagesResponseObject, error) {
	var size int64
	var mtime string
	cond, args := libScope(ctx, "b.library_id")
	if err := s.DB.QueryRowContext(ctx, `SELECT size_bytes, mtime FROM books b WHERE b.id = ? AND b.deleted_at IS NULL AND `+cond,
		append([]any{req.BookId}, args...)...).Scan(&size, &mtime); errors.Is(err, sql.ErrNoRows) {
		return ListBookPages404JSONResponse{notFound("本が見つかりません")}, nil
	} else if err != nil {
		return nil, err
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
		out = append(out, p)
	}
	return out, rows.Err()
}

// The image operations are served by servePage / serveThumbnail (see New);
// the strict handlers exist only to satisfy the generated interface.
func (s *server) GetBookPage(context.Context, GetBookPageRequestObject) (GetBookPageResponseObject, error) {
	return GetBookPage404JSONResponse{notFound("not routed")}, nil
}
func (s *server) GetBookThumbnail(context.Context, GetBookThumbnailRequestObject) (GetBookThumbnailResponseObject, error) {
	return GetBookThumbnail404JSONResponse{notFound("not routed")}, nil
}
func (s *server) GetSeriesThumbnail(context.Context, GetSeriesThumbnailRequestObject) (GetSeriesThumbnailResponseObject, error) {
	return GetSeriesThumbnail404JSONResponse{notFound("not routed")}, nil
}

func (s *server) servePage(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("page"))
	if err != nil || n < 1 {
		writeError(w, http.StatusNotFound, "ページが見つかりません")
		return
	}
	if ok, err := s.visible(r.Context(), "books", r.PathValue("bookId")); err != nil {
		writeError(w, http.StatusInternalServerError, "ページを読めませんでした")
		return
	} else if !ok {
		writeError(w, http.StatusNotFound, "ページが見つかりません")
		return
	}
	if !canStream(r.Context()) {
		writeError(w, http.StatusForbidden, "このアカウントでは本を読めません")
		return
	}
	p, err := s.Library.OpenPage(r.Context(), r.PathValue("bookId"), n)
	if errors.Is(err, library.ErrNotFound) {
		writeError(w, http.StatusNotFound, "ページが見つかりません")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ページを読めませんでした")
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
			writeError(w, http.StatusInternalServerError, "表紙を読めませんでした")
			return
		} else if !ok {
			writeError(w, http.StatusNotFound, "表紙がありません")
			return
		}
		p, err := s.Library.ThumbnailPath(r.Context(), kind, id)
		if errors.Is(err, library.ErrNotFound) {
			writeError(w, http.StatusNotFound, "表紙がありません")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "表紙を読めませんでした")
			return
		}
		f, err := os.Open(p)
		if err != nil {
			writeError(w, http.StatusNotFound, "表紙がありません")
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "表紙を読めませんでした")
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
}

// ─── Tasks and media issues ──────────────────────────────────────────────────

func toTask(t task.Task) Task {
	out := Task{Id: t.ID, Kind: TaskKind(t.Kind), Label: t.Label, State: TaskState(t.State), Progress: t.Progress, Error: t.Error}
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
		out = append(out, toTask(t))
	}
	return out, nil
}

func (s *server) CancelTask(ctx context.Context, req CancelTaskRequestObject) (CancelTaskResponseObject, error) {
	err := s.Tasks.Cancel(ctx, req.TaskId)
	if errors.Is(err, task.ErrNotFound) {
		return CancelTask404JSONResponse{notFound("タスクが見つかりません")}, nil
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
