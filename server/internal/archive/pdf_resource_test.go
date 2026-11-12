package archive

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"
)

// noisePixels returns w*h RGB bytes of pseudo-random noise, so JPEG and Flate
// output is close to the size of a scanned page.
func noisePixels(w, h int, seed int64) []byte {
	pix := make([]byte, w*h*3)
	rand.New(rand.NewSource(seed)).Read(pix)
	return pix
}

func noiseJPEG(t *testing.T, w, h int, seed int64) pdfObj {
	t.Helper()
	pix := noisePixels(w, h, seed)
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		img.Pix[i*4+0], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = pix[i*3], pix[i*3+1], pix[i*3+2], 255
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return pdfObj{
		body: "/Type /XObject /Subtype /Image /Width " + strconv.Itoa(w) + " /Height " + strconv.Itoa(h) +
			" /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode",
		stream: buf.Bytes(),
	}
}

// heapInUse forces a collection and returns the bytes in in-use spans.
func heapInUse() uint64 {
	runtime.GC()
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapInuse
}

// TestPDFOpenMemory reports the heap held by an open multi-page PDF against
// its file size. It logs only; run with -v to see the numbers.
func TestPDFOpenMemory(t *testing.T) {
	// It builds PDFs of about 100 MB and takes seconds: run it on request.
	if os.Getenv("COMINAVI_MEMTEST") == "" {
		t.Skip("set COMINAVI_MEMTEST=1 to measure PDF memory")
	}
	cases := []struct {
		name  string
		pages func(t *testing.T) []pdfPageSpec
	}{
		{"jpeg-60p", func(t *testing.T) []pdfPageSpec {
			var ps []pdfPageSpec
			for i := 0; i < 60; i++ {
				ps = append(ps, pdfPageSpec{images: []pdfObj{noiseJPEG(t, 1200, 1700, int64(i))}})
			}
			return ps
		}},
		{"flate-30p", func(t *testing.T) []pdfPageSpec {
			var ps []pdfPageSpec
			for i := 0; i < 30; i++ {
				ps = append(ps, pdfPageSpec{images: []pdfObj{flateObj(800, 1200, "/DeviceRGB", noisePixels(800, 1200, int64(i)), nil)}})
			}
			return ps
		}},
	}
	for _, c := range cases {
		data := makePDF(c.pages(t))
		path := writePDFFile(t, data)
		before := heapInUse()
		start := time.Now()
		a, _, err := OpenPathContext(context.Background(), PathOf(path))
		if err != nil {
			t.Fatalf("%s: open: %v", c.name, err)
		}
		openDur := time.Since(start)
		during := heapInUse()
		t.Logf("%s: file %.1f MB, open %v, HeapInuse before %.1f MB, while open %.1f MB (+%.1f MB)",
			c.name, float64(len(data))/(1<<20), openDur, float64(before)/(1<<20), float64(during)/(1<<20),
			float64(int64(during)-int64(before))/(1<<20))
		runtime.KeepAlive(a)
		a.Close()
		a = nil
		after := heapInUse()
		t.Logf("%s: after Close HeapInuse %.1f MB", c.name, float64(after)/(1<<20))
	}
}

// TestPDFOpenContextWaitsForRender checks that a caller waiting for another
// page's render gives up when its context ends, instead of blocking.
func TestPDFOpenContextWaitsForRender(t *testing.T) {
	pix := noisePixels(16, 16, 1)
	data := makePDF([]pdfPageSpec{{images: []pdfObj{flateObj(16, 16, "/DeviceRGB", pix, nil)}}})
	a, err := openPDFFile(t, data)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	pa := a.(*pdfArchive)

	pa.busy <- struct{}{} // another render holds the archive
	defer pa.unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	rc, err := OpenContext(ctx, a, "0001.png")
	if rc != nil {
		rc.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("waited %v after the context ended", d)
	}
}

// TestPDFOpenContextCanceled checks that a canceled context stops a page
// render for both raw JPEG pages and decoded pages, and that a live context
// still serves them.
func TestPDFOpenContextCanceled(t *testing.T) {
	pix := noisePixels(16, 16, 2)
	data := makePDF([]pdfPageSpec{
		{images: []pdfObj{jpegObj(t, 16, 16)}},
		{images: []pdfObj{flateObj(16, 16, "/DeviceRGB", pix, nil)}},
	})
	a, err := openPDFFile(t, data)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	dead, cancel := context.WithCancel(context.Background())
	cancel()
	for _, name := range []string{"0001.jpg", "0002.png"} {
		if _, err := OpenContext(dead, a, name); !errors.Is(err, context.Canceled) {
			t.Fatalf("%s: err = %v, want context.Canceled", name, err)
		}
		rc, err := OpenContext(context.Background(), a, name)
		if err != nil {
			t.Fatalf("%s: live context: %v", name, err)
		}
		if b, _ := io.ReadAll(rc); len(b) == 0 {
			t.Fatalf("%s: empty entry", name)
		}
		rc.Close()
	}
}
