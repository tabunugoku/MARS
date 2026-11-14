package media

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/jpeg"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/tabunugoku/mars/server/internal/archive"
)

// zsNoiseJPEG returns a 1600x2400 JPEG of noise, about 0.5 MB, the size of a
// scanned page. The noise is kept to a narrow band so the file is realistic
// rather than full-entropy.
func zsNoiseJPEG(t testing.TB) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1600, 2400))
	rng := rand.New(rand.NewSource(1))
	for i := range img.Pix {
		img.Pix[i] = byte(200 + rng.Intn(40))
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type zsEntry struct {
	name string
	data []byte
}

// zsWriteZip writes a zip at dir/name. Entries are stored (no compression)
// when store is true, deflated otherwise.
func zsWriteZip(t testing.TB, dir, name string, store bool, entries []zsEntry) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		method := zip.Deflate
		if store {
			method = zip.Store
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e.name, Method: method})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// zsPages returns n page entries, all the same JPEG.
func zsPages(jpg []byte, n int) []zsEntry {
	out := make([]zsEntry, n)
	for i := range out {
		out[i] = zsEntry{name: fmt.Sprintf("p%03d.jpg", i+1), data: jpg}
	}
	return out
}

// zsBigEntry is a JPEG header followed by padding, so the entry is over
// MaxEntryBytes while its start is a valid image.
func zsBigEntry(jpg []byte) []byte {
	data := make([]byte, MaxEntryBytes+1)
	copy(data, jpg)
	return data
}

func TestAnalyzeZipDirectorySizeOverLimit(t *testing.T) {
	jpg := zsNoiseJPEG(t)
	big := zsBigEntry(jpg)
	dir := t.TempDir()

	t.Run("non-cover page", func(t *testing.T) {
		entries := append(zsPages(jpg, 2), zsEntry{name: "p003.jpg", data: big})
		path := zsWriteZip(t, dir, "nc.cbz", true, entries)
		_, err := AnalyzeCover(context.Background(), archive.PathOf(path), 1)
		if !errors.Is(err, ErrTooLarge) {
			t.Fatalf("err = %v, want ErrTooLarge", err)
		}
	})
	t.Run("cover page", func(t *testing.T) {
		entries := append([]zsEntry{{name: "p001.jpg", data: big}}, zsPages(jpg, 2)...)
		path := zsWriteZip(t, dir, "cover.cbz", true, entries)
		_, err := AnalyzeCover(context.Background(), archive.PathOf(path), 1)
		if !errors.Is(err, ErrTooLarge) {
			t.Fatalf("err = %v, want ErrTooLarge", err)
		}
	})
}

func TestAnalyzeZipNormalBookUnchanged(t *testing.T) {
	jpg := zsNoiseJPEG(t)
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(jpg))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	entries := zsPages(jpg, 5)
	entries = append(entries, zsEntry{name: "notes.txt", data: []byte("x")})
	for _, store := range []bool{true, false} {
		path := zsWriteZip(t, dir, fmt.Sprintf("b%v.cbz", store), store, entries)
		a, err := AnalyzeCover(context.Background(), archive.PathOf(path), 3)
		if err != nil {
			t.Fatalf("store=%v: %v", store, err)
		}
		if len(a.Pages) != 5 || a.CoverPage != 3 {
			t.Fatalf("store=%v: pages=%d cover=%d", store, len(a.Pages), a.CoverPage)
		}
		for i, p := range a.Pages {
			if p.Entry != entries[i].name || p.Number != i+1 || p.MediaType != "image/jpeg" {
				t.Errorf("store=%v: page %d = %+v", store, i+1, p)
			}
			if p.Width != cfg.Width || p.Height != cfg.Height {
				t.Errorf("store=%v: page %d size %dx%d, want %dx%d", store, i+1, p.Width, p.Height, cfg.Width, cfg.Height)
			}
		}
		if !bytes.Equal(a.Cover, jpg) {
			t.Errorf("store=%v: cover bytes differ from the entry", store)
		}
	}
}

// zsWriteStoredBadCRC writes a stored zip in which the entries named in bad
// carry a CRC-32 that does not match their bytes. The bytes are intact, so a
// reader that reaches the end of such an entry fails with zip.ErrChecksum and
// one that stops after the header does not.
func zsWriteStoredBadCRC(t testing.TB, dir, name string, entries []zsEntry, bad map[string]bool) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		fh := &zip.FileHeader{
			Name:               e.name,
			Method:             zip.Store,
			CRC32:              crc32.ChecksumIEEE(e.data),
			CompressedSize64:   uint64(len(e.data)),
			UncompressedSize64: uint64(len(e.data)),
		}
		if bad[e.name] {
			fh.CRC32 ^= 0xdeadbeef
		}
		w, err := zw.CreateRaw(fh)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// Non-cover pages are read only as far as their header, so a stored book
// whose non-cover pages would fail a full read still analyses: the bodies are
// never reached. The cover is read to its end, so a bad cover fails.
func TestAnalyzeZipReadsOnlyHeaders(t *testing.T) {
	jpg := zsNoiseJPEG(t)
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(jpg))
	if err != nil {
		t.Fatal(err)
	}
	const n = 100
	entries := zsPages(jpg, n)
	dir := t.TempDir()

	t.Run("non-cover bodies not read", func(t *testing.T) {
		bad := make(map[string]bool)
		for _, e := range entries[1:] {
			bad[e.name] = true
		}
		path := zsWriteStoredBadCRC(t, dir, "headers.cbz", entries, bad)
		a, err := AnalyzeCover(context.Background(), archive.PathOf(path), 1)
		if err != nil {
			t.Fatalf("analysis failed: %v (a non-cover body was read to its end)", err)
		}
		if len(a.Pages) != n {
			t.Fatalf("pages = %d, want %d", len(a.Pages), n)
		}
		for i, p := range a.Pages {
			if p.Width != cfg.Width || p.Height != cfg.Height {
				t.Errorf("page %d size %dx%d, want %dx%d", i+1, p.Width, p.Height, cfg.Width, cfg.Height)
			}
		}
	})
	t.Run("cover body read", func(t *testing.T) {
		path := zsWriteStoredBadCRC(t, dir, "badcover.cbz", entries, map[string]bool{entries[0].name: true})
		if _, err := AnalyzeCover(context.Background(), archive.PathOf(path), 1); err == nil {
			t.Fatal("analysis of a book with a bad cover body succeeded; the cover must be read to its end")
		} else {
			t.Logf("bad cover: %v", err)
		}
	})
}

// BenchmarkAnalyzeStoredCBZ analyses a stored CBZ of 100 noise JPEG pages of
// 1600x2400.
func BenchmarkAnalyzeStoredCBZ(b *testing.B) {
	jpg := zsNoiseJPEG(b)
	dir := b.TempDir()
	path := zsWriteZip(b, dir, "stored.cbz", true, zsPages(jpg, 100))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := AnalyzeCover(context.Background(), archive.PathOf(path), 1); err != nil {
			b.Fatal(err)
		}
	}
}
