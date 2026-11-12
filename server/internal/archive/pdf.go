package archive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"os"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/filter"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// maxPDFBytes bounds the PDFs we open. pdfcpu parses the whole file, so every
// object stream and image stream of the book is held in memory while it is
// open: measured resident size is about 1x the file size (see
// TestPDFOpenMemory). A PDF is open only while the page cache extracts it, and
// the cache runs two extractions at once, so the peak is about 2x this bound.
// Larger files are refused. Lowering it would turn PDFs that open today into
// unsupported books, so it stays at 512 MB.
const maxPDFBytes = 512 << 20

// maxPDFImageBytes bounds the decoded pixels of one page image. A small Flate
// stream can inflate to far more than its own size, so the decoded size is
// worked out from the image dictionary and refused before any byte is
// inflated. It is a var so tests can lower it.
var maxPDFImageBytes int64 = 256 << 20

// maxPDFImageSide bounds the width and height of one page image.
const maxPDFImageSide = 20000

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
	// decodeLen is the number of decoded bytes to inflate before pdfcpu
	// renders the image, bounded by maxPDFImageBytes. Zero means no bounded
	// decode happens here (raw JPEG, and CMYK JPEG, which is checked by size).
	decodeLen int64
	sd        *types.StreamDict
}

// pdfArchive exposes each page's single image. The parsed PDF stays in
// memory; pdfcpu decodes images into it, so access is serialised by busy, a
// one-slot semaphore. A waiter gives up when its context ends.
type pdfArchive struct {
	file   *os.File
	ctx    *model.Context
	pages  []pdfPage
	byName map[string]int
	busy   chan struct{} // holds one token while a render or Close runs
	closed bool          // guarded by busy
}

// lock takes the archive's render slot, or returns c's error if c ends first.
func (pa *pdfArchive) lock(c context.Context) error {
	select {
	case pa.busy <- struct{}{}:
		return nil
	case <-c.Done():
		return c.Err()
	}
}

func (pa *pdfArchive) unlock() { <-pa.busy }

// openPDF reads the PDF on file without writing anything. Encrypted files
// and files whose pages are not one browser-servable image are unsupported.
// It gives up with c's error when c ends while the file is being read.
func openPDF(c context.Context, file *os.File, size int64) (Archive, error) {
	if err := c.Err(); err != nil {
		file.Close()
		return nil, err
	}
	if size > maxPDFBytes {
		file.Close()
		return nil, fmt.Errorf("%w: PDF over %d MB", ErrUnsupported, maxPDFBytes>>20)
	}
	pa := &pdfArchive{file: file, busy: make(chan struct{}, 1)}
	if err := pa.load(c, file); err != nil {
		file.Close()
		return nil, err
	}
	return pa, nil
}

