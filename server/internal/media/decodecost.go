package media

import (
	"image"
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
	useThumb decodeUse = iota // a thumbnail (Thumbnail, DecodeBudgeted)
	useJPEG                   // ToJPEG, which draws into a full-size RGBA
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
	d, shrinkable, counted := decodedBytes(img, format, px)
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
func decodedBytes(img []byte, format string, px int64) (d int64, shrinkable, counted bool) {
	switch format {
	case "jpeg":
		progressive, comps, ok := jpegFrame(img)
		if !ok {
			return bytesPerPixelFallback * px, false, false
		}
		hmax, vmax := int64(1), int64(1)
		var sum int64
		for _, c := range comps {
			hmax = max(hmax, int64(c[0]))
			vmax = max(vmax, int64(c[1]))
			sum += int64(c[0]) * int64(c[1])
		}
		samples := px * sum / (hmax * vmax)
		d = samples
		if len(comps) == 4 { // CMYK output image
			d += 4 * px
		}
		if progressive { // int32 coefficient blocks
			d += 4 * samples
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
