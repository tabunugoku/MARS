package media

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"testing"
)

func TestCheckPixels(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	cfg, err := CheckPixels(bytes.NewReader(buf.Bytes()), 12)
	if err != nil || cfg.Width != 4 || cfg.Height != 3 {
		t.Fatalf("at the limit: %+v %v", cfg, err)
	}
	if _, err := CheckPixels(bytes.NewReader(buf.Bytes()), 11); !errors.Is(err, ErrTooManyPixels) {
		t.Fatalf("over the limit: %v, want ErrTooManyPixels", err)
	}
	if _, err := CheckPixels(bytes.NewReader([]byte("not an image")), 12); err == nil || errors.Is(err, ErrTooManyPixels) {
		t.Fatalf("garbage: %v, want a decode error", err)
	}
}
