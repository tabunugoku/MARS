package archive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

// pdfRenderLongSide is the long side of a drawn page, in pixels.
const pdfRenderLongSide = 2000

// pdfRenderSlots bounds how many pages render at once. A pdfium instance is
// single-threaded, so each slot is one instance in the pool.
const pdfRenderSlots = 2

// PDFRenderSlots is the number of PDF pages that can be drawn at once. Callers
// that run PDF analyses in parallel limit themselves to it, so the wait for an
// instance happens before a caller's timeout starts.
const PDFRenderSlots = pdfRenderSlots

// pdfiumMemoryPages caps one instance's WebAssembly memory, in 64 KiB pages:
// 1 GiB. A page needing more fails to draw instead of growing the process.
const pdfiumMemoryPages = 16384

// pdfRenderTimeout bounds one use of an instance (opening the document and
// drawing or measuring), whatever the caller's own deadline.
var pdfRenderTimeout = time.Minute

// ErrRendererBusy is returned when no PDF renderer could be had: the caller's
// context ended while every instance was in use, or the renderer could not
// start. It says nothing about the file, so a book must not be marked
// unsupported for it; trying again later can succeed.
var ErrRendererBusy = errors.New("archive: PDF renderer unavailable")

// pdfiumRenders counts the page renders done, for tests.
var pdfiumRenders atomic.Int64

var (
	pdfiumOnce sync.Once
	pdfiumPool pdfium.Pool
	pdfiumErr  error
)

// pdfiumGet returns the instance pool, creating it on first use. Compiling
// the WebAssembly module takes a couple of seconds, so a server that never
// draws a page never pays for it. A failed start is remembered.
func pdfiumGet() (pdfium.Pool, error) {
	pdfiumOnce.Do(func() {
		pdfiumPool, pdfiumErr = webassembly.Init(webassembly.Config{
			MinIdle:  0,
			MaxIdle:  1,
			MaxTotal: pdfRenderSlots,
			// Nothing is mounted: pdfium reads the PDF through FileReader only.
			FSConfig: wazero.NewFSConfig(),
			// The exception-handling feature is what go-pdfium's default
			// config enables (its module needs it). CloseOnContextDone lets
			// Kill stop a running call; the memory limit bounds what one
			// hostile page can take.
			RuntimeConfig: wazero.NewRuntimeConfig().
				WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling).
				WithCloseOnContextDone(true).
				WithMemoryLimitPages(pdfiumMemoryPages),
			Stdout: io.Discard,
			Stderr: io.Discard,
		})
	})
	return pdfiumPool, pdfiumErr
}

// withPDFDocument takes an instance from the pool, opens the PDF read from r
// and calls fn with it. The document is closed and the instance returned when
// fn returns, and a panic inside pdfium becomes an error.
func withPDFDocument(c context.Context, r io.ReaderAt, size int64,
	fn func(inst pdfium.Pdfium, doc references.FPDF_DOCUMENT) error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("pdfium: panic: %v", p)
		}
	}()
	pool, err := pdfiumGet()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRendererBusy, err)
	}
	// Waiting for a free instance is bounded by the caller only; a wait that
	// ends is not the PDF's fault, so it is reported as ErrRendererBusy.
	inst, err := pool.GetInstanceWithContext(c)
	if err != nil {
		if cerr := c.Err(); cerr != nil {
			err = cerr // the pool's own message hides that the caller gave up
		}
		return fmt.Errorf("%w: %w", ErrRendererBusy, err)
	}
	// The time limit applies to the work itself, from here on.
	c, cancel := context.WithTimeout(c, pdfRenderTimeout)
	defer cancel()
	// When c ends mid-call, Kill interrupts the running WebAssembly and drops
	// the instance; the pool starts a fresh one when it is next needed.
	stop := context.AfterFunc(c, func() { _ = inst.Kill() })
	defer func() {
		if stop() { // not killed: give the instance back to the pool
			_ = inst.Close()
		}
	}()
	doc, err := inst.OpenDocument(&requests.OpenDocument{
		FileReader:     io.NewSectionReader(r, 0, size),
		FileReaderSize: size,
	})
	if err != nil {
		return err
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	return fn(inst, doc.Document)
}

// pdfRenderDims returns the pixel size of a page of wPt x hPt points: the long
// side is pdfRenderLongSide and the other side keeps the aspect ratio, rounded
// up the way pdfium rounds it, and at least 1 pixel. The float operations
// mirror pdfium's own sizing, so a render asked for one long side comes back
// with exactly these dimensions.
func pdfRenderDims(wPt, hPt float64) (w, h int, err error) {
	if !(wPt > 0 && hPt > 0) || math.IsInf(wPt, 0) || math.IsInf(hPt, 0) {
		return 0, 0, fmt.Errorf("archive: invalid PDF page size %g x %g", wPt, hPt)
	}
	long := float64(pdfRenderLongSide)
	if wPt >= hPt {
		return pdfRenderLongSide, max(1, int(math.Ceil(long*(hPt/wPt)))), nil
	}
	return max(1, int(math.Ceil(long*(wPt/hPt)))), pdfRenderLongSide, nil
}

