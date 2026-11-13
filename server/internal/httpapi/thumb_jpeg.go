package httpapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/tabunugoku/mars/server/internal/media"
)

// isJPEGName reports whether a stored thumbnail file is a JPEG by its name.
func isJPEGName(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".jpg", ".jpeg":
		return true
	}
	return false
}

// jpegThumbnail returns the path of a JPEG copy of the stored thumbnail src
// (a WebP or AVIF file), making the copy on first use. The copy is named after
// the stored file, which is unique per image, so it never goes stale. It is
// written to a temporary file and renamed into place.
func (s *server) jpegThumbnail(ctx context.Context, src string) (string, error) {
	if s.JPEGCacheDir == "" {
		return "", errors.New("httpapi: no jpeg cache dir")
	}
	base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	dst := filepath.Join(s.JPEGCacheDir, base+".jpg")
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	jpg, err := media.ToJPEG(data)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.JPEGCacheDir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(s.JPEGCacheDir, base+".*.tmp")
	if err != nil {
		return "", err
	}
	_, err = tmp.Write(jpg)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), dst)
	}
	if err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return dst, nil
}
