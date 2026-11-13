//go:build perf

// Peak memory of concurrent book analysis under the decode budget. Not part of
// the normal test run: build with -tags perf.
//
//	go test -tags perf -run TestPerfDecodeBudget -v -timeout 10m ./internal/media
//
// It writes 40 CBZ books of 8 realistic 1600x2400 pages (half JPEG, half AVIF),
// then analyses them all with 6 goroutines through the scanner's own steps
// (AnalyzeCover, then the cover Thumbnail). It samples the resident Go memory
// every 20 ms and reports the peak, once with the budget effectively unlimited and
// once at the 256 MiB default. Both runs use a soft memory limit of 768 MiB,
// the container limit the test stands in for.
package media

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime/debug"
	"runtime/metrics"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gen2brain/avif"
	"github.com/tabunugoku/mars/server/internal/archive"
)

const (
	perfBooks      = 40
	perfPages      = 8
	perfWorkers    = 6
	perfSoftLimit  = 768 << 20
	perfSampleStep = 20 * time.Millisecond
)

// writePerfBook writes one CBZ of perfPages copies of page (named in order).
func writePerfBook(t *testing.T, path, ext string, page []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for i := 1; i <= perfPages; i++ {
		w, err := zw.Create(fmt.Sprintf("%03d.%s", i, ext))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(page); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// residentBytes is the memory the runtime holds in RAM: everything it has
// mapped (total) less what it returned to the OS (released). Released pages
// stay in total, so total alone would carry one run's peak into the next.
func residentBytes() uint64 {
	s := []metrics.Sample{
		{Name: "/memory/classes/total:bytes"},
		{Name: "/memory/classes/heap/released:bytes"},
	}
	metrics.Read(s)
	return s[0].Value.Uint64() - s[1].Value.Uint64()
}

// runPerfAnalysis analyses every book with perfWorkers goroutines and returns
// the peak resident Go memory seen and the wall time.
func runPerfAnalysis(t *testing.T, books []string) (uint64, time.Duration) {
	t.Helper()
	var peak atomic.Uint64
	stop := make(chan struct{})
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		tick := time.NewTicker(perfSampleStep)
		defer tick.Stop()
		for {
			if v := residentBytes(); v > peak.Load() {
				peak.Store(v)
			}
			select {
			case <-stop:
				return
			case <-tick.C:
			}
		}
	}()

	jobs := make(chan string)
	var wg sync.WaitGroup
	var failed atomic.Int64
	start := time.Now()
	for w := 0; w < perfWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				a, err := AnalyzeCover(context.Background(), archive.PathOf(path), 1)
				if err != nil {
					failed.Add(1)
					continue
				}
				if _, err := Thumbnail(a.Cover, 400, 600, FormatJPEG); err != nil {
					failed.Add(1)
				}
			}
		}()
	}
	for _, p := range books {
		jobs <- p
	}
	close(jobs)
	wg.Wait()
	wall := time.Since(start)
	close(stop)
	<-sampled
	if n := failed.Load(); n != 0 {
		t.Fatalf("%d books failed to analyse", n)
	}
	return peak.Load(), wall
}

func TestPerfDecodeBudget(t *testing.T) {
	dir := t.TempDir()

	// One realistic page per format, encoded once and shared by the books.
	var jpgBuf, avifBuf bytes.Buffer
	page := realisticPage(1600, 2400)
	if err := jpeg.Encode(&jpgBuf, page, &jpeg.Options{Quality: 60}); err != nil {
		t.Fatal(err)
	}
	if err := avif.Encode(&avifBuf, page, avif.Options{Quality: 60, Speed: 8}); err != nil {
		t.Fatal(err)
	}

	books := make([]string, perfBooks)
	for i := range books {
		books[i] = filepath.Join(dir, fmt.Sprintf("book%02d.cbz", i))
		if i%2 == 0 {
			writePerfBook(t, books[i], "jpg", jpgBuf.Bytes())
		} else {
			writePerfBook(t, books[i], "avif", avifBuf.Bytes())
		}
	}

	oldLimit := debug.SetMemoryLimit(perfSoftLimit)
	defer debug.SetMemoryLimit(oldLimit)
	defer SetDecodeBudget(DefaultDecodeBudget)

	for _, run := range []struct {
		name   string
		budget int64
	}{
		{"unlimited budget", 1 << 40},
		{"256 MiB budget", DefaultDecodeBudget},
	} {
		SetDecodeBudget(run.budget)
		decodeHoldersPeak.Store(0)
		debug.FreeOSMemory()
		peak, wall := runPerfAnalysis(t, books)
		t.Logf("%-16s peak resident %6.1f MiB, wall %6.2f s, peak concurrent decodes %d",
			run.name, float64(peak)/(1<<20), wall.Seconds(), decodeHoldersPeak.Load())
	}
}
