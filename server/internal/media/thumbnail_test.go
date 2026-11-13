package media

import (
	"bytes"
	"errors"
	"image"
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
