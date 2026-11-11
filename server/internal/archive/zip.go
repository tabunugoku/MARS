package archive

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"strings"
)

type zipFile struct {
	Entry
	f *zip.File
}

type zipArchive struct {
	file   *os.File
	files  []zipFile            // regular files in archive order
	byName map[string]*zip.File // first entry for each name
}

func openZip(file *os.File, size int64) (Archive, error) {
	zr, err := zip.NewReader(file, size)
	if err != nil {
		file.Close()
		return nil, headerErr(err)
	}
	z := &zipArchive{file: file, byName: make(map[string]*zip.File)}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/") {
			continue
		}
		z.files = append(z.files, zipFile{
			Entry: Entry{Name: f.Name, Size: int64(f.UncompressedSize64)},
			f:     f,
		})
		if _, dup := z.byName[f.Name]; !dup {
			z.byName[f.Name] = f
		}
	}
	return z, nil
}

func zipEncrypted(f *zip.File) bool { return f.Flags&0x1 != 0 }

func (z *zipArchive) Entries() []Entry {
	out := make([]Entry, len(z.files))
	for i, zf := range z.files {
		out[i] = zf.Entry
	}
	return out
}

func (z *zipArchive) Open(name string) (io.ReadCloser, error) {
	f, ok := z.byName[name]
	if !ok {
		return nil, notExist(name)
	}
	if zipEncrypted(f) {
		return nil, errEncrypted(name)
	}
	return f.Open()
}

func (z *zipArchive) Walk(fn func(e Entry, r io.Reader) error) error {
	return z.walkLimited(newBudget(Limit{}), fn)
}

// Zip entries are independent, so skipping costs nothing and only what fn
// reads counts.
func (z *zipArchive) walkLimited(b *budget, fn func(e Entry, r io.Reader) error) error {
	for _, zf := range z.files {
		if zipEncrypted(zf.f) {
			return errEncrypted(zf.Name)
		}
		rc, err := zf.f.Open()
		if err != nil {
			return err
		}
		err = fn(zf.Entry, b.reader(rc))
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

func (z *zipArchive) RandomAccess() bool { return true }

func (z *zipArchive) Close() error { return z.file.Close() }