// load parses the file and indexes its pages. A panic inside pdfcpu on a
// malformed file becomes an unsupported error.
func (pa *pdfArchive) load(c context.Context, file *os.File) (err error) {
	defer func() {
		if recover() != nil {
			err = errPDFUnreadable
		}
	}()
	conf := model.NewStatelessConfiguration() // no config file on disk
	conf.Cmd = model.EXTRACTIMAGES            // builds the image index
	conf.ValidationMode = model.ValidationRelaxed
	ctx, err := api.ReadValidateAndOptimize(c, file, conf, nil)
	if err != nil {
		if cerr := c.Err(); cerr != nil {
			return cerr
		}
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
	return pa.index(c)
}

// index picks the image of every page. It uses pdfcpu's metadata-only
// extraction, so no pixels are decoded here.
func (pa *pdfArchive) index(c context.Context) error {
	ctx := pa.ctx
	pa.byName = make(map[string]int, ctx.PageCount)
	for p := 1; p <= ctx.PageCount; p++ {
		if err := c.Err(); err != nil {
			return err
		}
		stubs, err := pdfcpu.ExtractPageImages(c, ctx, p, true)
		if err != nil {
			if cerr := c.Err(); cerr != nil {
				return cerr
			}
			return pdfUnsupported(p, "cannot be read")
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
			return pdfUnsupported(p, "is not a single image")
		}
		imgObj := ctx.Optimize.ImageObjects[objN]
		if imgObj == nil || imgObj.ImageDict == nil {
			return pdfUnsupported(p, "cannot be read")
		}
		sd := imgObj.ImageDict

		images, draws, inline, err := pageUse(c, ctx, p)
		if err != nil {
			if cerr := c.Err(); cerr != nil {
				return cerr
			}
			return pdfUnsupported(p, "cannot be read")
		}
		if images != 1 || draws != 1 || inline {
			return pdfUnsupported(p, "is not a single image")
		}

		ext, raw, err := pdfImageKind(p, stub, sd)
		if err != nil {
			return err
		}
		size := int64(-1)
		var decodeLen int64
		switch {
		case raw:
			if len(sd.Raw) == 0 {
				return pdfUnsupported(p, "has no image data")
			}
			size = int64(len(sd.Raw))
		case pdfLastFilter(sd) == filter.DCT:
			// pdfcpu decodes a CMYK JPEG in full, so its real size is checked first.
			if err := checkPDFJPEG(p, sd.Raw); err != nil {
				return err
			}
		default:
			decodeLen, err = pdfDecodedSize(p, stub, sd)
			if err != nil {
				return err
			}
		}
		name := fmt.Sprintf("%04d.%s", p, ext)
		pa.byName[name] = len(pa.pages)
		pa.pages = append(pa.pages, pdfPage{
			entry:     Entry{Name: name, Size: size},
			page:      p,
			objNr:     objN,
			resName:   stub.Name,
			ext:       ext,
			raw:       raw,
			decodeLen: decodeLen,
			sd:        sd,
		})
	}
	return nil
}

// pdfUnsupported is the error for a page that cannot be served.
func pdfUnsupported(page int, why string) error {
	return fmt.Errorf("%w: PDF page %d %s", ErrUnsupported, page, why)
}

// pdfLastFilter returns the name of the last filter of sd, or "" if none.
func pdfLastFilter(sd *types.StreamDict) string {
	if n := len(sd.FilterPipeline); n > 0 {
		return sd.FilterPipeline[n-1].Name
	}
	return ""
}

// pdfImageKind decides the entry type for a page's image. A JPEG that pdfcpu
// can leave alone is served as its own bytes; flate-style images are
// re-encoded to PNG by pdfcpu. Anything whose mask or Decode mapping pdfcpu's
// output would drop is refused.
func pdfImageKind(page int, st model.Image, sd *types.StreamDict) (ext string, raw bool, err error) {
	if st.IsImgMask {
		return "", false, pdfUnsupported(page, "uses an image mask")
	}
	if sd.Dict.HasEntry("SMask") || sd.Dict.HasEntry("Mask") || sd.Dict.HasEntry("Decode") {
		return "", false, pdfUnsupported(page, "uses an unsupported image encoding")
	}
	pipeline := sd.FilterPipeline
	// Only ASCII filters may precede the last one: a second inflating filter
	// would be decoded in full before the size cap applies.
	for _, f := range pipeline[:max(len(pipeline)-1, 0)] {
		if f.Name != filter.ASCII85 && f.Name != filter.ASCIIHex {
			return "", false, pdfUnsupported(page, "uses an unsupported image encoding")
		}
	}
	switch pdfLastFilter(sd) {
	case "", filter.Flate, filter.LZW, filter.RunLength:
		if st.Comp == 4 {
			// pdfcpu writes CMYK images as TIFF, which browsers cannot show.
			return "", false, pdfUnsupported(page, "uses CMYK colour")
		}
		return "png", false, nil
	case filter.DCT:
		if len(pipeline) != 1 {
			return "", false, pdfUnsupported(page, "uses an unsupported image encoding")
		}
		if st.Comp == 4 {
			return "png", false, nil // pdfcpu converts CMYK JPEG to PNG
		}
		if !pdfPlainJPEGSpace(st) {
			return "", false, pdfUnsupported(page, "uses an unsupported image encoding")
		}
		return "jpg", true, nil
	}
	return "", false, pdfUnsupported(page, "uses an unsupported image encoding")
}

// pdfPlainJPEGSpace reports whether a JPEG can be served as its own bytes:
// its colour space must be one a browser shows as the JPEG's components.
func pdfPlainJPEGSpace(st model.Image) bool {
	switch st.Cs {
	case string(model.DeviceRGBCS):
		return st.Comp == 3
	case string(model.DeviceGrayCS):
		return st.Comp == 1
	case string(model.ICCBasedCS):
		return st.Comp == 1 || st.Comp == 3
	}
	return false
}

// pdfPNGPredictor reports whether sd's last filter applies a PNG predictor,
// which puts one filter-type byte in front of every row.
func pdfPNGPredictor(sd *types.StreamDict) bool {
	n := len(sd.FilterPipeline)
	if n == 0 {
		return false
	}
	p := sd.FilterPipeline[n-1].DecodeParms.IntEntry("Predictor")
	return p != nil && *p >= 10
}

// pdfDecodedSize returns the bytes the pixels of a page image take once
// decoded, worked out from the image dictionary alone. It refuses images
// that are too large before anything is inflated.
func pdfDecodedSize(page int, st model.Image, sd *types.StreamDict) (int64, error) {
	w, h := int64(st.Width), int64(st.Height)
	comp, bpc := int64(st.Comp), int64(st.Bpc)
	if st.Cs == string(model.IndexedCS) {
		comp = 1 // pdfcpu reports the base colour space here; samples are indices
	}
	if w <= 0 || h <= 0 || comp <= 0 || bpc <= 0 {
		return 0, pdfUnsupported(page, "cannot be decoded")
	}
	if w > maxPDFImageSide || h > maxPDFImageSide {
		return 0, pdfUnsupported(page, "image is too large")
	}
	size := (w*comp*bpc + 7) / 8 * h
	if pdfPNGPredictor(sd) {
		size += h
	}
	if size > maxPDFImageBytes {
		return 0, pdfUnsupported(page, "image is too large")
	}
	return size, nil
}

// checkPDFJPEG refuses a CMYK JPEG whose real size is too large. The size is
// read from the JPEG header, since the dictionary's dimensions may be wrong.
func checkPDFJPEG(page int, raw []byte) error {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return pdfUnsupported(page, "cannot be decoded")
	}
	w, h := int64(cfg.Width), int64(cfg.Height)
	if w > maxPDFImageSide || h > maxPDFImageSide || w*h*4 > maxPDFImageBytes {
		return pdfUnsupported(page, "image is too large")
	}
	return nil
}

