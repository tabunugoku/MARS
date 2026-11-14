package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"

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
	src, release, err := decodeChecked(context.Background(), img)
	if err != nil {
		return nil, fmt.Errorf("media: thumbnail: %w", err)
	}
	defer release()

	b := src.Bounds()
	dw, dh := thumbSize(b.Dx(), b.Dy(), maxW, maxH)
	dst := scaleThumb(src, dw, dh)

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
	src, release, err := decodeChecked(context.Background(), img)
	if err != nil {
		return nil, fmt.Errorf("media: to jpeg: %w", err)
	}
	defer release()
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

// decodeChecked decodes img after the pixel guard, under the decode budget. An
// image over MaxPixels is ErrTooLarge. The caller calls release once the image
// is no longer used.
func decodeChecked(ctx context.Context, img []byte) (image.Image, func(), error) {
	if _, err := CheckPixels(bytes.NewReader(img), MaxPixels); errors.Is(err, ErrTooManyPixels) {
		return nil, nil, ErrTooLarge
	} else if err != nil {
		return nil, nil, err
	}
	return DecodeBudgeted(ctx, img)
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
