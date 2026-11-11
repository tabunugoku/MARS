// Package media turns a book file into what the reader needs: the ordered
// page list with sizes, the ComicInfo.xml bytes, the cover thumbnail, and an
// on-disk cache of extracted pages for archives without cheap random access.
package media

import (
	"errors"
	"time"

	"github.com/tabunugoku/cominavi/server/internal/archive"
)

// Limits that keep a hostile or broken file from exhausting the server.
const (
	MaxPixels        = 100_000_000 // width*height from the image header, checked before any decode
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
	MediaType string // image/jpeg, image/png, image/gif, image/webp
	Width     int
	Height    int
}

// Analysis is the result of reading a book once.
type Analysis struct {
	Format    archive.Format
	Pages     []Page
	ComicInfo []byte // nil when the archive has no ComicInfo.xml
	Cover     []byte // raw bytes of page 1, for the thumbnail
}

var (
	ErrNoPages     = errors.New("media: no images in archive")
	ErrTooLarge    = errors.New("media: image or entry over the size limit")
	ErrCacheTooBig = errors.New("media: book exceeds the extraction limit")
)
