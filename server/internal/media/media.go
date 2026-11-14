// Package media turns a book file into what the reader needs: the ordered
// page list with sizes, the ComicInfo.xml bytes, the cover thumbnail, and an
// on-disk cache of extracted pages for archives without cheap random access.
package media

import (
	"errors"
	"image"
	"io"
	"time"

	"github.com/tabunugoku/mars/server/internal/archive"
)

// Limits that keep a hostile or broken file from exhausting the server.
const (
	MaxPixels        = 200_000_000 // width*height from the image header, checked before any decode; print masters reach about 115 MP
	MaxEntryBytes    = 64 << 20    // one page or the ComicInfo.xml read
	MaxComicInfo     = 1 << 20     // ComicInfo.xml is truncated past this and treated as absent
	MaxExtractedBook = 2 << 30     // total bytes a page cache extraction may write for one book
	// MaxDecompressed caps the bytes one pass over a book may decompress,
	// counting entries that are skipped (solid RAR and 7z still decode them).
	MaxDecompressed = 4 << 30
	AnalyzeTimeout  = 10 * time.Minute // one book's analysis
	ExtractTimeout  = 5 * time.Minute  // one book's page cache extraction
)

// Page is one image of a book. Number is 1-based in natural name order.
type Page struct {
	Number    int
	Entry     string // archive entry name (data, never a filesystem path)
	MediaType string // image/jpeg, image/png, image/gif, image/webp, image/avif
	Width     int
	Height    int
}

// Analysis is the result of reading a book once.
type Analysis struct {
	Format    archive.Format
	Pages     []Page
	ComicInfo []byte // nil when the archive has no ComicInfo.xml
	Cover     []byte // raw bytes of the cover page, for the thumbnail
	CoverPage int    // which page Cover is (1-based)
}

var (
	ErrNoPages       = errors.New("media: no images in archive")
	ErrTooLarge      = errors.New("media: image or entry over the size limit")
	ErrCacheTooBig   = errors.New("media: book exceeds the extraction limit")
	ErrTooManyPixels = errors.New("media: image has more pixels than the limit")
)

// CheckPixels reads an image's header from r and returns its config. When
// width*height is over limit it returns ErrTooManyPixels, before any pixel is
// decoded. A header that does not decode returns the image package's error.
func CheckPixels(r io.Reader, limit int64) (image.Config, error) {
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return image.Config{}, err
	}
	if int64(cfg.Width)*int64(cfg.Height) > limit {
		return cfg, ErrTooManyPixels
	}
	return cfg, nil
}
