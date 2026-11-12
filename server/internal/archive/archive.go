// Package archive reads comic books: CBZ (zip), CBR (RAR4/RAR5), CB7 (7z)
// and PDF (one image per page), all in pure Go.
//
// Names returned by Entries are the archive's own entry names. They are data
// from the file, not trusted paths: callers must never join them onto a
// filesystem path.
package archive

import (
	"errors"
	"io"
)

// Format is the book format stored in books.format.
type Format string

const (
	CBZ Format = "cbz"
	CBR Format = "cbr"
	CB7 Format = "cb7"
	PDF Format = "pdf"
)

// Entry is one regular file in an archive (directories are left out).
type Entry struct {
	Name string // entry name as stored, with '/' separators
	Size int64  // uncompressed size; -1 when the format does not record it
}

// Archive is an open book file.
type Archive interface {
	// Entries lists the regular files in archive order.
	Entries() []Entry
	// Open returns one entry by its exact name. For solid RAR and 7z this
	// may decompress everything before it; prefer Walk to read many.
	Open(name string) (io.ReadCloser, error)
	// Walk reads every entry once, in archive order. Returning SkipRest from
	// fn stops the walk without an error. r is only valid during the call.
	Walk(fn func(e Entry, r io.Reader) error) error
	// RandomAccess reports whether Open is cheap (zip, and non-solid
	// archives), so pages can be served straight from the file.
	RandomAccess() bool
	Close() error
}

// SkipRest stops a Walk early.
var SkipRest = errors.New("archive: skip rest")

// ErrNotExist is returned by Open for an unknown entry name.
var ErrNotExist = errors.New("archive: no such entry")

// ErrUnsupported is returned by Open for a file that is not a known archive
// (wrong extension or unreadable header), and for encrypted archives.
var ErrUnsupported = errors.New("archive: unsupported")

// FormatOf maps a file name to its format by extension (case-insensitive):
// .cbz/.zip -> CBZ, .cbr/.rar -> CBR, .cb7/.7z -> CB7. ok is false otherwise.
func FormatOf(name string) (f Format, ok bool) { return formatOf(name) }

// Open opens the book file at path, choosing the reader by extension.
func Open(path string) (Archive, Format, error) { return open(path) }
