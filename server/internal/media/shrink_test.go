package media

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"testing"

	xdraw "golang.org/x/image/draw"
)

// oldScale is the single-step thumbnail that Thumbnail used before the box
// shrink: CatmullRom straight from full size over white. Tests compare the
// two-step result against it.
func oldScale(src image.Image, dw, dh int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	xdraw.Draw(dst, dst.Bounds(), image.White, image.Point{}, xdraw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	return dst
}

// psnrRGB is the peak signal-to-noise ratio of the RGB channels of a and b,
// both the same size. Alpha is ignored.
func psnrRGB(a, b *image.RGBA) float64 {
	var se float64
	n := 0
	for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
		for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
			i, j := a.PixOffset(x, y), b.PixOffset(x, y)
			for c := 0; c < 3; c++ {
				d := float64(a.Pix[i+c]) - float64(b.Pix[j+c])
				se += d * d
				n++
			}
		}
	}
	mse := se / float64(n)
	if mse == 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10(255*255/mse)
}

func TestBoxShrinkRGBAExactAverages(t *testing.T) {
	// R(x,y) = x + 4y. Blocks of 2x2 average to 3, 5, 11, 13 with rounding
	// half up: (sum+2)/4 for sums 10, 18, 42, 50.
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			i := src.PixOffset(x, y)
			src.Pix[i], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = uint8(x+4*y), 0, 8, 255
		}
	}
	got, ok := boxShrink(src, 2)
	if !ok {
		t.Fatal("RGBA not supported")
	}
	out := got.(*image.RGBA)
	if out.Rect != image.Rect(0, 0, 2, 2) {
		t.Fatalf("bounds %v, want 2x2", out.Rect)
	}
	want := []uint8{3, 5, 11, 13}
	for i, wr := range want {
		p := out.Pix[i*4 : i*4+4]
		if p[0] != wr || p[2] != 8 || p[3] != 255 {
			t.Errorf("pixel %d = %v, want R %d B 8 A 255", i, p, wr)
		}
	}
}

func TestBoxShrinkRGBAEdgeIsPartialAverage(t *testing.T) {
	// 5x3 with k=2 gives 3x2. R(x,y) = 10x + y. The right column holds only
	// x=4, so its block averages over the pixels that exist.
	src := image.NewRGBA(image.Rect(0, 0, 5, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 5; x++ {
			src.Pix[src.PixOffset(x, y)] = uint8(10*x + y)
			src.Pix[src.PixOffset(x, y)+3] = 255
		}
	}
	got, _ := boxShrink(src, 2)
	out := got.(*image.RGBA)
	if out.Rect != image.Rect(0, 0, 3, 2) {
		t.Fatalf("bounds %v, want 3x2", out.Rect)
	}
	// (0,0): x 0,1 y 0,1 -> 0,1,10,11 sum 22 n 4 -> 6 (5.5 rounds up).
	// (2,0): x 4 y 0,1 -> 40,41 sum 81 n 2 -> 41.
	// (0,1): x 0,1 y 2 -> 2,12 sum 14 n 2 -> 7.
	// (2,1): x 4 y 2 -> 42 n 1 -> 42.
	for _, c := range []struct {
		x, y int
		want uint8
	}{{0, 0, 6}, {2, 0, 41}, {0, 1, 7}, {2, 1, 42}} {
		if got := out.Pix[out.PixOffset(c.x, c.y)]; got != c.want {
			t.Errorf("(%d,%d) R = %d, want %d", c.x, c.y, got, c.want)
		}
	}
}

func TestBoxShrinkNRGBAWeightsByAlpha(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	// Two opaque pixels of red 200 and 100, two fully transparent pixels whose
	// colour must not count.
	src.Pix[0], src.Pix[3] = 200, 255
	src.Pix[src.PixOffset(0, 1)], src.Pix[src.PixOffset(0, 1)+3] = 100, 255
	src.Pix[src.PixOffset(1, 0)+0] = 255 // transparent, colour ignored
	src.Pix[src.PixOffset(1, 1)+1] = 255 // transparent, colour ignored
	got, ok := boxShrink(src, 2)
	if !ok {
		t.Fatal("NRGBA not supported")
	}
	out := got.(*image.NRGBA)
	// sum(c*a)/sum(a) = (200*255 + 100*255) / 510 = 150. Alpha 510/4 -> 128.
	want := color.NRGBA{R: 150, G: 0, B: 0, A: 128}
	if c := out.NRGBAAt(0, 0); c != want {
		t.Errorf("colour = %v, want %v", c, want)
	}
}

func TestBoxShrinkNRGBAAllTransparent(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	src.Pix[0] = 255 // colour with alpha 0
	got, _ := boxShrink(src, 2)
	if c := got.(*image.NRGBA).NRGBAAt(0, 0); c != (color.NRGBA{}) {
		t.Errorf("colour = %v, want zero", c)
	}
}

func TestBoxShrinkGrayExactAverages(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			src.Pix[src.PixOffset(x, y)] = uint8(x + 4*y)
		}
	}
	got, ok := boxShrink(src, 2)
	if !ok {
		t.Fatal("Gray not supported")
	}
	out := got.(*image.Gray)
	want := []uint8{3, 5, 11, 13}
	for i, wv := range want {
		if out.Pix[i] != wv {
			t.Errorf("pixel %d = %d, want %d", i, out.Pix[i], wv)
		}
	}
}

