package archive

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nwaples/rardecode/v2"
)

type rarFile struct {
	Entry
	encrypted bool
}

// rarArchive keeps the listing and the library root. Every Open and Walk
// opens the file again through the root, because a RAR stream can only be
// read forward.
type rarArchive struct {
	root   *os.Root // closed with the archive
	name   string   // file inside root, '/'-separated
	opts   []rardecode.Option
	files  []rarFile
	byName map[string]int // index into files of the first entry with a name
	solid  bool           // some file continues the previous one's stream
}

// maxRarDictionary caps the decoder window rardecode allocates up front
// (its default is 4 GiB, which a small hostile file can claim). WinRAR's
// default dictionary is 32 MiB; books made with larger ones fail to open.
const maxRarDictionary = 128 << 20

var rarOpts = []rardecode.Option{rardecode.MaxDictionarySize(maxRarDictionary)}

func openRar(root *os.Root, name string) (Archive, error) {
	st, err := root.Stat(filepath.FromSlash(name))
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %q is not a regular file", ErrUnsupported, name)
	}
	opts := append(rarOpts[:len(rarOpts):len(rarOpts)], rardecode.FileSystem(root.FS()))
	// List reads headers only, so listing does not decompress anything.
	files, err := rardecode.List(name, opts...)
	if err != nil {
		return nil, rarErr(err)
	}
	a := &rarArchive{root: root, name: name, opts: opts, byName: make(map[string]int)}
	for _, f := range files {
		h := &f.FileHeader
		a.solid = a.solid || h.Solid
		if h.IsDir {
			continue
		}
		if _, dup := a.byName[h.Name]; !dup {
			a.byName[h.Name] = len(a.files)
		}
		a.files = append(a.files, rarFile{Entry: rarEntry(h), encrypted: h.Encrypted})
	}
	return a, nil
}

func rarEntry(h *rardecode.FileHeader) Entry {
	size := h.UnPackedSize
	if h.UnKnownSize {
		size = -1
	}
	return Entry{Name: h.Name, Size: size}
}

// rarErr maps rardecode errors: password-protected archives become
// ErrUnsupported, and anything else that is not an OS error does too.
func rarErr(err error) error {
	if rarEncrypted(err) {
		return fmt.Errorf("%w: %w", ErrUnsupported, err)
	}
	return headerErr(err)
}

func rarEncrypted(err error) bool {
	return errors.Is(err, rardecode.ErrArchiveEncrypted) ||
		errors.Is(err, rardecode.ErrArchivedFileEncrypted) ||
		errors.Is(err, rardecode.ErrBadPassword)
}

func (a *rarArchive) Entries() []Entry {
	out := make([]Entry, len(a.files))
	for i, f := range a.files {
		out[i] = f.Entry
	}
	return out
}

// rarReadCloser reads one entry and closes the whole archive handle.
type rarReadCloser struct {
	io.Reader
	rc *rardecode.ReadCloser
}

func (r *rarReadCloser) Close() error { return r.rc.Close() }

func (a *rarArchive) Open(name string) (io.ReadCloser, error) {
	i, ok := a.byName[name]
	if !ok {
		return nil, notExist(name)
	}
	if a.files[i].encrypted {
		return nil, errEncrypted(name)
	}
	rr, err := rardecode.OpenReader(a.name, a.opts...)
	if err != nil {
		return nil, rarErr(err)
	}
	for {
		h, err := rr.Next()
		if errors.Is(err, io.EOF) {
			rr.Close()
			return nil, notExist(name)
		}
		if err != nil {
			rr.Close()
			return nil, rarErr(err)
		}
		if h.IsDir || h.Name != name {
			continue
		}
		return &rarReadCloser{Reader: rr, rc: rr}, nil
	}
}

func (a *rarArchive) Walk(fn func(e Entry, r io.Reader) error) error {
	return a.walkLimited(newBudget(Limit{}), fn)
}

func (a *rarArchive) walkLimited(b *budget, fn func(e Entry, r io.Reader) error) error {
	rr, err := rardecode.OpenReader(a.name, a.opts...)
	if err != nil {
		return rarErr(err)
	}
	defer rr.Close()

	for {
		h, err := rr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return rarErr(err)
		}
		if h.IsDir {
			continue
		}
		if h.Encrypted {
			return errEncrypted(h.Name)
		}
		err = fn(rarEntry(h), b.reader(rr))
		if errors.Is(err, SkipRest) {
			return nil
		}
		if err != nil {
			return err
		}
		// In a solid archive Next decodes whatever fn left unread; do it
		// here so it counts against the budget. Otherwise Next just seeks.
		if a.solid {
			if err := b.drain(rr); err != nil {
				return rarErr(err)
			}
		}
	}
}

func (a *rarArchive) RandomAccess() bool { return false }

// Close releases the root; the archive file itself is opened per call.
func (a *rarArchive) Close() error { return a.root.Close() }
