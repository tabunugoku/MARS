package httpapi

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/tabunugoku/cominavi/server/internal/library"
)

// ─── Downloads ───────────────────────────────────────────────────────────────

// The downloads are served by serveBookFile / serveSeriesZip (see New), so
// they can stream; the strict handlers only satisfy the generated interface.
func (s *server) DownloadBook(context.Context, DownloadBookRequestObject) (DownloadBookResponseObject, error) {
	return DownloadBook404JSONResponse{notFound("not routed")}, nil
}
func (s *server) DownloadSeries(context.Context, DownloadSeriesRequestObject) (DownloadSeriesResponseObject, error) {
	return DownloadSeries404JSONResponse{notFound("not routed")}, nil
}

const noDownload = "このアカウントではファイルをダウンロードできません"

// serveBookFile sends a book's original file. Visibility is checked before
// the permission, as for pages, so a book outside the caller's libraries
// answers 404 whatever their permissions.
//
// With inline=true the file is shown in the browser instead of saved: only
// PDFs, under the streaming permission (as for reading a page) rather than
// the download one.
func (s *server) serveBookFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	inline := false
	if q := r.URL.Query().Get("inline"); q != "" {
		v, err := strconv.ParseBool(q)
		if err != nil {
			writeError(w, http.StatusBadRequest, "inline が正しくありません")
			return
		}
		inline = v
	}
	b, ok, err := s.oneBook(ctx, r.PathValue("bookId"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ファイルを読めませんでした")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "本が見つかりません")
		return
	}
	if inline {
		if !canStream(ctx) {
			writeError(w, http.StatusForbidden, "このアカウントでは本を読めません")
			return
		}
		if string(b.Format) != "pdf" {
			writeError(w, http.StatusBadRequest, "PDF だけをブラウザで開けます")
			return
		}
	} else if !canDownload(ctx) {
		writeError(w, http.StatusForbidden, noDownload)
		return
	}
	f, err := s.Library.OpenBookFile(ctx, b.Id)
	if errors.Is(err, library.ErrNotFound) {
		writeError(w, http.StatusNotFound, "ファイルが見つかりません")
		return
	}
	if err != nil {
		slog.Error("download", "book", b.Id, "err", err)
		writeError(w, http.StatusInternalServerError, "ファイルを読めませんでした")
		return
	}
	defer f.Close()
	h := w.Header()
	if inline {
		h.Set("Content-Disposition", inlineDisposition(path.Base(b.Path)))
		h.Set("Content-Type", "application/pdf")
	} else {
		h.Set("Content-Disposition", attachment(path.Base(b.Path)))
		h.Set("Content-Type", "application/octet-stream")
	}
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, no-cache")
	// ServeContent answers Range and HEAD, so a broken download resumes.
	http.ServeContent(w, r, "", f.Info.ModTime(), f)
}

// serveSeriesZip streams every live book of a series as one ZIP. Entries
// are stored, not recompressed (the books are already compressed), and
// written straight from the files: nothing is built in memory or on disk,
// so a series of many GB costs one copy buffer. archive/zip switches to
// ZIP64 by itself for entries or archives over 4 GB.
func (s *server) serveSeriesZip(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	x, ok, err := s.oneSeries(ctx, r.PathValue("seriesId"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ファイルを読めませんでした")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "シリーズが見つかりません")
		return
	}
	if !canDownload(ctx) {
		writeError(w, http.StatusForbidden, noDownload)
		return
	}
	books, err := s.listBooks(ctx, ` AND b.series_id = ?`, `ORDER BY b.number_sort, b.path`, x.Id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ファイルを読めませんでした")
		return
	}
	if len(books) == 0 {
		writeError(w, http.StatusNotFound, "シリーズに本がありません")
		return
	}
	// Check every file before the first byte: once the ZIP has started an
	// error can no longer be reported as a status.
	for _, b := range books {
		f, err := s.Library.OpenBookFile(ctx, b.Id)
		if err != nil {
			writeError(w, http.StatusNotFound, "ファイルが見つかりません: "+path.Base(b.Path))
			return
		}
		f.Close()
	}
	names := zipNames(books)

	h := w.Header()
	h.Set("Content-Disposition", attachment(x.Title+".zip"))
	h.Set("Content-Type", "application/zip")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, no-store")
	if r.Method == http.MethodHead {
		return
	}
	zw := zip.NewWriter(w)
	buf := make([]byte, 256<<10)
	for i, b := range books {
		if err := writeZipEntry(ctx, s.Library, zw, b.Id, names[i], buf); err != nil {
			// A file vanished or the client left mid-stream. Abort the
			// connection so the client sees a broken download rather than
			// a ZIP that looks complete but lacks books.
			slog.Warn("series zip", "series", x.Id, "book", b.Id, "err", err)
			panic(http.ErrAbortHandler)
		}
	}
	if err := zw.Close(); err != nil {
		slog.Debug("series zip close", "err", err)
		panic(http.ErrAbortHandler)
	}
}

func writeZipEntry(ctx context.Context, lib *library.Service, zw *zip.Writer, bookID, name string, buf []byte) error {
	f, err := lib.OpenBookFile(ctx, bookID)
	if err != nil {
		return err
	}
	defer f.Close()
	hdr := &zip.FileHeader{Name: name, Method: zip.Store, Modified: f.Info.ModTime()}
	hdr.SetMode(0o644)
	dst, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	n, err := io.CopyBuffer(dst, f, buf)
	if err == nil && n != f.Info.Size() {
		err = fmt.Errorf("file changed while sending: %d of %d bytes", n, f.Info.Size())
	}
	return err
}

// zipNames gives each book its file name, adding " (2)", " (3)"... before
// the extension when two books of the series share one.
func zipNames(books []Book) []string {
	out := make([]string, len(books))
	seen := map[string]bool{}
	for i, b := range books {
		base := path.Base(b.Path)
		name := base
		ext := path.Ext(base)
		for n := 2; seen[strings.ToLower(name)]; n++ {
			name = fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(base, ext), n, ext)
		}
		seen[strings.ToLower(name)] = true
		out[i] = name
	}
	return out
}

