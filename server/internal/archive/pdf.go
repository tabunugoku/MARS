package archive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/filter"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// maxPDFBytes bounds the PDFs we open. pdfcpu parses the whole file, so every
// object stream and image stream of the book is held in memory while it is
// open. Larger files are refused rather than risking the server's memory.
const maxPDFBytes = 512 << 20

var (
	errPDFEncrypted  = fmt.Errorf("%w: encrypted PDF", ErrUnsupported)
	errPDFUnreadable = fmt.Errorf("%w: not a readable PDF", ErrUnsupported)
)

// pdfPage is one page of the book, served as one image entry.
type pdfPage struct {
	entry   Entry
	page    int    // 1-based page number
	objNr   int    // the page's only image object
	resName string // resource name of the image on the page
	ext     string // "jpg" or "png", the entry's extension
	raw     bool   // serve sd.Raw unchanged (a single DCTDecode filter)
	sd      *types.StreamDict
}

// pdfArchive exposes each page's single image. The parsed PDF stays in
// memory; pdfcpu decodes images into it, so access is serialised by mu.
type pdfArchive struct {
	file   *os.File
	ctx    *model.Context
	pages  []pdfPage
	byName map[string]int
	mu     sync.Mutex
	closed bool
}

// openPDF reads the PDF on file without writing anything. Encrypted files
// and files whose pages are not one browser-servable image are unsupported.
func openPDF(file *os.File, size int64) (Archive, error) {
	if size > maxPDFBytes {
		file.Close()
		return nil, fmt.Errorf("%w: PDF over %d MB", ErrUnsupported, maxPDFBytes>>20)
	}
	pa := &pdfArchive{file: file}
	if err := pa.load(file); err != nil {
		file.Close()
		return nil, err
	}
	return pa, nil
}

// load parses the file and indexes its pages. A panic inside pdfcpu on a
// malformed file becomes an unsupported error.
func (pa *pdfArchive) load(file *os.File) (err error) {
	defer func() {
		if recover() != nil {
			err = errPDFUnreadable
		}
	}()
	conf := model.NewStatelessConfiguration() // no config file on disk
	conf.Cmd = model.EXTRACTIMAGES            // builds the image index
	conf.ValidationMode = model.ValidationRelaxed
	ctx, err := api.ReadValidateAndOptimize(context.Background(), file, conf, nil)
	if err != nil {
		if errors.Is(err, pdfcpu.ErrWrongPassword) || errors.Is(err, pdfcpu.ErrEncrypted) {
			return errPDFEncrypted
		}
		return errPDFUnreadable
	}
	if ctx.Encrypt != nil {
		// Owner-only encryption opens with an empty password, so check here too.
		return errPDFEncrypted
	}
	pa.ctx = ctx
	return pa.index()
}

// index picks the image of every page. It uses pdfcpu's metadata-only
// extraction, so no pixels are decoded here.
func (pa *pdfArchive) index() error {
	ctx := pa.ctx
	c := context.Background()
	pa.byName = make(map[string]int, ctx.PageCount)
	for p := 1; p <= ctx.PageCount; p++ {
		stubs, err := pdfcpu.ExtractPageImages(c, ctx, p, true)
		if err != nil {
			return fmt.Errorf("%w: PDF page %d cannot be read", ErrUnsupported, p)
		}
		var (
			n    int
			stub model.Image
			objN int
		)
		for objNr, st := range stubs {
			if st.Thumb {
				continue // the page thumbnail is not a page image
			}
			n++
			stub, objN = st, objNr
		}
		if n != 1 {
			return fmt.Errorf("%w: PDF page %d is not a single image", ErrUnsupported, p)
		}
		imgObj := ctx.Optimize.ImageObjects[objN]
		if imgObj == nil || imgObj.ImageDict == nil {
			return fmt.Errorf("%w: PDF page %d cannot be read", ErrUnsupported, p)
		}
		sd := imgObj.ImageDict
		ext, raw, err := pdfImageKind(p, stub, sd)
		if err != nil {
			return err
		}
		size := int64(-1)
		if raw {
			if len(sd.Raw) == 0 {
				return fmt.Errorf("%w: PDF page %d has no image data", ErrUnsupported, p)
			}
			size = int64(len(sd.Raw))
		}
		name := fmt.Sprintf("%04d.%s", p, ext)
		pa.byName[name] = len(pa.pages)
		pa.pages = append(pa.pages, pdfPage{
			entry:   Entry{Name: name, Size: size},
			page:    p,
			objNr:   objN,
			resName: stub.Name,
			ext:     ext,
			raw:     raw,
			sd:      sd,
		})
	}
	return nil
}

