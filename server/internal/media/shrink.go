package media

import (
	"image"
	"image/color"
	"math"

	xdraw "golang.org/x/image/draw"
)

// thumbSize is the size of a w x h image scaled to fit maxW x maxH, keeping
// the aspect ratio. An image that already fits keeps its size.
func thumbSize(w, h, maxW, maxH int) (int, int) {
	if w <= maxW && h <= maxH {
		return w, h
	}
	scale := math.Min(float64(maxW)/float64(w), float64(maxH)/float64(h))
	dw := min(maxW, max(1, int(math.Round(float64(w)*scale))))
	dh := min(maxH, max(1, int(math.Round(float64(h)*scale))))
	return dw, dh
}

// scaleThumb scales src to dw x dh over a white background. A source more
// than 4x the target (in the smaller ratio) is first shrunk by an integer box
// average, which is much faster than CatmullRom from full size and keeps the
// result close to it; the CatmullRom pass then finishes the scale.
func scaleThumb(src image.Image, dw, dh int) *image.RGBA {
	b := src.Bounds()
	if k := shrinkFactor(b.Dx(), b.Dy(), dw, dh); k >= 2 {
		if small, ok := boxShrink(src, k); ok {
			src = small
			b = small.Bounds()
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	xdraw.Draw(dst, dst.Bounds(), image.White, image.Point{}, xdraw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	return dst
}

// shrinkFactor is the integer box factor for a w x h source scaled to dw x dh:
// floor(min ratio / 2). It is 0 when that is below 2, so small reductions keep
// the single CatmullRom pass. The shrunk image is then still at least twice
// the target, so the final pass keeps its quality.
func shrinkFactor(w, h, dw, dh int) int {
	if dw <= 0 || dh <= 0 {
		return 0
	}
	r := math.Min(float64(w)/float64(dw), float64(h)/float64(dh))
	k := int(r / 2)
	if k < 2 {
		return 0
	}
	return k
}

// boxShrink shrinks src by the integer factor k. Each output pixel is the mean
// of a k x k block of source pixels; the blocks on the right and bottom edges
// are partial when the size is not a multiple of k, and are averaged over the
// pixels they hold, so nothing is dropped and nothing shifts. The output is
// ceil(w/k) x ceil(h/k) with its origin at (0, 0).
//
// Supported types: *image.YCbCr (output *image.RGBA, averaged in YCbCr and
// converted once per output pixel), *image.RGBA (output *image.RGBA, its
// premultiplied values average directly), *image.NRGBA (output *image.NRGBA,
// colour weighted by alpha so it composites the same as before), and
// *image.Gray, and *image.CMYK (output *image.RGBA, each pixel converted to
// RGB before averaging). Any other type returns false.
func boxShrink(src image.Image, k int) (image.Image, bool) {
	switch s := src.(type) {
	case *image.YCbCr:
		return shrinkYCbCr(s, k), true
	case *image.RGBA:
		return shrinkRGBA(s, k), true
	case *image.NRGBA:
		return shrinkNRGBA(s, k), true
	case *image.Gray:
		return shrinkGray(s, k), true
	case *image.CMYK:
		return shrinkCMYK(s, k), true
	}
	return nil, false
}

// avgRound is sum / n rounded to the nearest integer. sum is at most 255*n, so
// the result fits a byte.
func avgRound(sum, n int) uint8 {
	return uint8((sum + n/2) / n)
}

// chromaHShift is the horizontal subsampling factor of a YCbCr ratio, the
// value by which a luma x maps to its chroma column (x / factor). Unknown
// ratios are treated as 4:4:4, as image.YCbCr.COffset does.
func chromaHShift(r image.YCbCrSubsampleRatio) int {
	switch r {
	case image.YCbCrSubsampleRatio422, image.YCbCrSubsampleRatio420:
		return 2
	case image.YCbCrSubsampleRatio411, image.YCbCrSubsampleRatio410:
		return 4
	}
	return 1
}

func shrinkYCbCr(s *image.YCbCr, k int) *image.RGBA {
	b := s.Bounds()
	w, h := b.Dx(), b.Dy()
	ow, oh := (w+k-1)/k, (h+k-1)/k
	dst := image.NewRGBA(image.Rect(0, 0, ow, oh))
	hs := chromaHShift(s.SubsampleRatio)
	for oy := 0; oy < oh; oy++ {
		y0 := b.Min.Y + oy*k
		y1 := min(y0+k, b.Max.Y)
		for ox := 0; ox < ow; ox++ {
			x0 := b.Min.X + ox*k
			x1 := min(x0+k, b.Max.X)
			var sy, scb, scr int
			for y := y0; y < y1; y++ {
				yo := s.YOffset(x0, y)
				co := s.COffset(x0, y)
				// Luma x maps to chroma co + x/hs - x0/hs on this row.
				c0 := x0 / hs
				for x := x0; x < x1; x++ {
					sy += int(s.Y[yo+x-x0])
					ci := co + x/hs - c0
					scb += int(s.Cb[ci])
					scr += int(s.Cr[ci])
				}
			}
			n := (x1 - x0) * (y1 - y0)
			r, g, bl := color.YCbCrToRGB(avgRound(sy, n), avgRound(scb, n), avgRound(scr, n))
			i := dst.PixOffset(ox, oy)
			dst.Pix[i+0] = r
			dst.Pix[i+1] = g
			dst.Pix[i+2] = bl
			dst.Pix[i+3] = 255
		}
	}
	return dst
}

func shrinkRGBA(s *image.RGBA, k int) *image.RGBA {
	b := s.Bounds()
	w, h := b.Dx(), b.Dy()
	ow, oh := (w+k-1)/k, (h+k-1)/k
	dst := image.NewRGBA(image.Rect(0, 0, ow, oh))
	for oy := 0; oy < oh; oy++ {
		y0 := b.Min.Y + oy*k
		y1 := min(y0+k, b.Max.Y)
		for ox := 0; ox < ow; ox++ {
			x0 := b.Min.X + ox*k
			x1 := min(x0+k, b.Max.X)
			var r, g, bl, a int
			for y := y0; y < y1; y++ {
				i := s.PixOffset(x0, y)
				for x := x0; x < x1; x++ {
					p := s.Pix[i : i+4 : i+4]
					r += int(p[0])
					g += int(p[1])
					bl += int(p[2])
					a += int(p[3])
					i += 4
				}
			}
			n := (x1 - x0) * (y1 - y0)
			j := dst.PixOffset(ox, oy)
			dst.Pix[j+0] = avgRound(r, n)
			dst.Pix[j+1] = avgRound(g, n)
			dst.Pix[j+2] = avgRound(bl, n)
			dst.Pix[j+3] = avgRound(a, n)
		}
	}
	return dst
}

func shrinkNRGBA(s *image.NRGBA, k int) *image.NRGBA {
	b := s.Bounds()
	w, h := b.Dx(), b.Dy()
	ow, oh := (w+k-1)/k, (h+k-1)/k
	dst := image.NewNRGBA(image.Rect(0, 0, ow, oh))
	for oy := 0; oy < oh; oy++ {
		y0 := b.Min.Y + oy*k
		y1 := min(y0+k, b.Max.Y)
		for ox := 0; ox < ow; ox++ {
			x0 := b.Min.X + ox*k
			x1 := min(x0+k, b.Max.X)
			// Colour is weighted by alpha: sum(c*a)/sum(a) is the colour
			// that, with alpha sum(a)/n, reproduces the mean premultiplied
			// value sum(c*a)/n.
			var sr, sg, sb, sa int
			for y := y0; y < y1; y++ {
				i := s.PixOffset(x0, y)
				for x := x0; x < x1; x++ {
					p := s.Pix[i : i+4 : i+4]
					a := int(p[3])
					sr += int(p[0]) * a
					sg += int(p[1]) * a
					sb += int(p[2]) * a
					sa += a
					i += 4
				}
			}
			n := (x1 - x0) * (y1 - y0)
			j := dst.PixOffset(ox, oy)
			if sa == 0 {
				dst.Pix[j+0], dst.Pix[j+1], dst.Pix[j+2] = 0, 0, 0
			} else {
				dst.Pix[j+0] = avgRound(sr, sa)
				dst.Pix[j+1] = avgRound(sg, sa)
				dst.Pix[j+2] = avgRound(sb, sa)
			}
			dst.Pix[j+3] = avgRound(sa, n)
		}
	}
	return dst
}

// shrinkCMYK shrinks a CMYK image to RGB: each source pixel is converted with
// color.CMYKToRGB and the RGB channels are averaged. The output is opaque.
func shrinkCMYK(s *image.CMYK, k int) *image.RGBA {
	b := s.Bounds()
	w, h := b.Dx(), b.Dy()
	ow, oh := (w+k-1)/k, (h+k-1)/k
	dst := image.NewRGBA(image.Rect(0, 0, ow, oh))
	for oy := 0; oy < oh; oy++ {
		y0 := b.Min.Y + oy*k
		y1 := min(y0+k, b.Max.Y)
		for ox := 0; ox < ow; ox++ {
			x0 := b.Min.X + ox*k
			x1 := min(x0+k, b.Max.X)
			var r, g, bl int
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					c := s.CMYKAt(x, y)
					pr, pg, pb := color.CMYKToRGB(c.C, c.M, c.Y, c.K)
					r += int(pr)
					g += int(pg)
					bl += int(pb)
				}
			}
			n := (x1 - x0) * (y1 - y0)
			j := dst.PixOffset(ox, oy)
			dst.Pix[j+0] = avgRound(r, n)
			dst.Pix[j+1] = avgRound(g, n)
			dst.Pix[j+2] = avgRound(bl, n)
			dst.Pix[j+3] = 255
		}
	}
	return dst
}

func shrinkGray(s *image.Gray, k int) *image.Gray {
	b := s.Bounds()
	w, h := b.Dx(), b.Dy()
	ow, oh := (w+k-1)/k, (h+k-1)/k
	dst := image.NewGray(image.Rect(0, 0, ow, oh))
	for oy := 0; oy < oh; oy++ {
		y0 := b.Min.Y + oy*k
		y1 := min(y0+k, b.Max.Y)
		for ox := 0; ox < ow; ox++ {
			x0 := b.Min.X + ox*k
			x1 := min(x0+k, b.Max.X)
			var sum int
			for y := y0; y < y1; y++ {
				i := s.PixOffset(x0, y)
				for x := x0; x < x1; x++ {
					sum += int(s.Pix[i])
					i++
				}
			}
			n := (x1 - x0) * (y1 - y0)
			dst.Pix[dst.PixOffset(ox, oy)] = avgRound(sum, n)
		}
	}
	return dst
}
