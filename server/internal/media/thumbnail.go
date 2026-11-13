package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"math"

	"github.com/gen2brain/avif"
	"github.com/gen2brain/webp"
	xdraw "golang.org/x/image/draw"
)

// Thumbnail scales img to fit inside maxW x maxH, keeping the aspect ratio.
// Images that already fit are not enlarged. The result is encoded in f:
// JPEG (quality 85), WebP (quality 80) or AVIF (quality 60). Transparent and
// palette images are composited onto white.
func Thumbnail(img []byte, maxW, maxH int, f Format) ([]byte, error) {
	if maxW <= 0 || maxH <= 0 {
		return nil, fmt.Errorf("media: thumbnail bounds %dx%d", maxW, maxH)
	}
	src, err := decodeChecked(img)
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

	out, err := encode(dst, f)
	if err != nil {
		return nil, fmt.Errorf("media: thumbnail encode: %w", err)
	}
	return out, nil
}

// ToJPEG decodes img (JPEG, PNG, GIF, WebP or AVIF) and encodes it as a JPEG
// (quality 85) at its own size. Transparent and palette images are
// composited onto white. The pixel limit is checked before any decode.
func ToJPEG(img []byte) ([]byte, error) {
	src, err := decodeChecked(img)
	if err != nil {
		return nil, fmt.Errorf("media: to jpeg: %w", err)
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	xdraw.Draw(dst, dst.Bounds(), image.White, image.Point{}, xdraw.Src)
	xdraw.Draw(dst, dst.Bounds(), src, b.Min, xdraw.Over)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("media: to jpeg encode: %w", err)
	}
	return out.Bytes(), nil
}

// decodeChecked decodes img after the pixel guard. An image over MaxPixels is
// ErrTooLarge.
func decodeChecked(img []byte) (image.Image, error) {
	if _, err := CheckPixels(bytes.NewReader(img), MaxPixels); errors.Is(err, ErrTooManyPixels) {
		return nil, ErrTooLarge
	} else if err != nil {
		return nil, err
	}
	src, _, err := image.Decode(bytes.NewReader(img))
	return src, err
}

// encode writes m in format f. AVIF uses speed 8 (of 0 to 10), the chosen
// trade-off: about 0.4 s per 400x600 image measured. Slower speeds give
// smaller files at a higher encode cost.
func encode(m image.Image, f Format) ([]byte, error) {
	var out bytes.Buffer
	var err error
	switch f {
	case FormatWebP:
		err = webp.Encode(&out, m, webp.Options{Quality: 80, Method: 4})
	case FormatAVIF:
		err = avif.Encode(&out, m, avif.Options{Quality: 60, Speed: 8})
	default:
		err = jpeg.Encode(&out, m, &jpeg.Options{Quality: 85})
	}
	if err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
