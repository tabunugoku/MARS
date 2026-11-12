package httpapi

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/jpeg" // decoder registration for DecodeConfig
	_ "image/png"  // decoder registration for DecodeConfig
	"io"
	"net/http"

	"github.com/tabunugoku/cominavi/server/internal/library"
	_ "golang.org/x/image/webp" // decoder registration for DecodeConfig
)

// Uploaded covers: at most maxCoverBytes, and no side longer than
// maxCoverSide pixels. The format is taken from the bytes, not the name.
const (
	maxCoverBytes = 20 << 20
	maxCoverSide  = 20000
)

// readCover reads an uploaded image and checks that it is a JPEG, PNG or WebP
// no bigger than the limits. It returns the bytes, or the code of the 400
// answer.
func readCover(body io.Reader) ([]byte, string) {
	if body == nil {
		return nil, "bad_request.image_format"
	}
	data, err := io.ReadAll(io.LimitReader(body, maxCoverBytes+1))
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		return nil, "bad_request.image_too_large"
	case err != nil:
		return nil, "bad_request.image_format"
	case len(data) > maxCoverBytes:
		return nil, "bad_request.image_too_large"
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "bad_request.image_format"
	}
	switch format {
	case "jpeg", "png", "webp":
	default:
		return nil, "bad_request.image_format"
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxCoverSide || cfg.Height > maxCoverSide {
		return nil, "bad_request.image_size"
	}
	return data, ""
}

// UploadBookThumbnail replaces a book's thumbnail with an uploaded image.
func (s *server) UploadBookThumbnail(ctx context.Context, req UploadBookThumbnailRequestObject) (UploadBookThumbnailResponseObject, error) {
	ok, err := s.visible(ctx, "books", req.BookId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return UploadBookThumbnail404JSONResponse{notFound(ctx, "not_found.book")}, nil
	}
	img, code := readCover(req.Body)
	if code != "" {
		return UploadBookThumbnail400JSONResponse{badRequest(ctx, code)}, nil
	}
	err = s.Library.SetBookUpload(ctx, req.BookId, img)
	switch {
	case errors.Is(err, library.ErrNotFound):
		return UploadBookThumbnail404JSONResponse{notFound(ctx, "not_found.book")}, nil
	case errors.Is(err, library.ErrImage):
		return UploadBookThumbnail400JSONResponse{badRequest(ctx, "bad_request.image_format")}, nil
	case err != nil:
		return nil, err
	}
	b, ok, err := s.oneBook(ctx, req.BookId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return UploadBookThumbnail404JSONResponse{notFound(ctx, "not_found.book")}, nil
	}
	return UploadBookThumbnail200JSONResponse(b), nil
}

// UploadSeriesThumbnail replaces a series' cover with an uploaded image.
func (s *server) UploadSeriesThumbnail(ctx context.Context, req UploadSeriesThumbnailRequestObject) (UploadSeriesThumbnailResponseObject, error) {
	ok, err := s.visible(ctx, "series", req.SeriesId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return UploadSeriesThumbnail404JSONResponse{notFound(ctx, "not_found.series")}, nil
	}
	img, code := readCover(req.Body)
	if code != "" {
		return UploadSeriesThumbnail400JSONResponse{badRequest(ctx, code)}, nil
	}
	err = s.Library.SetSeriesUpload(ctx, req.SeriesId, img)
	switch {
	case errors.Is(err, library.ErrNotFound):
		return UploadSeriesThumbnail404JSONResponse{notFound(ctx, "not_found.series")}, nil
	case errors.Is(err, library.ErrImage):
		return UploadSeriesThumbnail400JSONResponse{badRequest(ctx, "bad_request.image_format")}, nil
	case err != nil:
		return nil, err
	}
	x, ok, err := s.oneSeries(ctx, req.SeriesId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return UploadSeriesThumbnail404JSONResponse{notFound(ctx, "not_found.series")}, nil
	}
	return UploadSeriesThumbnail200JSONResponse(x), nil
}

// DeleteSeriesThumbnail drops an uploaded series cover.
func (s *server) DeleteSeriesThumbnail(ctx context.Context, req DeleteSeriesThumbnailRequestObject) (DeleteSeriesThumbnailResponseObject, error) {
	ok, err := s.visible(ctx, "series", req.SeriesId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteSeriesThumbnail404JSONResponse{notFound(ctx, "not_found.series")}, nil
	}
	err = s.Library.DropSeriesUpload(ctx, req.SeriesId)
	switch {
	case errors.Is(err, library.ErrNotFound):
		return DeleteSeriesThumbnail404JSONResponse{notFound(ctx, "not_found.series")}, nil
	case err != nil:
		return nil, err
	}
	x, ok, err := s.oneSeries(ctx, req.SeriesId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteSeriesThumbnail404JSONResponse{notFound(ctx, "not_found.series")}, nil
	}
	return DeleteSeriesThumbnail200JSONResponse(x), nil
}
