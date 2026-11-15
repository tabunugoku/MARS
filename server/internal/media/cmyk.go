package media

import (
	"bytes"
	"io"
)

// adobeAPP14 is an Adobe APP14 segment: version 100, flags 0, colour transform
// 0. Placed right after SOI it tells image/jpeg that a 4-component JPEG is
// CMYK in the inverted convention that Photoshop, browsers and Pillow use.
var adobeAPP14 = [16]byte{
	0xFF, 0xEE, 0x00, 0x0E, 'A', 'd', 'o', 'b', 'e', 0x00, 0x64, 0x00, 0x00, 0x00, 0x00, 0x00,
}

// needsAdobeMarker reports whether img is a 4-component (CMYK) JPEG with no
// Adobe APP14 segment before the scan. image/jpeg rejects such files, while
// browsers and Pillow read them with the inverted CMYK convention. Malformed
// input returns false.
func needsAdobeMarker(img []byte) bool {
	four := false
	ok := jpegSegments(img, func(m byte, payload []byte) bool {
		switch {
		case isSOF(m):
			if len(payload) < 6 {
				return false
			}
			if payload[5] == 4 {
				four = true
			}
		case m == 0xEE && bytes.HasPrefix(payload, []byte("Adobe")):
			return false
		}
		return true
	})
	return ok && four
}

// jpegSegments walks the marker segments of a JPEG from SOI up to the SOS or
// EOI marker, calling visit with each marker and its payload (nil for the
// standalone markers, which have no length). visit returns false to stop the
// walk. jpegSegments returns true only when it reaches SOS or EOI; malformed or
// truncated input, and a visit that stopped the walk, return false.
func jpegSegments(img []byte, visit func(m byte, payload []byte) bool) bool {
	if len(img) < 4 || img[0] != 0xFF || img[1] != 0xD8 {
		return false
	}
	i := 2
	for {
		if i+1 >= len(img) || img[i] != 0xFF {
			return false
		}
		m := img[i+1]
		if m == 0xFF { // fill byte before a marker
			i++
			continue
		}
		i += 2
		if m == 0xDA || m == 0xD9 { // SOS or EOI
			return true
		}
		if m == 0x00 {
			return false
		}
		if m == 0x01 || (m >= 0xD0 && m <= 0xD7) { // standalone: no length
			if !visit(m, nil) {
				return false
			}
			continue
		}
		if i+2 > len(img) {
			return false
		}
		segLen := int(img[i])<<8 | int(img[i+1])
		if segLen < 2 || i+segLen > len(img) {
			return false
		}
		if !visit(m, img[i+2:i+segLen]) {
			return false
		}
		i += segLen
	}
}

// isSOF reports whether marker m starts a frame header (SOF0-SOF15 except the
// DHT, JPG and DAC markers C4, C8 and CC).
func isSOF(m byte) bool {
	switch {
	case m >= 0xC0 && m <= 0xC3, m >= 0xC5 && m <= 0xC7,
		m >= 0xC9 && m <= 0xCB, m >= 0xCD && m <= 0xCF:
		return true
	}
	return false
}

// jpegReader returns a reader over img for image.Decode. A 4-component JPEG
// without an Adobe APP14 segment gets one inserted after SOI (the original
// bytes are not copied), which makes image/jpeg decode it as inverted CMYK,
// the convention Photoshop, browsers and Pillow use. Other input is read as is.
func jpegReader(img []byte) io.Reader {
	if needsAdobeMarker(img) {
		return io.MultiReader(bytes.NewReader(img[:2]), bytes.NewReader(adobeAPP14[:]), bytes.NewReader(img[2:]))
	}
	return bytes.NewReader(img)
}
