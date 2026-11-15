package media

import (
	"bytes"
	"image"
	"image/color"
	"math"
)

// Bytes per decoded pixel for formats the estimate models only roughly. The
// AVIF figure was measured on a 1600x2400 realistic page: the wasm decoder
// allocates about 193 MB (about 50 bytes a pixel); the constant rounds it up.
// bytesPerPixelFallback is used for any format or header the estimate does not
// read, and is generous on purpose.
const (
	bytesPerPixelAVIF     = 52
	bytesPerPixelFallback = 12
)

// decodeUse is what a decoded image is used for. It adds the memory that use
// needs on top of the decode itself.
type decodeUse int

const (
	useThumb decodeUse = iota // a thumbnail (Thumbnail)
	useJPEG                   // ToJPEG, which draws into a full-size RGBA
	usePNG                    // DecodeBudgeted: the Komga PNG conversion
)

// pngRowBuffers is the bytes per pixel of image width that a conversion to
// PNG holds besides the decoded image: image/png's encoder keeps six filter
// rows (five candidates and the previous row) and a PNG source is read through
// two rows, each up to 8 bytes a pixel (16-bit RGBA). They do not depend on the
// height, so a very wide, one-row image costs far more than its pixels.
// pngFixedBuffers covers the zlib and bufio state of the encoder.
const (
	pngRowBuffers   = (6 + 2) * 8
	pngFixedBuffers = 1 << 20
)

// decodeCost estimates the peak bytes that decoding img (in format, with header
// cfg) and then using it for use needs. It is never less than 1.
//
// The estimate is the decoded size D plus an overhead for the use. D follows
// the image's frame header: JPEG sampling (and CMYK and progressive coefficient
// blocks), PNG colour type, bit depth and interlace, one byte a pixel for GIF.
// The overhead applies to JPEG, PNG and GIF whose header parsed. A thumbnail
// adds 1 pixel when boxShrink handles the decoded type (every JPEG, PNG gray up
// to 8 bits, PNG RGB, RGBA and gray+alpha at 8 bits) and 4 pixels otherwise;
// ToJPEG adds 5 pixels in every case. Fallbacks (an unparsed header, AVIF and
// other formats) use bytesPerPixelFallback or bytesPerPixelAVIF with no overhead.
//
// Peak live heap per pixel, measured with Go 1.27 (shortened). Every formula
// result is at or above the matching measured total:
//
//	image                     decode  thumbnail  ToJPEG
//	JPEG baseline 4:2:0         1.50       2.36    5.86
//	JPEG baseline 4:4:4         3.00       3.85    7.36
//	JPEG gray                   1.00       1.73    5.18
//	JPEG CMYK (4 comps 1x1)     8.01       8.87   12.01
//	JPEG progressive 4:2:0      7.51       8.36   11.86
//	JPEG progressive gray       5.00       5.74    9.18
//	JPEG progressive CMYK      24.01      24.88   28.02
//	PNG gray8                   1.00       1.74    5.18
//	PNG RGB8 / RGBA8            4.01       4.87    8.20
//	PNG paletted                1.05       4.32    5.71
//	PNG RGBA16                  8.01      11.30   12.06
func decodeCost(img []byte, format string, cfg image.Config, use decodeUse) int64 {
	px := int64(cfg.Width) * int64(cfg.Height)
	d, shrinkable, counted := decodedBytes(img, format, cfg, px)
	if use == usePNG {
		// The encoder reads the decoded image as it is: no full-size copy,
		// only the row buffers, whatever the source format.
		return max(d+pngRowBuffers*(int64(cfg.Width)+1)+pngFixedBuffers, 1)
	}
	if counted {
		switch {
		case use == useJPEG:
			d += 5 * px
		case shrinkable:
			d += px
		default:
			d += 4 * px
		}
	}
	return max(d, 1)
}