// pdfPageSize returns the size of page (1-based) in points.
func pdfPageSize(inst pdfium.Pdfium, doc references.FPDF_DOCUMENT, page int) (wPt, hPt float64, err error) {
	pc, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc})
	if err != nil {
		return 0, 0, err
	}
	if page < 1 || page > pc.PageCount {
		return 0, 0, fmt.Errorf("page %d out of range (%d pages)", page, pc.PageCount)
	}
	sz, err := inst.FPDF_GetPageSizeByIndex(&requests.FPDF_GetPageSizeByIndex{Document: doc, Index: page - 1})
	if err != nil {
		return 0, 0, err
	}
	return sz.Width, sz.Height, nil
}

// pdfRenderFailure is the error for a failed render of page. A busy renderer
// and a context error pass through unchanged, an unsupported error is kept,
// and anything else makes the page unrenderable.
func pdfRenderFailure(c context.Context, err error, page int) error {
	if errors.Is(err, ErrRendererBusy) {
		return err
	}
	if cerr := c.Err(); cerr != nil {
		return cerr
	}
	if errors.Is(err, ErrUnsupported) {
		return err
	}
	return pdfUnsupported(page, "cannot be rendered")
}

// renderPDFPagePNG draws page (1-based) of the PDF of the given size read
// from r, and returns it as a PNG on an opaque white background. The PNG is
// exactly pdfRenderDims of the page.
func renderPDFPagePNG(c context.Context, r io.ReaderAt, size int64, page int) ([]byte, error) {
	if err := c.Err(); err != nil {
		return nil, err
	}
	var out []byte
	err := withPDFDocument(c, r, size, func(inst pdfium.Pdfium, doc references.FPDF_DOCUMENT) error {
		wPt, hPt, err := pdfPageSize(inst, doc, page)
		if err != nil {
			return err
		}
		w, h, err := pdfRenderDims(wPt, hPt)
		if err != nil {
			return err
		}
		req := &requests.RenderPageInPixels{
			Page: requests.Page{ByIndex: &requests.PageByIndex{Document: doc, Index: page - 1}},
			// Annotations with an appearance (stamps, free text, ink) are
			// part of what the page shows; pdfium leaves them out otherwise.
			RenderFlags: enums.FPDF_RENDER_FLAG_ANNOT,
		}
		// Only the long side is asked for; pdfium derives the other from the
		// page's aspect ratio, the same way pdfRenderDims does.
		if wPt >= hPt {
			req.Width = pdfRenderLongSide
		} else {
			req.Height = pdfRenderLongSide
		}
		pdfiumRenders.Add(1)
		res, err := inst.RenderPageInPixels(req)
		if err != nil {
			return err
		}
		defer res.Cleanup()
		img := res.Result.RenderedImage
		if img == nil || res.Result.Width != w || res.Result.Height != h ||
			img.Bounds().Dx() != w || img.Bounds().Dy() != h {
			return fmt.Errorf("pdfium rendered a different size than %dx%d", w, h)
		}
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
		draw.Draw(dst, dst.Bounds(), img, img.Bounds().Min, draw.Over)
		var buf bytes.Buffer
		enc := png.Encoder{CompressionLevel: png.BestSpeed}
		if err := enc.Encode(&buf, dst); err != nil {
			return err
		}
		out = buf.Bytes()
		return nil
	})
	if err != nil {
		return nil, pdfRenderFailure(c, err, page)
	}
	return out, nil
}

// pdfRenderedSizes returns the pixel size of each page in pages, read from one
// document open. Nothing is drawn.
func pdfRenderedSizes(c context.Context, r io.ReaderAt, size int64, pages []int) (map[int][2]int, error) {
	if err := c.Err(); err != nil {
		return nil, err
	}
	out := make(map[int][2]int, len(pages))
	err := withPDFDocument(c, r, size, func(inst pdfium.Pdfium, doc references.FPDF_DOCUMENT) error {
		for _, p := range pages {
			if err := c.Err(); err != nil {
				return err
			}
			wPt, hPt, err := pdfPageSize(inst, doc, p)
			if err == nil {
				var w, h int
				w, h, err = pdfRenderDims(wPt, hPt)
				out[p] = [2]int{w, h}
			}
			if err != nil {
				return pdfRenderFailure(c, err, p)
			}
		}
		return nil
	})
	if err != nil {
		return nil, pdfRenderFailure(c, err, pages[0])
	}
	return out, nil
}

// renderedPDFName is the entry name of a drawn page.
func renderedPDFName(page int) string {
	return fmt.Sprintf("%04d-rendered.png", page)
}

var renderedPDFNameRe = regexp.MustCompile(`^([0-9]{4,})-rendered\.png$`)

// RenderedPDFPage reports whether entry is the name of a page drawn by
// pdfium ("0007-rendered.png"), and its 1-based page number.
func RenderedPDFPage(entry string) (page int, ok bool) {
	m := renderedPDFNameRe.FindStringSubmatch(entry)
	if m == nil {
		return 0, false
	}
	page, err := strconv.Atoi(m[1])
	if err != nil || page < 1 {
		return 0, false
	}
	return page, true
}

// RenderPDFPage draws page (1-based) of the PDF at p as a PNG, without
// parsing the file with pdfcpu. Files over maxPDFBytes are ErrUnsupported.
// The file is opened through os.Root like OpenPathContext does.
func RenderPDFPage(ctx context.Context, p Path, page int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(p.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(filepath.FromSlash(p.Rel))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %q is not a regular file", ErrUnsupported, p.Rel)
	}
	if st.Size() > maxPDFBytes {
		return nil, fmt.Errorf("%w: PDF over %d MB", ErrUnsupported, maxPDFBytes>>20)
	}
	return renderPDFPagePNG(ctx, f, st.Size(), page)
}
