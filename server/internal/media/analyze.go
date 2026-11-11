package media

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"  // decoder registration for DecodeConfig
	_ "image/jpeg" // decoder registration for DecodeConfig
	_ "image/png"  // decoder registration for DecodeConfig
	"io"
	"path"
	"sort"
	"strings"

	_ "golang.org/x/image/webp" // decoder registration for DecodeConfig

	"github.com/tabunugoku/cominavi/server/internal/archive"
	"github.com/tabunugoku/cominavi/server/internal/natsort"
)

// imageTypes maps a lower-case extension to the media type served for it.
var imageTypes = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
}

// isIgnored reports whether an entry is never a page or ComicInfo: macOS
// resource forks (__MACOSX/) and dot-files such as ._p1.jpg or .hidden.png.
func isIgnored(name string) bool {
	for _, seg := range strings.Split(name, "/") {
		if seg == "__MACOSX" {
			return true
		}
	}
	return strings.HasPrefix(path.Base(name), ".")
}

// pageMediaType returns the media type for an entry that counts as a page.
func pageMediaType(name string) (string, bool) {
	if isIgnored(name) {
		return "", false
	}
	mt, ok := imageTypes[strings.ToLower(path.Ext(name))]
	return mt, ok
}

func isComicInfo(name string) bool {
	return !isIgnored(name) && strings.EqualFold(path.Base(name), "comicinfo.xml")
}

// Analyze reads the book at path once and returns its pages, ComicInfo.xml
// and cover. Errors from archive.Open pass through unchanged, so callers can
// check errors.Is(err, archive.ErrUnsupported).
func Analyze(path string) (Analysis, error) {
	return AnalyzeContext(context.Background(), archive.PathOf(path))
}

// AnalyzeContext is Analyze that stops when ctx ends, after AnalyzeTimeout,
// or once the book has decompressed MaxDecompressed bytes.
func AnalyzeContext(ctx context.Context, path archive.Path) (Analysis, error) {
	ctx, cancel := context.WithTimeout(ctx, AnalyzeTimeout)
	defer cancel()
	a, format, err := archive.OpenPath(path)
	if err != nil {
		return Analysis{}, err
	}
	defer a.Close()

	// Choose pages and ComicInfo from the entry list alone; no data is read yet.
	mediaTypes := make(map[string]string)
	var names []string
	ciName, ciRoot := "", false
	for _, e := range a.Entries() {
		if isComicInfo(e.Name) {
			root := !strings.Contains(e.Name, "/")
			if ciName == "" || (root && !ciRoot) {
				ciName, ciRoot = e.Name, root
			}
			continue
		}
		mt, ok := pageMediaType(e.Name)
		if !ok {
			continue
		}
		if _, dup := mediaTypes[e.Name]; dup {
			continue // a repeated name is one page; the first in archive order is used
		}
		mediaTypes[e.Name] = mt
		names = append(names, e.Name)
	}
	if len(names) == 0 {
		return Analysis{}, ErrNoPages
	}
	sort.Slice(names, func(i, j int) bool { return natsort.Less(names[i], names[j]) })

	index := make(map[string]int, len(names))
	pages := make([]Page, len(names))
	for i, name := range names {
		index[name] = i
		pages[i] = Page{Number: i + 1, Entry: name, MediaType: mediaTypes[name]}
	}

	var (
		cover    []byte
		ciBytes  []byte
		ciSeen   bool
		pageSeen = make([]bool, len(names))
	)
	err = archive.WalkLimited(a, archive.Limit{Ctx: ctx, MaxBytes: MaxDecompressed}, func(e archive.Entry, r io.Reader) error {
		if ciName != "" && e.Name == ciName {
			if ciSeen {
				return nil
			}
			ciSeen = true
			data, err := io.ReadAll(io.LimitReader(r, MaxComicInfo+1))
			if err != nil {
				return fmt.Errorf("comicinfo.xml: %w", err)
			}
			if len(data) <= MaxComicInfo {
				ciBytes = data
			}
			return nil
		}
		i, ok := index[e.Name]
		if !ok || pageSeen[i] {
			return nil
		}
		pageSeen[i] = true
		var coverBuf *bytes.Buffer
		if i == 0 {
			coverBuf = &bytes.Buffer{}
		}
		w, h, err := readPageConfig(e, r, coverBuf)
		if err != nil {
			return fmt.Errorf("page %d: %w", i+1, err)
		}
		pages[i].Width, pages[i].Height = w, h
		if coverBuf != nil {
			cover = coverBuf.Bytes()
		}
		return nil
	})
	if err != nil {
		return Analysis{}, err
	}
	for i, seen := range pageSeen {
		if !seen {
			return Analysis{}, fmt.Errorf("media: page %d not found while reading archive", i+1)
		}
	}
	return Analysis{Format: format, Pages: pages, ComicInfo: ciBytes, Cover: cover}, nil
}

// readPageConfig reads the header of one page entry and checks its limits.
// The rest of the entry is drained so the size limit is enforced for the
// whole entry. When cover is non-nil, every byte read is also kept in it.
func readPageConfig(e archive.Entry, r io.Reader, cover *bytes.Buffer) (int, int, error) {
	if e.Size > MaxEntryBytes {
		return 0, 0, ErrTooLarge
	}
	lr := &io.LimitedReader{R: r, N: MaxEntryBytes + 1}
	var src io.Reader = lr
	if cover != nil {
		src = io.TeeReader(lr, cover)
	}
	br := bufio.NewReader(src)
	cfg, _, err := image.DecodeConfig(br)
	if err != nil {
		return 0, 0, err
	}
	if int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return 0, 0, ErrTooLarge
	}
	if _, err := io.Copy(io.Discard, br); err != nil {
		return 0, 0, err
	}
	if lr.N <= 0 { // consumed MaxEntryBytes+1 bytes: the entry is over the limit
		return 0, 0, ErrTooLarge
	}
	return cfg.Width, cfg.Height, nil
}
