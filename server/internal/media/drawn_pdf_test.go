package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tabunugoku/mars/server/internal/archive"
)

// drawnTestPage is one page of a test PDF: a full-page JPEG, text, or both.
type drawnTestPage struct {
	jpeg bool
	text string
}

// drawnTestPDF writes a minimal PDF with the given pages (200x200 points each)
// to a temp dir and returns its path. Its xref table is correct, so the
// archive reader accepts it.
func drawnTestPDF(t *testing.T, pages []drawnTestPage) string {
	t.Helper()
	objs := []pdfTestObj{
		{body: "<< /Type /Catalog /Pages 2 0 R >>"},
		{}, // 2: page tree, filled below
		{body: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"}, // 3
	}
	var kids string
	for _, pg := range pages {
		pageNum := len(objs) + 1
		kids += fmt.Sprintf("%d 0 R ", pageNum)
		objs = append(objs, pdfTestObj{}) // page, filled below
		contentNum := len(objs) + 1
		objs = append(objs, pdfTestObj{}) // content stream, filled below

		var xobjs, draw string
		if pg.jpeg {
			imgNum := len(objs) + 1
			jp := jpegBytes(t, 100, 100)
			objs = append(objs, pdfTestObj{
				body: "/Type /XObject /Subtype /Image /Width 100 /Height 100 " +
					"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode",
				stream: jp,
			})
			xobjs = fmt.Sprintf("/XObject << /Im1 %d 0 R >>", imgNum)
			draw = "q 200 0 0 200 0 0 cm /Im1 Do Q\n"
		}
		if pg.text != "" {
			draw += fmt.Sprintf("BT /F1 12 Tf 10 180 Td (%s) Tj ET\n", pg.text)
		}
		objs[pageNum-1] = pdfTestObj{body: fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] "+
				"/Resources << /Font << /F1 3 0 R >> %s >> /Contents %d 0 R >>", xobjs, contentNum)}
		objs[contentNum-1] = pdfTestObj{stream: []byte(draw)}
	}
	objs[1] = pdfTestObj{body: fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids, len(pages))}

	path := filepath.Join(t.TempDir(), "book.pdf")
	if err := os.WriteFile(path, buildDrawnPDF(objs), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// pdfTestObj is one PDF object: a dictionary body, plus stream bytes when
// stream is set.
type pdfTestObj struct {
	body   string
	stream []byte
}

// buildDrawnPDF writes objs as numbered objects with an xref table and trailer.
func buildDrawnPDF(objs []pdfTestObj) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n", i+1)
		if o.stream == nil {
			buf.WriteString(o.body)
		} else {
			fmt.Fprintf(&buf, "<< %s /Length %d >>\nstream\n", o.body, len(o.stream))
			buf.Write(o.stream)
			buf.WriteString("\nendstream")
		}
		buf.WriteString("\nendobj\n")
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offs {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return buf.Bytes()
}

// cachedNames lists the file names in a book's cache dir, sorted.
func cachedNames(t *testing.T, c *PageCache, bookID string) []string {
	t.Helper()
	des, err := os.ReadDir(filepath.Join(c.Dir, bookID))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, de := range des {
		names = append(names, de.Name())
	}
	sort.Strings(names)
	return names
}

func TestAnalyzeDrawnPagesAreSizedNotDrawn(t *testing.T) {
	path := drawnTestPDF(t, []drawnTestPage{{text: "one"}, {text: "two"}, {text: "three"}, {text: "four"}})
	a, err := Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Pages) != 4 {
		t.Fatalf("%d pages, want 4", len(a.Pages))
	}
	c := &PageCache{Dir: t.TempDir(), MaxBytes: 1 << 30}
	for i, p := range a.Pages {
		if want := fmt.Sprintf("%04d-rendered.png", i+1); p.Entry != want {
			t.Errorf("page %d entry %q, want %q", i+1, p.Entry, want)
		}
		if p.MediaType != "image/png" {
			t.Errorf("page %d media type %q, want image/png", i+1, p.MediaType)
		}
		data, err := readPage(context.Background(), c, "book1", "v1", path, a.Pages, i+1)
		if err != nil {
			t.Fatalf("Open(%d): %v", i+1, err)
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || format != "png" {
			t.Fatalf("page %d does not decode as PNG: %v (%s)", i+1, err, format)
		}
		if cfg.Width != p.Width || cfg.Height != p.Height {
			t.Errorf("page %d analysed as %dx%d, served as %dx%d", i+1, p.Width, p.Height, cfg.Width, cfg.Height)
		}
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(a.Cover))
	if err != nil || format != "png" || a.CoverPage != 1 {
		t.Fatalf("cover: format %q, page %d, err %v (want png, page 1)", format, a.CoverPage, err)
	}
	if cfg.Width != a.Pages[0].Width {
		t.Errorf("cover width %d, page 1 width %d", cfg.Width, a.Pages[0].Width)
	}
}

func TestPageCacheDrawsOnlyRequestedPage(t *testing.T) {
	ctx := context.Background()
	path := drawnTestPDF(t, []drawnTestPage{{text: "one"}, {text: "two"}, {text: "three"}, {text: "four"}})
	a, err := Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	c := &PageCache{Dir: t.TempDir(), MaxBytes: 1 << 30}

	first, err := readPage(ctx, c, "book1", "v1", path, a.Pages, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := cachedNames(t, c, "book1"); strings.Join(got, ",") != "3,key" {
		t.Fatalf("after page 3: cache holds %v, want [3 key]", got)
	}

	// The file is not rewritten on a hit: a marker written into it survives.
	marker := filepath.Join(c.Dir, "book1", "3")
	if err := os.WriteFile(marker, []byte("marker"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := readPage(ctx, c, "book1", "v1", path, a.Pages, 3)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != "marker" {
		t.Errorf("hit rewrote page 3 (got %d bytes, first render was %d)", len(again), len(first))
	}

	// A different key removes the directory and starts again with page 3 only.
	if _, err := readPage(ctx, c, "book1", "v2", path, a.Pages, 3); err != nil {
		t.Fatal(err)
	}
	if got := cachedNames(t, c, "book1"); strings.Join(got, ",") != "3,key" {
		t.Errorf("after key change: cache holds %v, want [3 key]", got)
	}
	key, err := os.ReadFile(filepath.Join(c.Dir, "book1", "key"))
	if err != nil || !strings.HasPrefix(string(key), "v2-") {
		t.Errorf("key file = %q, %v; want v2 and the page list hash", key, err)
	}
}

func TestPageCacheMixedPDFWritesPagesOnDemand(t *testing.T) {
	ctx := context.Background()
	path := drawnTestPDF(t, []drawnTestPage{{jpeg: true}, {text: "hello"}})
	a, err := Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Pages) != 2 {
		t.Fatalf("%d pages, want 2", len(a.Pages))
	}
	if _, drawn := archive.RenderedPDFPage(a.Pages[0].Entry); drawn {
		t.Fatalf("page 1 (%q) should be an image, not drawn", a.Pages[0].Entry)
	}
	if _, drawn := archive.RenderedPDFPage(a.Pages[1].Entry); !drawn {
		t.Fatalf("page 2 (%q) should be drawn", a.Pages[1].Entry)
	}
	c := &PageCache{Dir: t.TempDir(), MaxBytes: 1 << 30}

	p1, err := readPage(ctx, c, "book1", "v1", path, a.Pages, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := cachedNames(t, c, "book1"); strings.Join(got, ",") != "1,key" {
		t.Fatalf("after page 1: cache holds %v, want [1 key]", got)
	}

	p2, err := readPage(ctx, c, "book1", "v1", path, a.Pages, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := cachedNames(t, c, "book1"); strings.Join(got, ",") != "1,2,key" {
		t.Fatalf("after page 2: cache holds %v, want [1 2 key]", got)
	}

	if _, format, err := image.DecodeConfig(bytes.NewReader(p1)); err != nil || format != "jpeg" {
		t.Errorf("page 1 does not decode as JPEG: %v (%s)", err, format)
	}
	if _, format, err := image.DecodeConfig(bytes.NewReader(p2)); err != nil || format != "png" {
		t.Errorf("page 2 does not decode as PNG: %v (%s)", err, format)
	}
}

func TestPageCacheDrawnPagesKeepBookBudget(t *testing.T) {
	// Pages kept one at a time count toward MaxExtractedBook like a whole
	// extraction does. A sparse file stands in for 2 GiB of pages.
	ctx := context.Background()
	path := drawnTestPDF(t, []drawnTestPage{{text: "one"}, {text: "two"}})
	a, err := Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	c := &PageCache{Dir: t.TempDir(), MaxBytes: 4 << 30}
	if _, err := readPage(ctx, c, "book1", "v1", path, a.Pages, 1); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(c.Dir, "book1", "filler"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxExtractedBook); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	if _, err := readPage(ctx, c, "book1", "v1", path, a.Pages, 2); !errors.Is(err, ErrCacheTooBig) {
		t.Fatalf("err = %v, want ErrCacheTooBig", err)
	}
	if _, err := os.Stat(filepath.Join(c.Dir, "book1", "2")); !os.IsNotExist(err) {
		t.Errorf("page 2 was kept over the budget: %v", err)
	}
}