// attachment builds a Content-Disposition header for name: an ASCII
// fallback for old clients, and the real name as RFC 5987 UTF-8
// (filename*), which every current browser prefers.
func attachment(name string) string { return disposition("attachment", name) }

// inlineDisposition is attachment for a file shown in the browser.
func inlineDisposition(name string) string { return disposition("inline", name) }

func disposition(kind, name string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\':
			return '_'
		case r < 0x20 || r == 0x7f:
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		name = "download"
	}
	return kind + `; filename="` + asciiFallback(name) + `"; filename*=UTF-8''` + rfc5987(name)
}

func asciiFallback(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '"' || r == '\\' || r == '%':
			b.WriteByte('_')
		case r >= 0x20 && r < 0x7f:
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// rfc5987 percent-encodes every byte that is not an attr-char (RFC 5987
// section 3.2.1). url.PathEscape is not enough: it leaves ' (the ext-value
// delimiter) and a few other characters as they are.
func rfc5987(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.IndexByte("!#$&+-.^_`|~", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&15])
	}
	return b.String()
}

// ─── Thumbnails and covers ───────────────────────────────────────────────────

func thumbErr(err error) string {
	switch {
	case errors.Is(err, library.ErrNotReady):
		return "この本はまだ解析されていません"
	case errors.Is(err, library.ErrBadPage):
		return "そのページはありません"
	}
	return "表紙を作れませんでした"
}

func (s *server) RegenerateBookThumbnail(ctx context.Context, req RegenerateBookThumbnailRequestObject) (RegenerateBookThumbnailResponseObject, error) {
	err := s.Library.RegenerateThumbnail(ctx, req.BookId)
	switch {
	case errors.Is(err, library.ErrNotFound):
		return RegenerateBookThumbnail404JSONResponse{notFound("本が見つかりません")}, nil
	case err != nil:
		slog.Warn("thumbnail", "book", req.BookId, "err", err)
		return RegenerateBookThumbnail400JSONResponse{badRequest(thumbErr(err))}, nil
	}
	return RegenerateBookThumbnail204Response{}, nil
}

func (s *server) SetBookCover(ctx context.Context, req SetBookCoverRequestObject) (SetBookCoverResponseObject, error) {
	if req.Body == nil {
		return SetBookCover400JSONResponse{badRequest("本文がありません")}, nil
	}
	err := s.Library.SetCover(ctx, req.BookId, req.Body.Page)
	switch {
	case errors.Is(err, library.ErrNotFound):
		return SetBookCover404JSONResponse{notFound("本が見つかりません")}, nil
	case err != nil:
		slog.Warn("cover", "book", req.BookId, "err", err)
		return SetBookCover400JSONResponse{badRequest(thumbErr(err))}, nil
	}
	b, ok, err := s.oneBook(ctx, req.BookId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return SetBookCover404JSONResponse{notFound("本が見つかりません")}, nil
	}
	return SetBookCover200JSONResponse(b), nil
}

func (s *server) RegenerateSeriesThumbnails(ctx context.Context, req RegenerateSeriesThumbnailsRequestObject) (RegenerateSeriesThumbnailsResponseObject, error) {
	x, ok, err := s.oneSeries(ctx, req.SeriesId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return RegenerateSeriesThumbnails404JSONResponse{notFound("シリーズが見つかりません")}, nil
	}
	t, err := s.Library.QueueThumbnails(ctx, x.LibraryId, x.Id)
	if errors.Is(err, library.ErrNotFound) {
		return RegenerateSeriesThumbnails404JSONResponse{notFound("シリーズが見つかりません")}, nil
	}
	if err != nil {
		return nil, err
	}
	return RegenerateSeriesThumbnails202JSONResponse(toTask(t)), nil
}

func (s *server) RegenerateLibraryThumbnails(ctx context.Context, req RegenerateLibraryThumbnailsRequestObject) (RegenerateLibraryThumbnailsResponseObject, error) {
	t, err := s.Library.QueueThumbnails(ctx, req.LibraryId, "")
	if errors.Is(err, library.ErrNotFound) {
		return RegenerateLibraryThumbnails404JSONResponse{notFound("ライブラリが見つかりません")}, nil
	}
	if err != nil {
		return nil, err
	}
	return RegenerateLibraryThumbnails202JSONResponse(toTask(t)), nil
}