// decodedBytes returns D, the decoded size of img in bytes for px pixels;
// whether boxShrink handles the decoded type; and whether the use overhead
// applies. The overhead does not apply to the fallbacks or to AVIF.
func decodedBytes(img []byte, format string, cfg image.Config, px int64) (d int64, shrinkable, counted bool) {
	switch format {
	case "jpeg":
		progressive, comps, ok := jpegFrame(img)
		if !ok {
			return jpegWorstBytes(int64(cfg.Width), int64(cfg.Height)), false, false
		}
		// DecodeConfig stops at the first scan, but markers after it still
		// decide whether the decoder converts to RGBA: read them all.
		rgb := cfg.ColorModel == color.RGBAModel || (len(comps) == 3 && jpegIsRGB(img))
		d, ok = jpegDecodedBytes(int64(cfg.Width), int64(cfg.Height), comps, progressive, rgb)
		if !ok {
			return jpegWorstBytes(int64(cfg.Width), int64(cfg.Height)), false, false
		}
		return d, true, true
	case "png":
		ct, depth, interlace, trns, ok := pngHeader(img)
		if !ok {
			return bytesPerPixelFallback * px, false, false
		}
		var b int64
		switch ct {
		case 0: // gray; with tRNS image/png decodes it as NRGBA(64)
			b = 1
			if trns {
				b = 4
			}
			if depth == 16 {
				b *= 2
			}
		case 2, 4, 6: // RGB, gray+alpha, RGBA
			b = 4
			if depth == 16 {
				b = 8
			}
		case 3: // palette
			b = 1
		default:
			return bytesPerPixelFallback * px, false, false
		}
		d = b * px
		if interlace == 1 { // Adam7 decodes into a second buffer
			d *= 2
		}
		return d, ct != 3 && depth <= 8, true
	case "gif":
		return px, false, true
	case "avif":
		return bytesPerPixelAVIF * px, false, false
	default:
		return bytesPerPixelFallback * px, false, false
	}
}

// jpegWorstBytes bounds what image/jpeg can allocate for a w x h JPEG whose
// frame header the estimate could not read: a progressive four-component image
// whose components all hold the largest sampling factors (4x4), so 3 planes
// (flex mode stores them at full resolution), a K plane, the CMYK output and
// four coefficient planes of 4 bytes a pixel each: 24 bytes a pixel over the
// size padded to 32-pixel MCUs, plus one for a thumbnail.
func jpegWorstBytes(w, h int64) int64 {
	wp, hp := (w+31)/32*32, (h+31)/32*32
	return 24*wp*hp + w*h
}

// jpegIsRGB reports whether image/jpeg will decode the three-component JPEG
// img as RGB (and convert it into a new RGBA image). It mirrors the decoder's
// isRGB, which runs after the whole file is read: the last APP0 decides JFIF
// (then never RGB), else the last Adobe APP14 with transform 0 means RGB, else
// components named R, G and B do. Unlike DecodeConfig, which stops at the first
// scan, it walks the markers after each scan too, skipping the entropy-coded
// data (where a 0xFF byte is followed by 0x00 or a restart marker).
func jpegIsRGB(img []byte) bool {
	if len(img) < 4 || img[0] != 0xFF || img[1] != 0xD8 {
		return false
	}
	jfif, adobeValid, adobeTransform, idsRGB := false, false, byte(0), false
	sawFrame := false
	for i := 2; i+1 < len(img); {
		if img[i] != 0xFF {
			j := bytes.IndexByte(img[i:], 0xFF)
			if j < 0 {
				break
			}
			i += j
			continue
		}
		m := img[i+1]
		switch {
		case m == 0xFF: // fill byte
			i++
			continue
		case m == 0x00, m == 0x01, m >= 0xD0 && m <= 0xD8: // stuffed byte, TEM, RSTn, SOI
			i += 2
			continue
		case m == 0xD9: // EOI
			i = len(img)
			continue
		}
		if i+4 > len(img) {
			break
		}
		n := int(img[i+2])<<8 | int(img[i+3])
		if n < 2 || i+2+n > len(img) {
			break
		}
		p := img[i+4 : i+2+n]
		switch {
		case m == 0xE0 && len(p) >= 5:
			jfif = string(p[:5]) == "JFIF\x00"
		case m == 0xEE && len(p) >= 12 && string(p[:5]) == "Adobe":
			adobeValid, adobeTransform = true, p[11]
		case isSOF(m) && !sawFrame && len(p) >= 6+9 && p[5] == 3:
			sawFrame = true
			idsRGB = p[6] == 'R' && p[9] == 'G' && p[12] == 'B'
		}
		i += 2 + n
	}
	switch {
	case jfif:
		return false
	case adobeValid && adobeTransform == 0:
		return true
	}
	return idsRGB
}

