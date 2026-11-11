package archive

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/bodgit/sevenzip"
)

type sevenFile struct {
	Entry
	f *sevenzip.File
}

type sevenArchive struct {
	file   *os.File
	files  []sevenFile
	byName map[string]*sevenzip.File
}

func openSevenZip(file *os.File, size int64) (Archive, error) {
	sr, err := sevenzip.NewReader(file, size)
	if err != nil {
		file.Close()
		return nil, headerErr(err)
	}
	a := &sevenArchive{file: file, byName: make(map[string]*sevenzip.File)}
	for _, f := range sr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		a.files = append(a.files, sevenFile{
			Entry: Entry{Name: f.Name, Size: int64(f.UncompressedSize)},
			f:     f,
		})
		if _, dup := a.byName[f.Name]; !dup {
			a.byName[f.Name] = f
		}
	}
	return a, nil
}

// sevenErr maps read errors that sevenzip reports as encrypted data.
func sevenErr(err error) error {
	var re *sevenzip.ReadError
	if errors.As(err, &re) && re.Encrypted {
		return fmt.Errorf("%w: %w", ErrUnsupported, err)
	}
	return err
}

// sevenReader passes data through and maps read errors with sevenErr.
type sevenReader struct {
	r io.ReadCloser
}

func (s sevenReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		err = sevenErr(err)
	}
	return n, err
}

func (s sevenReader) Close() error { return s.r.Close() }

func (a *sevenArchive) Entries() []Entry {
	out := make([]Entry, len(a.files))
	for i, f := range a.files {
		out[i] = f.Entry
	}
	return out
}

func (a *sevenArchive) Open(name string) (io.ReadCloser, error) {
	f, ok := a.byName[name]
	if !ok {
		return nil, notExist(name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, sevenErr(err)
	}
	return sevenReader{rc}, nil
}

func (a *sevenArchive) Walk(fn func(e Entry, r io.Reader) error) error {
	return a.walkLimited(newBudget(Limit{}), fn)
}

func (a *sevenArchive) walkLimited(b *budget, fn func(e Entry, r io.Reader) error) error {
	for _, sf := range a.files {
		rc, err := sf.f.Open()
		if err != nil {
			return sevenErr(err)
		}
		r := b.reader(sevenReader{rc})
		err = fn(sf.Entry, r)
		if err == nil {
			// The next entry of a solid folder decodes past whatever is
			// left of this one; do it here, within the budget.
			err = b.drain(sevenReader{rc})
		}
		cerr := rc.Close()
		switch {
		case errors.Is(err, SkipRest):
			return nil
		case err != nil:
			return err
		case cerr != nil:
			return cerr
		}
	}
	return nil
}

// RandomAccess is false: solid folders may decode everything before an entry.
func (a *sevenArchive) RandomAccess() bool { return false }

func (a *sevenArchive) Close() error { return a.file.Close() }