func TestBoxShrinkYCbCr420ChromaMapping(t *testing.T) {
	// 4x4 4:2:0: luma is flat 128, chroma is 2x2 with distinct values. A 2x2
	// block covers exactly one chroma sample, so each output pixel must be
	// YCbCr(128, cb, cr) of that sample.
	src := image.NewYCbCr(image.Rect(0, 0, 4, 4), image.YCbCrSubsampleRatio420)
	for i := range src.Y {
		src.Y[i] = 128
	}
	cb := [][]uint8{{0, 40}, {80, 120}}
	for cy := 0; cy < 2; cy++ {
		for cx := 0; cx < 2; cx++ {
			src.Cb[cy*src.CStride+cx] = cb[cy][cx]
			src.Cr[cy*src.CStride+cx] = 128
		}
	}
	got, ok := boxShrink(src, 2)
	if !ok {
		t.Fatal("YCbCr not supported")
	}
	out := got.(*image.RGBA)
	for oy := 0; oy < 2; oy++ {
		for ox := 0; ox < 2; ox++ {
			r, g, b := color.YCbCrToRGB(128, cb[oy][ox], 128)
			i := out.PixOffset(ox, oy)
			if out.Pix[i] != r || out.Pix[i+1] != g || out.Pix[i+2] != b || out.Pix[i+3] != 255 {
				t.Errorf("(%d,%d) = %v, want %d %d %d 255", ox, oy, out.Pix[i:i+4], r, g, b)
			}
		}
	}
}

func TestBoxShrinkYCbCr444Luma(t *testing.T) {
	// 4:4:4, luma x+4y, neutral chroma: the output is grey with the averaged
	// luma 3, 5, 11, 13 (the same values as the RGBA test).
	src := image.NewYCbCr(image.Rect(0, 0, 4, 4), image.YCbCrSubsampleRatio444)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			src.Y[src.YOffset(x, y)] = uint8(x + 4*y)
			src.Cb[src.COffset(x, y)] = 128
			src.Cr[src.COffset(x, y)] = 128
		}
	}
	got, _ := boxShrink(src, 2)
	out := got.(*image.RGBA)
	for i, wy := range []uint8{3, 5, 11, 13} {
		r, g, b := color.YCbCrToRGB(wy, 128, 128)
		p := out.Pix[i*4 : i*4+4]
		if p[0] != r || p[1] != g || p[2] != b {
			t.Errorf("pixel %d = %v, want %d %d %d", i, p, r, g, b)
		}
	}
}

func TestBoxShrinkUnsupportedTypeSkipped(t *testing.T) {
	src := image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.Black, color.White})
	if _, ok := boxShrink(src, 2); ok {
		t.Error("paletted image: boxShrink ok, want skip")
	}
}

func TestShrinkFactor(t *testing.T) {
	cases := []struct {
		w, h, dw, dh, want int
	}{
		{1600, 2400, 400, 600, 2}, // ratio 4 -> k 2
		{1600, 2400, 200, 300, 4}, // ratio 8 -> k 4
		{600, 900, 200, 300, 0},   // ratio 3 -> k 1, no shrink
		{300, 450, 200, 300, 0},   // ratio 1.5, small reduction
		{1600, 2400, 400, 100, 2}, // ratios 4 and 24 -> min 4 -> k 2
	}
	for _, c := range cases {
		if got := shrinkFactor(c.w, c.h, c.dw, c.dh); got != c.want {
			t.Errorf("shrinkFactor(%d,%d,%d,%d) = %d, want %d", c.w, c.h, c.dw, c.dh, got, c.want)
		}
	}
}