// pageUse counts what page p draws: the image XObjects its resources name,
// the Do operators in its content, and whether that content has an inline
// image. pdfcpu merges identical image objects, so a page that draws one
// picture twice looks like a single image; counting names and operators
// catches it.
func pageUse(c context.Context, ctx *model.Context, p int) (int, int, bool, error) {
	d, _, attrs, err := ctx.PageDict(c, p, true)
	if err != nil {
		return 0, 0, false, err
	}
	images := 0
	if attrs != nil && attrs.Resources != nil {
		o, err := ctx.DereferenceDictEntry(attrs.Resources, "XObject")
		if err != nil {
			return 0, 0, false, err
		}
		xobjs, _ := o.(types.Dict)
		for _, v := range xobjs {
			o, err := ctx.Dereference(v)
			if err != nil {
				return 0, 0, false, err
			}
			var xd types.Dict
			switch s := o.(type) {
			case types.StreamDict:
				xd = s.Dict
			case *types.StreamDict:
				if s != nil {
					xd = s.Dict
				}
			}
			if st := xd.Subtype(); st != nil && *st == "Image" {
				images++
			}
		}
	}
	content, err := ctx.PageContent(d, p)
	if err != nil {
		return 0, 0, false, err
	}
	draws, inline := contentOps(content)
	return images, draws, inline, nil
}

