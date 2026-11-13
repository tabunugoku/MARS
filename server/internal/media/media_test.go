package media

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tabunugoku/mars/server/internal/archive"
)

type zipEntry struct {
	name string
	data []byte
}

// gradient returns a w x h test image with some structure in every pixel.
func gradient(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), uint8(x + y), 255})
		}
	}
	return img
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, gradient(w, h)); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, gradient(w, h), &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// writeZip writes entries in the given order, so tests control archive order.
func writeZip(t *testing.T, path string, entries []zipEntry) string {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		w, err := zw.Create(e.name)
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

// sampleBook is a zip with pages that sort as p1.jpg, p2.png, p10.png, plus
// entries that must be ignored.
func sampleBook(t *testing.T, dir string) (path string, entries []zipEntry) {
	t.Helper()
	entries = []zipEntry{
		{"p10.png", pngBytes(t, 20, 20)},
		{"__MACOSX/p1.jpg", []byte("not an image")},
		{".hidden.png", []byte("not an image")},
		{"p2.png", pngBytes(t, 30, 40)},
		{"ComicInfo.xml", []byte("<ComicInfo><Title>Sample</Title></ComicInfo>")},
		{"notes.txt", []byte("hello")},
		{"p1.jpg", jpegBytes(t, 400, 600)},
	}
	return writeZip(t, filepath.Join(dir, "book.cbz"), entries), entries
}

func entryData(entries []zipEntry, name string) []byte {
	for _, e := range entries {
		if e.name == name {
			return e.data
		}
	}
	return nil
}

func TestAnalyzeOrderAndMetadata(t *testing.T) {
	path, entries := sampleBook(t, t.TempDir())

	got, err := Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if got.Format != archive.CBZ {
		t.Errorf("Format = %q, want cbz", got.Format)
	}
	want := []Page{
		{Number: 1, Entry: "p1.jpg", MediaType: "image/jpeg", Width: 400, Height: 600},
		{Number: 2, Entry: "p2.png", MediaType: "image/png", Width: 30, Height: 40},
		{Number: 3, Entry: "p10.png", MediaType: "image/png", Width: 20, Height: 20},
	}
	if len(got.Pages) != len(want) {
		t.Fatalf("got %d pages, want %d: %+v", len(got.Pages), len(want), got.Pages)
	}
	for i := range want {
		if got.Pages[i] != want[i] {
			t.Errorf("page %d = %+v, want %+v", i+1, got.Pages[i], want[i])
		}
	}
	if string(got.ComicInfo) != string(entryData(entries, "ComicInfo.xml")) {
		t.Errorf("ComicInfo = %q", got.ComicInfo)
	}
	if !bytes.Equal(got.Cover, entryData(entries, "p1.jpg")) {
		t.Errorf("Cover does not equal p1.jpg bytes (len %d)", len(got.Cover))
	}
}

func TestAnalyzeNoPages(t *testing.T) {
	path := writeZip(t, filepath.Join(t.TempDir(), "text.cbz"), []zipEntry{
		{"notes.txt", []byte("hello")},
		{"ComicInfo.xml", []byte("<ComicInfo/>")},
	})
	if _, err := Analyze(path); !errors.Is(err, ErrNoPages) {
		t.Fatalf("err = %v, want ErrNoPages", err)
	}
}

func TestAnalyzeUnsupported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Analyze(path); !errors.Is(err, archive.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

func TestAnalyzeCorruptPage(t *testing.T) {
	path := writeZip(t, filepath.Join(t.TempDir(), "bad.cbz"), []zipEntry{
		{"p1.png", pngBytes(t, 10, 10)},
		{"p2.png", []byte("garbage, not a png")},
	})
	_, err := Analyze(path)
	if err == nil || !strings.Contains(err.Error(), "page 2") {
		t.Fatalf("err = %v, want an error naming page 2", err)
	}
}

// patchPNGSize rewrites the IHDR width and height and fixes the IHDR CRC.
func patchPNGSize(t *testing.T, data []byte, w, h uint32) []byte {
	t.Helper()
	out := append([]byte(nil), data...)
	// signature (8) + length (4) + "IHDR" (4): data starts at offset 16.
	binary.BigEndian.PutUint32(out[16:20], w)
	binary.BigEndian.PutUint32(out[20:24], h)
	// CRC covers the chunk type and data: offsets 12..29, CRC at 29..33.
	binary.BigEndian.PutUint32(out[29:33], crc32.ChecksumIEEE(out[12:29]))
	return out
}

func TestAnalyzeTooLarge(t *testing.T) {
	huge := patchPNGSize(t, pngBytes(t, 4, 4), 20000, 20000)
	path := writeZip(t, filepath.Join(t.TempDir(), "huge.cbz"), []zipEntry{
		{"p1.png", huge},
	})
	if _, err := Analyze(path); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestAnalyzeComicInfoRules(t *testing.T) {
	dir := t.TempDir()
	png := pngBytes(t, 8, 8)

	t.Run("root preferred over nested", func(t *testing.T) {
		path := writeZip(t, filepath.Join(dir, "nested.cbz"), []zipEntry{
			{"extras/comicinfo.xml", []byte("nested")},
			{"p1.png", png},
			{"COMICINFO.XML", []byte("root")},
		})
		got, err := Analyze(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got.ComicInfo) != "root" {
			t.Errorf("ComicInfo = %q, want root copy", got.ComicInfo)
		}
	})

	t.Run("oversized treated as absent", func(t *testing.T) {
		big := make([]byte, MaxComicInfo+1)
		path := writeZip(t, filepath.Join(dir, "big.cbz"), []zipEntry{
			{"ComicInfo.xml", big},
			{"p1.png", png},
		})
		got, err := Analyze(path)
		if err != nil {
			t.Fatalf("Analyze: %v", err)
		}
		if got.ComicInfo != nil {
			t.Errorf("ComicInfo len %d, want nil", len(got.ComicInfo))
		}
	})
}

func TestThumbnailScalesAndKeepsSmallImages(t *testing.T) {
	big := jpegBytes(t, 400, 600)
	out, err := Thumbnail(big, 200, 300, FormatJPEG)
	if err != nil {
		t.Fatal(err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if format != "jpeg" || cfg.Width != 200 || cfg.Height != 300 {
		t.Errorf("thumbnail = %s %dx%d, want jpeg 200x300", format, cfg.Width, cfg.Height)
	}

	small := pngBytes(t, 30, 40)
	out, err = Thumbnail(small, 200, 300, FormatJPEG)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err = image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 30 || cfg.Height != 40 {
		t.Errorf("small thumbnail = %dx%d, want 30x40 (no upscale)", cfg.Width, cfg.Height)
	}
}

func TestThumbnailTooLarge(t *testing.T) {
	huge := patchPNGSize(t, pngBytes(t, 4, 4), 20000, 20000)
	if _, err := Thumbnail(huge, 200, 300, FormatJPEG); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestAnalyzeAVIFOnlyBook(t *testing.T) {
	dir := t.TempDir()
	cover := encodedAs(t, FormatAVIF, 64, 96)
	path := writeZip(t, filepath.Join(dir, "avif.cbz"), []zipEntry{
		{"002.avif", encodedAs(t, FormatAVIF, 32, 48)},
		{"001.avif", cover},
	})

	got, err := Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	want := []Page{
		{Number: 1, Entry: "001.avif", MediaType: "image/avif", Width: 64, Height: 96},
		{Number: 2, Entry: "002.avif", MediaType: "image/avif", Width: 32, Height: 48},
	}
	if len(got.Pages) != len(want) {
		t.Fatalf("got %d pages, want %d: %+v", len(got.Pages), len(want), got.Pages)
	}
	for i := range want {
		if got.Pages[i] != want[i] {
			t.Errorf("page %d = %+v, want %+v", i+1, got.Pages[i], want[i])
		}
	}
	if !bytes.Equal(got.Cover, cover) {
		t.Errorf("Cover does not equal 001.avif bytes (len %d)", len(got.Cover))
	}
	thumb, err := Thumbnail(got.Cover, 32, 48, FormatJPEG)
	if err != nil {
		t.Fatalf("cover thumbnail: %v", err)
	}
	if cfg, name, err := image.DecodeConfig(bytes.NewReader(thumb)); err != nil || name != "jpeg" || cfg.Width != 32 || cfg.Height != 48 {
		t.Errorf("cover thumbnail = %v %q %v, want 32x48 jpeg", cfg, name, err)
	}
}

func TestAnalyzeMixedAVIFAndJPEGKeepsNaturalOrder(t *testing.T) {
	dir := t.TempDir()
	path := writeZip(t, filepath.Join(dir, "mixed.cbz"), []zipEntry{
		{"p10.avif", encodedAs(t, FormatAVIF, 20, 20)},
		{"p2.jpg", jpegBytes(t, 30, 40)},
		{"p1.avif", encodedAs(t, FormatAVIF, 64, 96)},
	})

	got, err := Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	want := []Page{
		{Number: 1, Entry: "p1.avif", MediaType: "image/avif", Width: 64, Height: 96},
		{Number: 2, Entry: "p2.jpg", MediaType: "image/jpeg", Width: 30, Height: 40},
		{Number: 3, Entry: "p10.avif", MediaType: "image/avif", Width: 20, Height: 20},
	}
	if len(got.Pages) != len(want) {
		t.Fatalf("got %d pages, want %d: %+v", len(got.Pages), len(want), got.Pages)
	}
	for i := range want {
		if got.Pages[i] != want[i] {
			t.Errorf("page %d = %+v, want %+v", i+1, got.Pages[i], want[i])
		}
	}
}
