package media

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"math"

	xdraw "golang.org/x/image/draw"
)

// Thumbnail scales img to fit inside maxW x maxH, keeping the aspect ratio.
// Images that already fit are not enlarged. The result is a JPEG (quality
// 85). Transparent and palette images are composited onto white.
func Thumbnail(img []byte, maxW, maxH int) ([]byte, error) {
	if maxW <= 0 || maxH <= 0 {
		return nil, fmt.Errorf("media: thumbnail bounds %dx%d", maxW, maxH)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(img))
	if err != nil {
		return nil, fmt.Errorf("media: thumbnail: %w", err)
	}
	if int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return nil, ErrTooLarge
	}
	src, _, err := image.Decode(bytes.NewReader(img))
	if err != nil {
		return nil, fmt.Errorf("media: thumbnail: %w", err)
	}

	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if w > maxW || h > maxH {
		scale := math.Min(float64(maxW)/float64(w), float64(maxH)/float64(h))
		dw = min(maxW, max(1, int(math.Round(float64(w)*scale))))
		dh = min(maxH, max(1, int(math.Round(float64(h)*scale))))
	}

	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	xdraw.Draw(dst, dst.Bounds(), image.White, image.Point{}, xdraw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("media: thumbnail encode: %w", err)
	}
	return out.Bytes(), nil
}