// contentOps counts the Do operators in a page content stream and reports
// whether the stream has an inline image (BI). Strings, names, hex strings
// and comments are skipped, so text that happens to spell Do is not counted.
func contentOps(b []byte) (draws int, inline bool) {
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case pdfSpace(c):
			i++
		case c == '%':
			for i < len(b) && b[i] != '\n' && b[i] != '\r' {
				i++
			}
		case c == '(':
			i = skipPDFString(b, i)
		case c == '<' && i+1 < len(b) && b[i+1] == '<', c == '>' && i+1 < len(b) && b[i+1] == '>':
			i += 2
		case c == '<':
			for i < len(b) && b[i] != '>' {
				i++
			}
			i++
		case c == '/':
			i++
			for i < len(b) && !pdfSpace(b[i]) && !pdfDelim(b[i]) {
				i++
			}
		case pdfDelim(c):
			i++
		default:
			start := i
			for i < len(b) && !pdfSpace(b[i]) && !pdfDelim(b[i]) {
				i++
			}
			switch string(b[start:i]) {
			case "Do":
				draws++
			case "BI":
				inline = true
			}
		}
	}
	return draws, inline
}

// skipPDFString returns the index just past the literal string that starts
// at b[i] == '('.
func skipPDFString(b []byte, i int) int {
	depth := 0
	for ; i < len(b); i++ {
		switch b[i] {
		case '\\':
			i++
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(b)
}

func pdfSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == 0
}

func pdfDelim(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

// decodeBounded inflates the image stream of pg into sd.Content, producing at
// most pg.decodeLen bytes. pdfcpu reuses Content rather than decoding again.
func decodeBounded(pg *pdfPage) error {
	data, err := pg.sd.DecodeLengthWithLimit(pg.decodeLen, maxPDFImageBytes)
	if err != nil {
		return err
	}
	pg.sd.Content = data
	return nil
}

// render returns the entry bytes for pg. Raw JPEG is returned without
// decoding; anything else goes through pdfcpu, and the decoded pixels are
// dropped again so only the page being served is in memory.
func (pa *pdfArchive) render(c context.Context, pg *pdfPage) (r io.Reader, err error) {
	defer func() {
		if recover() != nil {
			r, err = nil, pdfUnsupported(pg.page, "cannot be decoded")
		}
	}()
	if err := c.Err(); err != nil {
		return nil, err
	}
	if pg.raw {
		return bytes.NewReader(pg.sd.Raw), nil
	}
	if err := pa.lock(c); err != nil {
		return nil, err
	}
	defer pa.unlock()
	if pa.closed {
		return nil, errors.New("archive: PDF is closed")
	}
	defer func() { pg.sd.Content = nil }()
	// The inflate below cannot be interrupted once started (it is bounded by
	// maxPDFImageBytes), so the context is checked on both sides of it.
	if pg.decodeLen > 0 {
		if err := c.Err(); err != nil {
			return nil, err
		}
		if err := decodeBounded(pg); err != nil {
			if cerr := c.Err(); cerr != nil {
				return nil, cerr
			}
			return nil, pdfUnsupported(pg.page, "cannot be decoded")
		}
		if err := c.Err(); err != nil {
			return nil, err
		}
	}
	img, err := pdfcpu.ExtractImage(c, pa.ctx, pg.sd, false, pg.resName, pg.objNr, false)
	if err != nil {
		if cerr := c.Err(); cerr != nil {
			return nil, cerr
		}
		return nil, pdfUnsupported(pg.page, "cannot be decoded")
	}
	if err := c.Err(); err != nil {
		return nil, err
	}
	if img.FileType != pg.ext {
		return nil, pdfUnsupported(pg.page, "cannot be decoded")
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
	return pa.OpenContext(context.Background(), name)
}

// OpenContext is Open that gives up with c's error when c ends, including
// while it waits for another page's render to finish.
func (pa *pdfArchive) OpenContext(c context.Context, name string) (io.ReadCloser, error) {
	i, ok := pa.byName[name]
	if !ok {
		return nil, notExist(name)
	}
	r, err := pa.render(c, &pa.pages[i])
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
	// Wait for a running render, then drop the parsed PDF (pdfcpu's context
	// holds the file's objects) so its memory is reclaimed once the archive
	// value itself is unreferenced.
	pa.busy <- struct{}{}
	pa.closed = true
	pa.ctx = nil
	<-pa.busy
	return pa.file.Close()
}