// jpegDecodedBytes is what image/jpeg (Go 1.27) allocates to decode a w x h
// JPEG whose components have the sampling factors comps, mirroring its makeImg
// and processSOS:
//   - one component: a gray image padded to 8x8 blocks;
//   - three or four: a YCbCr image padded to whole MCUs. A standard ratio
//     (4:4:4, 4:4:0, 4:2:2, 4:2:0, 4:1:1, 4:1:0, Y holding the largest
//     factors and Cb equal to Cr) keeps the chroma planes subsampled; any
//     other layout ("flex mode") stores all three planes at full resolution;
//   - four (CMYK): a K plane (counted at full padded size) and the CMYK output;
//   - RGB (rgb, three components that are not YCbCr): the RGBA output;
//   - progressive: an int32 coefficient block (256 bytes) per 8x8 block of
//     each component, at its sampled size.
//
// ok is false for a component count image/jpeg does not read (2, or over 4).
func jpegDecodedBytes(w, h int64, comps [][2]int, progressive, rgb bool) (int64, bool) {
	ceilDiv := func(a, b int64) int64 { return (a + b - 1) / b }
	n := len(comps)
	if n != 1 && n != 3 && n != 4 {
		return 0, false
	}
	hmax, vmax := int64(1), int64(1)
	for _, c := range comps {
		hmax = max(hmax, int64(c[0]))
		vmax = max(vmax, int64(c[1]))
	}
	var d int64
	if n == 1 {
		// image/jpeg ignores the sampling factors of a single component.
		d = 8 * ceilDiv(w, 8) * 8 * ceilDiv(h, 8)
		if progressive {
			d += 4 * d
		}
		return d, true
	}
	mxx, myy := ceilDiv(w, 8*hmax), ceilDiv(h, 8*vmax)
	wp, hp := 8*hmax*mxx, 8*vmax*myy
	y, cb, cr := comps[0], comps[1], comps[2]
	flex := cb != cr || int64(y[0]) != hmax || int64(y[1]) != vmax
	hr, vr := int64(1), int64(1)
	if !flex {
		hr, vr = hmax/int64(cb[0]), vmax/int64(cb[1])
		switch hr<<4 | vr {
		case 0x11, 0x12, 0x21, 0x22, 0x41, 0x42:
		default:
			flex = true
		}
	}
	if flex {
		d = 3 * wp * hp
	} else {
		d = wp*hp + 2*ceilDiv(wp, hr)*ceilDiv(hp, vr)
	}
	if n == 4 {
		d += wp*hp + 4*w*h
	}
	if n == 3 && rgb {
		d += 4 * w * h
	}
	if progressive {
		for _, c := range comps {
			d += mxx * myy * int64(c[0]) * int64(c[1]) * 256
		}
	}
	return d, true
}

