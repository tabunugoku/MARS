package archive

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPDFRenderDims(t *testing.T) {
	for _, tc := range []struct {
		w, h         float64
		wantW, wantH int
	}{
		{595, 842, 1414, 2000}, // A4 portrait
		{842, 595, 2000, 1414}, // A4 landscape
		{200, 200, 2000, 2000}, // square
		{1, 100000, 1, 2000},   // the other side is never below 1 pixel
		{100000, 1, 2000, 1},
	} {
		w, h, err := pdfRenderDims(tc.w, tc.h)
		if err != nil || w != tc.wantW || h != tc.wantH {
			t.Errorf("pdfRenderDims(%g, %g) = %d, %d, %v; want %d, %d", tc.w, tc.h, w, h, err, tc.wantW, tc.wantH)
		}
	}
	for _, bad := range [][2]float64{{0, 100}, {100, 0}, {-1, 100}, {math.NaN(), 100}, {math.Inf(1), 100}} {
		if _, _, err := pdfRenderDims(bad[0], bad[1]); err == nil {
			t.Errorf("pdfRenderDims(%g, %g) err = nil, want an error", bad[0], bad[1])
		}
	}
}

// measureRow is one PDF of the render measurement.
type measureRow struct {
	name     string
	fileMB   float64
	pages    int
	openMS   float64
	renderMS float64
	avgBytes float64 // average bytes served per page, pages 1-10
	pngKB    string  // average PNG size of the drawn pages among pages 1-10
}

// TestPDFRenderMeasure builds three 100-page PDFs and logs what opening them
// and drawing their first ten pages costs. It runs only on request:
// MARS_PDF_MEASURE=1 go test ./internal/archive -run TestPDFRenderMeasure -v
func TestPDFRenderMeasure(t *testing.T) {
	if os.Getenv("MARS_PDF_MEASURE") != "1" {
		t.Skip("set MARS_PDF_MEASURE=1 to measure PDF rendering")
	}
	const pages = 100
	box := [2]int{595, 842}
	dense := make([]pdfPageSpec, pages)
	mixed := make([]pdfPageSpec, pages)
	images := make([]pdfPageSpec, pages)
	for i := 0; i < pages; i++ {
		dense[i] = pdfPageSpec{text: true, lines: 50, box: box}
		// Odd pages: a picture with visible text on top, which is drawn.
		// Even pages: a scan with an OCR layer, served as its own image.
		mixed[i] = pdfPageSpec{images: []pdfObj{jpegObj(t, 1200, 1700)}, lines: 20 * ((i + 1) % 2), ocr: i%2 == 1, box: box, fill: true}
		images[i] = pdfPageSpec{images: []pdfObj{jpegObj(t, 1200, 1700)}, box: box, fill: true}
	}
	// Start the WebAssembly module before timing, so the one-off compile
	// (about 2.6 s) is not counted in the first render.
	if _, err := pdfiumGet(); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		pages []pdfPageSpec
	}{
		{"text only, 50 lines per page", dense},
		{"mixed: odd pages image + text, even pages OCR scans", mixed},
		{"one image per page (1200x1700 JPEG)", images},
	}
	ctx := context.Background()
	var rows []measureRow
	for _, c := range cases {
		data := makePDF(c.pages)
		path := writePDFFile(t, data)

		start := time.Now()
		a, _, err := OpenPathContext(ctx, PathOf(path))
		if err != nil {
			t.Fatalf("%s: open: %v", c.name, err)
		}
		openMS := float64(time.Since(start).Microseconds()) / 1000

		entries := a.Entries()
		const first = 10
		var served int64
		var pngBytes int64
		var drawn int
		start = time.Now()
		for _, e := range entries[:first] {
			b := readEntry(t, a, e.Name)
			served += int64(len(b))
			if _, ok := RenderedPDFPage(e.Name); ok {
				pngBytes += int64(len(b))
				drawn++
			}
		}
		renderMS := float64(time.Since(start).Microseconds()) / 1000 / first
		a.Close()

		row := measureRow{
			name:     c.name,
			fileMB:   float64(len(data)) / (1 << 20),
			pages:    len(entries),
			openMS:   openMS,
			renderMS: renderMS,
			avgBytes: float64(served) / first,
			pngKB:    "n/a (no drawn page)",
		}
		if drawn > 0 {
			row.pngKB = fmt.Sprintf("%.1f KB (%d drawn)", float64(pngBytes)/float64(drawn)/1024, drawn)
		}
		rows = append(rows, row)
	}

	var sb strings.Builder
	sb.WriteString("| PDF | file | pages | open (index) | render, pages 1-10 (avg ms/page) | avg bytes served per page | avg PNG size |\n")
	sb.WriteString("|---|---|---|---|---|---|---|\n")
	for _, r := range rows {
		fmt.Fprintf(&sb, "| %s | %.1f MB | %d | %.0f ms | %.1f ms | %.1f KB | %s |\n",
			r.name, r.fileMB, r.pages, r.openMS, r.renderMS, r.avgBytes/1024, r.pngKB)
	}
	t.Logf("\n%s", sb.String())
}

// TestPDFRenderTimeoutKillsAndRecovers ends renders through the instance
// time limit, which kills the instance mid-use, and checks the pool then
// draws again normally.
func TestPDFRenderTimeoutKillsAndRecovers(t *testing.T) {
	data := makePDF([]pdfPageSpec{{lines: 50, box: [2]int{595, 842}}})
	path := writePDFFile(t, data)
	if _, err := pdfiumGet(); err != nil {
		t.Fatal(err)
	}
	old := pdfRenderTimeout
	t.Cleanup(func() { pdfRenderTimeout = old })
	ctx := context.Background()
	pdfRenderTimeout = 200 * time.Microsecond
	failed := 0
	for i := 0; i < 20; i++ {
		if _, err := RenderPDFPage(ctx, PathOf(path), 1); err != nil {
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("timed-out render: err = %v, want ErrUnsupported", err)
			}
			failed++
		}
	}
	if failed == 0 {
		t.Log("no render hit the time limit on this machine")
	}
	pdfRenderTimeout = old
	for i := 0; i < 3; i++ {
		if _, err := RenderPDFPage(ctx, PathOf(path), 1); err != nil {
			t.Fatalf("render after timeouts: %v", err)
		}
	}
}

// TestPDFRenderCanceledContext returns the caller's context error.
func TestPDFRenderCanceledContext(t *testing.T) {
	path := writePDFFile(t, makePDF([]pdfPageSpec{{text: true}}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RenderPDFPage(ctx, PathOf(path), 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
