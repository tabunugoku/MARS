// Package archive reads comic books: CBZ (zip), CBR (RAR4/RAR5), CB7 (7z)
// and PDF, all without cgo. A PDF page that is one browser-servable image is
// served as that image; any other page is drawn to PNG by pdfium, which runs
// as WebAssembly on wazero.
//
// Names returned by Entries are the archive's own entry names. They are data
// from the file, not trusted paths: callers must never join them onto a
// filesystem path.
package archive

import (
	"context"
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

// ContextOpener is implemented by archives whose Open can give up when a
// context ends. Use OpenContext rather than asserting it directly.
type ContextOpener interface {
	OpenContext(ctx context.Context, name string) (io.ReadCloser, error)
}

// OpenContext is Open under ctx. Archives with a context-aware open (PDF) get
// ctx; the rest check ctx first and then call Open, which for them is cheap
// or at least not cancellable.
func OpenContext(ctx context.Context, a Archive, name string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if co, ok := a.(ContextOpener); ok {
		return co.OpenContext(ctx, name)
	}
	return a.Open(name)
}

// EntrySizer is implemented by archives that know an entry's pixel size
// without reading it (PDF pages drawn by pdfium). ok is false for entries it
// does not know; the caller then reads the entry as usual.
type EntrySizer interface {
	EntrySize(ctx context.Context, name string) (w, h int, ok bool, err error)
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
