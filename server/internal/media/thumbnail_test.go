package media

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"math/rand"
	"testing"

	"github.com/gen2brain/avif"
	"github.com/gen2brain/webp"
)

// encodedAs returns a w x h gradient in the named format, for the decoders.
func encodedAs(t *testing.T, f Format, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	var err error
	switch f {
	case FormatWebP:
		err = webp.Encode(&buf, gradient(w, h), webp.Options{Quality: 90})
	case FormatAVIF:
		err = avif.Encode(&buf, gradient(w, h), avif.Options{Quality: 60, Speed: 10})
	default:
		return jpegBytes(t, w, h)
	}
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestThumbnailFormats(t *testing.T) {
	src := jpegBytes(t, 400, 600)
	for _, f := range []Format{FormatJPEG, FormatWebP, FormatAVIF} {
		t.Run(string(f), func(t *testing.T) {
			out, err := Thumbnail(src, 200, 300, f)
			if err != nil {
				t.Fatal(err)
			}
			cfg, name, err := image.DecodeConfig(bytes.NewReader(out))
			if err != nil {
				t.Fatalf("decode config: %v", err)
			}
			want := map[Format]string{FormatJPEG: "jpeg", FormatWebP: "webp", FormatAVIF: "avif"}[f]
			if name != want {
				t.Errorf("format = %q, want %q", name, want)
			}
			if cfg.Width > 200 || cfg.Height > 300 {
				t.Errorf("size %dx%d does not fit 200x300", cfg.Width, cfg.Height)
			}
			// 400x600 scaled to fit 200x300 keeps the 2:3 aspect ratio.
			if cfg.Width != 200 || cfg.Height != 300 {
				t.Errorf("size %dx%d, want 200x300", cfg.Width, cfg.Height)
			}
		})
	}
}

func TestThumbnailUnknownFormatIsJPEG(t *testing.T) {
	out, err := Thumbnail(jpegBytes(t, 40, 60), 200, 300, Format("bmp"))
	if err != nil {
		t.Fatal(err)
	}
	if _, name, err := image.DecodeConfig(bytes.NewReader(out)); err != nil || name != "jpeg" {
		t.Fatalf("format %q, err %v; want jpeg", name, err)
	}
}

func TestToJPEGConvertsWebPAndAVIF(t *testing.T) {
	for _, f := range []Format{FormatWebP, FormatAVIF} {
		t.Run(string(f), func(t *testing.T) {
			out, err := ToJPEG(encodedAs(t, f, 64, 96))
			if err != nil {
				t.Fatal(err)
			}
			cfg, name, err := image.DecodeConfig(bytes.NewReader(out))
			if err != nil {
				t.Fatal(err)
			}
			if name != "jpeg" {
				t.Errorf("format = %q, want jpeg", name)
			}
			if cfg.Width != 64 || cfg.Height != 96 {
				t.Errorf("size %dx%d, want 64x96 (no resize)", cfg.Width, cfg.Height)
			}
		})
	}
}

func TestToJPEGRejectsBadAndHugeInput(t *testing.T) {
	if _, err := ToJPEG([]byte("not an image")); err == nil {
		t.Error("garbage: no error")
	}
	huge := patchPNGSize(t, pngBytes(t, 4, 4), 20000, 20000)
	if _, err := ToJPEG(huge); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

// realisticPage is a w x h page: a gradient with noise on every pixel, so the
// encoders have real work to do.
func realisticPage(w, h int) *image.RGBA {
	rnd := rand.New(rand.NewSource(1))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := img.PixOffset(x, y)
			img.Pix[i] = uint8((x*255/w + rnd.Intn(32)) & 0xff)
			img.Pix[i+1] = uint8((y*255/h + rnd.Intn(32)) & 0xff)
			img.Pix[i+2] = uint8((x + y + rnd.Intn(32)) & 0xff)
			img.Pix[i+3] = 255
		}
	}
	return img
}

// BenchmarkAVIFPageThumbnail makes a cover thumbnail (the scanner's size and
// format) from one AVIF manga page. The page is encoded once, outside the
// timed loop.
func BenchmarkAVIFPageThumbnail(b *testing.B) {
	var buf bytes.Buffer
	if err := avif.Encode(&buf, realisticPage(1600, 2400), avif.Options{Quality: 60, Speed: 8}); err != nil {
		b.Fatal(err)
	}
	page := buf.Bytes()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Thumbnail(page, 400, 600, FormatJPEG); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkJPEGPageThumbnail is BenchmarkAVIFPageThumbnail for a JPEG page of
// the same size and quality.
func BenchmarkJPEGPageThumbnail(b *testing.B) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, realisticPage(1600, 2400), &jpeg.Options{Quality: 60}); err != nil {
		b.Fatal(err)
	}
	page := buf.Bytes()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Thumbnail(page, 400, 600, FormatJPEG); err != nil {
			b.Fatal(err)
		}
	}
}
