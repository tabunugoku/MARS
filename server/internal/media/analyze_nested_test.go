package media

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/tabunugoku/mars/server/internal/archive"
)

// nestedPNG is a small PNG filled with one grey level, so each page differs.
func nestedPNG(t *testing.T, grey uint8) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, 8, 12))
	for i := range img.Pix {
		img.Pix[i] = grey
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func nestedZip(t *testing.T, entries map[string][]byte, order []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range order {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(entries[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestAnalyzeAndExtractNestedZip reads a book whose volumes are zips inside
// the book zip: the pages are the volumes' images in volume order, and the
// page cache serves each page's own bytes.
func TestAnalyzeAndExtractNestedZip(t *testing.T) {
	archive.SetTempDir(t.TempDir())
	defer archive.SetTempDir("")
	pages := map[string][]byte{}
	vol := func(name string, greys ...uint8) []byte {
		m, order := map[string][]byte{}, []string{}
		for i, g := range greys {
			n := string(rune('a'+i)) + ".png"
			m[n] = nestedPNG(t, g)
			pages[name+"/"+n] = m[n]
			order = append(order, n)
		}
		return nestedZip(t, m, order)
	}
	// vol10 before vol2 in the file: natural order must still put vol2 first.
	book := nestedZip(t, map[string][]byte{
		"vol10.zip": vol("vol10.zip", 30, 40),
		"vol2.zip":  vol("vol2.zip", 10, 20),
	}, []string{"vol10.zip", "vol2.zip"})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "book.cbz"), book, 0o644); err != nil {
		t.Fatal(err)
	}
	path := archive.Path{Root: dir, Rel: "book.cbz"}

	an, err := AnalyzeContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"vol2.zip/a.png", "vol2.zip/b.png", "vol10.zip/a.png", "vol10.zip/b.png"}
	if len(an.Pages) != len(want) {
		t.Fatalf("pages = %d, want %d", len(an.Pages), len(want))
	}
	for i, p := range an.Pages {
		if p.Entry != want[i] {
			t.Errorf("page %d = %q, want %q", i+1, p.Entry, want[i])
		}
	}
	if len(an.Cover) == 0 {
		t.Error("no cover bytes")
	} else if c, err := png.Decode(bytes.NewReader(an.Cover)); err != nil || c.At(0, 0) != (color.Gray{Y: 10}) {
		t.Errorf("cover is not the first page of vol2 (err %v)", err)
	}

	cache := &PageCache{Dir: t.TempDir(), MaxBytes: 1 << 30}
	for i, p := range an.Pages {
		f, err := cache.Open(context.Background(), "b1", "k1", path, an.Pages, i+1)
		if err != nil {
			t.Fatalf("page %d: %v", i+1, err)
		}
		got, _ := io.ReadAll(f)
		f.Close()
		if !bytes.Equal(got, pages[p.Entry]) {
			t.Errorf("page %d bytes differ from %s", i+1, p.Entry)
		}
	}
}