// TestThumbnailTwoStepMatchesSingleStep: a realistic 1600x2400 page, from RGBA
// and from a JPEG decode (YCbCr), downscaled to 400x600 by the two-step path
// and by the old single CatmullRom pass, must agree to at least 35 dB PSNR.
func TestThumbnailTwoStepMatchesSingleStep(t *testing.T) {
	page := realisticPage(1600, 2400)
	var jbuf bytes.Buffer
	if err := jpeg.Encode(&jbuf, page, &jpeg.Options{Quality: 60}); err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(jbuf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		src  image.Image
	}{{"rgba", page}, {"jpeg-ycbcr", decoded}} {
		t.Run(c.name, func(t *testing.T) {
			dw, dh := thumbSize(1600, 2400, 400, 600)
			if shrinkFactor(1600, 2400, dw, dh) < 2 {
				t.Fatal("test page does not take the two-step path")
			}
			got := scaleThumb(c.src, dw, dh)
			want := oldScale(c.src, dw, dh)
			if p := psnrRGB(got, want); p < 35 {
				t.Errorf("PSNR = %.2f dB, want >= 35", p)
			} else {
				t.Logf("PSNR = %.2f dB", p)
			}
		})
	}
}

// TestThumbnailOutputMatchesOldJPEG checks the encoded Thumbnail output
// against the encoded old single-step result, both decoded: the JPEG noise is
// the same on both sides, so the PSNR is high.
func TestThumbnailOutputMatchesOldJPEG(t *testing.T) {
	page := realisticPage(1600, 2400)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, page, &jpeg.Options{Quality: 60}); err != nil {
		t.Fatal(err)
	}
	out, err := Thumbnail(buf.Bytes(), 400, 600, FormatJPEG)
	if err != nil {
		t.Fatal(err)
	}
	gotImg, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	var old bytes.Buffer
	if err := jpeg.Encode(&old, oldScale(page, 400, 600), &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	wantImg, err := jpeg.Decode(bytes.NewReader(old.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	got := toRGBA(gotImg)
	want := toRGBA(wantImg)
	if p := psnrRGB(got, want); p < 35 {
		t.Errorf("encoded PSNR = %.2f dB, want >= 35", p)
	}
}

func toRGBA(img image.Image) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	xdraw.Draw(out, out.Bounds(), img, b.Min, xdraw.Src)
	return out
}

// A transparent PNG over 2x the target takes the two-step path and must still
// composite onto white.
func TestThumbnailTransparentCompositesOnWhite(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 80, 120)) // all alpha 0
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i] = 0 // black, but invisible
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	out, err := Thumbnail(buf.Bytes(), 20, 30, FormatJPEG)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(10, 15).RGBA()
	for _, v := range []uint32{r >> 8, g >> 8, b >> 8} {
		if v < 245 {
			t.Errorf("transparent centre = %d %d %d, want white", r>>8, g>>8, b>>8)
			break
		}
	}
}

// A half-transparent black image: the composite over white is mid grey, the
// same as the single-step path gives.
func TestThumbnailHalfAlphaCompositesLikeOldPath(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 80, 120))
	for i := 3; i < len(src.Pix); i += 4 {
		src.Pix[i] = 128
	}
	got := scaleThumb(src, 20, 30)
	want := oldScale(src, 20, 30)
	if p := psnrRGB(got, want); p < 45 {
		t.Errorf("PSNR vs old path = %.2f dB, want a close match", p)
	}
	if v := got.Pix[got.PixOffset(10, 15)]; v < 120 || v > 135 {
		t.Errorf("composited value %d, want about 128", v)
	}
}

// An image that is not over 2x the target keeps the old single-step path, so
// the result is identical to it.
func TestSmallReductionTakesOldPath(t *testing.T) {
	src := realisticPage(300, 450)
	dw, dh := thumbSize(300, 450, 200, 300)
	if shrinkFactor(300, 450, dw, dh) != 0 {
		t.Fatal("expected no shrink")
	}
	got := scaleThumb(src, dw, dh)
	want := oldScale(src, dw, dh)
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Error("result differs from the old single-step path")
	}
}