// pdfImageKind decides the entry type for a page's image. JPEG (single
// DCTDecode) is served as its own bytes; flate-style images are re-encoded
// to PNG by pdfcpu.
func pdfImageKind(page int, st model.Image, sd *types.StreamDict) (ext string, raw bool, err error) {
	unsupported := func(why string) error {
		return fmt.Errorf("%w: PDF page %d %s", ErrUnsupported, page, why)
	}
	if st.IsImgMask {
		return "", false, unsupported("uses an image mask")
	}
	pipeline := sd.FilterPipeline
	last := ""
	if n := len(pipeline); n > 0 {
		last = pipeline[n-1].Name
	}
	switch last {
	case "", filter.Flate, filter.LZW, filter.RunLength:
		if st.Comp == 4 {
			// pdfcpu writes CMYK images as TIFF, which browsers cannot show.
			return "", false, unsupported("uses CMYK colour")
		}
		return "png", false, nil
	case filter.DCT:
		if len(pipeline) != 1 {
			return "", false, unsupported("uses an unsupported image encoding")
		}
		if st.Comp == 4 {
			return "png", false, nil // pdfcpu converts CMYK JPEG to PNG
		}
		return "jpg", true, nil
	}
	return "", false, unsupported("uses an unsupported image encoding")
}

// render returns the entry bytes for pg. Raw JPEG is returned without
// decoding; anything else goes through pdfcpu, and the decoded pixels are
// dropped again so only the page being served is in memory.
func (pa *pdfArchive) render(c context.Context, pg *pdfPage) (r io.Reader, err error) {
	defer func() {
		if recover() != nil {
			r, err = nil, fmt.Errorf("%w: PDF page %d cannot be decoded", ErrUnsupported, pg.page)
		}
	}()
	if pg.raw {
		return bytes.NewReader(pg.sd.Raw), nil
	}
	pa.mu.Lock()
	defer pa.mu.Unlock()
	if pa.closed {
		return nil, errors.New("archive: PDF is closed")
	}
	defer func() { pg.sd.Content = nil }()
	img, err := pdfcpu.ExtractImage(c, pa.ctx, pg.sd, false, pg.resName, pg.objNr, false)
	if err != nil {
		return nil, fmt.Errorf("%w: PDF page %d cannot be decoded", ErrUnsupported, pg.page)
	}
	if img.FileType != pg.ext {
		return nil, fmt.Errorf("%w: PDF page %d cannot be decoded", ErrUnsupported, pg.page)
	}
	return img.Reader, nil
}

func (pa *pdfArchive) Entries() []Entry {
	out := make([]Entry, len(pa.pages))
	for i := range pa.pages {
		out[i] = pa.pages[i].entry
	}
	return out
}

func (pa *pdfArchive) Open(name string) (io.ReadCloser, error) {
	i, ok := pa.byName[name]
	if !ok {
		return nil, notExist(name)
	}
	r, err := pa.render(context.Background(), &pa.pages[i])
	if err != nil {
		return nil, err
	}
	return io.NopCloser(r), nil
}

func (pa *pdfArchive) Walk(fn func(e Entry, r io.Reader) error) error {
	return pa.walkLimited(newBudget(Limit{}), fn)
}

func (pa *pdfArchive) walkLimited(b *budget, fn func(e Entry, r io.Reader) error) error {
	for i := range pa.pages {
		pg := &pa.pages[i]
		r, err := pa.render(b.ctx, pg)
		if err != nil {
			return err
		}
		err = fn(pg.entry, b.reader(r))
		if errors.Is(err, SkipRest) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (pa *pdfArchive) RandomAccess() bool { return false }

func (pa *pdfArchive) Close() error {
	pa.mu.Lock()
	pa.closed = true
	pa.ctx = nil
	pa.mu.Unlock()
	return pa.file.Close()
}