// scaleCost estimates the memory scaleThumb needs on top of the decoded image
// to fit a w x h image of format into maxW x maxH: the box-shrunk copy is in
// decodeCost's overhead; this is the CatmullRom pass. x/image/draw allocates a
// [4]float64 buffer of destination width x source height, and per axis one
// contribution (16 bytes) per source pixel a destination pixel reads, about
// 4*scale+2 of them for each destination pixel. The source is the shrunk copy
// when boxShrink takes the decoded type, else the full image. This term grows
// with the source height and, for an extreme aspect ratio (a 1-pixel-wide
// strip, which boxShrink cannot reduce), it is what refuses the image.
// maxW or maxH of zero or less (no thumbnail) costs nothing.
func scaleCost(img []byte, format string, cfg image.Config, maxW, maxH int) int64 {
	if maxW <= 0 || maxH <= 0 || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0
	}
	_, shrinkable, _ := decodedBytes(img, format, cfg, 0)
	dw, dh := thumbSize(cfg.Width, cfg.Height, maxW, maxH)
	sw, sh := cfg.Width, cfg.Height
	if k := shrinkFactor(sw, sh, dw, dh); k >= 2 && shrinkable {
		sw, sh = (sw+k-1)/k, (sh+k-1)/k
	}
	taps := func(d, s int) float64 {
		return float64(d) * (4*math.Max(float64(s)/float64(d), 1) + 2)
	}
	tmp := 32 * float64(dw) * float64(sh)
	contribs := 16 * (taps(dw, sw) + taps(dh, sh))
	sources := 32 * float64(dw+dh)
	dst := 4 * float64(dw) * float64(dh)
	return int64(math.Ceil(tmp + contribs + sources + dst))
}

// pngHeader reads the fields decodeCost needs from the IHDR chunk, at fixed
// offsets: bit depth, colour type and interlace method. trns reports a tRNS
// chunk before the first IDAT, which makes image/png decode gray as NRGBA. ok
// is false when img is too short or does not start with an IHDR chunk.
func pngHeader(img []byte) (colourType, depth, interlace byte, trns, ok bool) {
	if len(img) < 29 || string(img[12:16]) != "IHDR" {
		return 0, 0, 0, false, false
	}
	for i := 8; i+8 <= len(img); {
		n := int64(img[i])<<24 | int64(img[i+1])<<16 | int64(img[i+2])<<8 | int64(img[i+3])
		typ := string(img[i+4 : i+8])
		if typ == "IDAT" || typ == "IEND" {
			break
		}
		if typ == "tRNS" {
			trns = true
			break
		}
		next := int64(i) + 12 + n
		if next > int64(len(img)) {
			break
		}
		i = int(next)
	}
	return img[25], img[24], img[28], trns, true
}

// jpegFrame reads the frame header of a JPEG: whether it is progressive, and
// the horizontal and vertical sampling factors (h, v) of each component. ok is
// false for malformed input, or when no frame header comes before the scan.
func jpegFrame(img []byte) (progressive bool, comps [][2]int, ok bool) {
	jpegSegments(img, func(m byte, payload []byte) bool {
		if !isSOF(m) {
			return true
		}
		comps, ok = parseFrame(payload)
		progressive = m == 0xC2 || m == 0xC6 || m == 0xCA || m == 0xCE
		return false // the first frame header is the one; stop here
	})
	return progressive, comps, ok
}

// parseFrame reads the components of a frame header payload: precision (1
// byte), height and width (2 bytes each), component count, then three bytes per
// component (id, sampling byte h<<4|v, quantisation table).
func parseFrame(payload []byte) ([][2]int, bool) {
	if len(payload) < 6 {
		return nil, false
	}
	n := int(payload[5])
	if n < 1 || len(payload) < 6+3*n {
		return nil, false
	}
	comps := make([][2]int, n)
	for k := 0; k < n; k++ {
		s := payload[6+3*k+1]
		h, v := int(s>>4), int(s&0x0F)
		if h == 0 || v == 0 {
			return nil, false
		}
		comps[k] = [2]int{h, v}
	}
	return comps, true
}
