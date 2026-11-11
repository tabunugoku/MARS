package archive

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

func formatOf(name string) (Format, bool) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".cbz", ".zip":
		return CBZ, true
	case ".cbr", ".rar":
		return CBR, true
	case ".cb7", ".7z":
		return CB7, true
	}
	return "", false
}

func open(path string) (Archive, Format, error) { return OpenPath(PathOf(path)) }

// headerErr marks a parse failure as ErrUnsupported and keeps the reader's
// own error in the chain. OS errors (missing file, permissions) pass through
// unchanged so callers can tell them apart.
func headerErr(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrUnsupported, err)
}

func errEncrypted(name string) error {
	return fmt.Errorf("%w: entry %q is encrypted", ErrUnsupported, name)
}

func notExist(name string) error {
	return fmt.Errorf("%w: %q", ErrNotExist, name)
}
