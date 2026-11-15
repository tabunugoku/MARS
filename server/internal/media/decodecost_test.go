package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"testing"
	"time"
)

var (
	cost64x48 = image.Config{Width: 64, Height: 48}
	cost32x24 = image.Config{Width: 32, Height: 24} // the CMYK fixtures
)

func encodeJPEG(t *testing.T, m image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, m, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func encodePNG(t *testing.T, m image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, m); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// withTRNS returns a copy of a PNG with a valid tRNS chunk (gray value 0)
// inserted after IHDR. image/png then decodes a gray image as NRGBA.
func withTRNS(t *testing.T, p []byte) []byte {
	t.Helper()
	chunk := []byte{0, 0, 0, 2, 't', 'R', 'N', 'S', 0, 0}
	crc := crc32.ChecksumIEEE(chunk[4:])
	chunk = append(chunk, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
	out := append(bytes.Clone(p[:33]), chunk...)
	out = append(out, p[33:]...)
	m, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.(*image.NRGBA); !ok {
		t.Fatalf("gray + tRNS decoded as %T, want *image.NRGBA", m)
	}
	return out
}

// progressiveOf returns a copy of a baseline JPEG with its SOF0 marker changed
// to SOF2. decodeCost reads only the header, so the scan data is not checked.
func progressiveOf(t *testing.T, base []byte) []byte {
	t.Helper()
	p := bytes.Clone(base)
	i := bytes.Index(p, []byte{0xFF, 0xC0})
	if i < 0 {
		t.Fatal("no SOF0 marker")
	}
	p[i+1] = 0xC2
	return p
}

func TestDecodeCost(t *testing.T) {
	baseline := encodeJPEG(t, image.NewRGBA(image.Rect(0, 0, 64, 48)))
	grayJPEG := encodeJPEG(t, image.NewGray(image.Rect(0, 0, 64, 48)))
	nrgbaPNG := encodePNG(t, image.NewNRGBA(image.Rect(0, 0, 64, 48)))
	// Adam7 flag (IHDR interlace byte at offset 28). Only the header is read.
	adam7 := bytes.Clone(nrgbaPNG)
	adam7[28] = 1
	grayTRNS := withTRNS(t, encodePNG(t, image.NewGray(image.Rect(0, 0, 64, 48))))
	pal := color.Palette{color.Black, color.White, color.RGBA{255, 0, 0, 255}, color.Gray{128}}
	var gifBuf bytes.Buffer
	if err := gif.Encode(&gifBuf, image.NewPaletted(image.Rect(0, 0, 64, 48), pal), nil); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name          string
		img           []byte
		format        string
		cfg           image.Config
		thumb, toJPEG int64
	}{
		// px = 3072. Thumbnail: D + overhead (1 px shrinkable, 4 px not);
		// ToJPEG: D + 5 px.
		{"jpeg 4:2:0", baseline, "jpeg", cost64x48, 7680, 19968},
		{"jpeg gray", grayJPEG, "jpeg", cost64x48, 6144, 18432},
		{"jpeg progressive 4:2:0", progressiveOf(t, baseline), "jpeg", cost64x48, 26112, 38400},
		// px = 768: D = 8 px (four 1x1 components plus the CMYK output image).
		{"jpeg CMYK no Adobe", readFixture(t, "cmyk-no-adobe.jpg"), "jpeg", cost32x24, 6912, 9984},
		{"jpeg CMYK Adobe", readFixture(t, "cmyk-adobe.jpg"), "jpeg", cost32x24, 6912, 9984},
		{"png gray8", encodePNG(t, image.NewGray(image.Rect(0, 0, 64, 48))), "png", cost64x48, 6144, 18432},
		{"png RGBA8", nrgbaPNG, "png", cost64x48, 15360, 27648},
		{"png RGBA16", encodePNG(t, image.NewNRGBA64(image.Rect(0, 0, 64, 48))), "png", cost64x48, 36864, 39936},
		{"png paletted", encodePNG(t, image.NewPaletted(image.Rect(0, 0, 64, 48), pal)), "png", cost64x48, 15360, 18432},
		{"png gray8 tRNS", grayTRNS, "png", cost64x48, 15360, 27648},
		{"png gray16", encodePNG(t, image.NewGray16(image.Rect(0, 0, 64, 48))), "png", cost64x48, 18432, 21504},
		{"png RGBA8 Adam7", adam7, "png", cost64x48, 27648, 39936},
		{"gif", gifBuf.Bytes(), "gif", cost64x48, 15360, 18432},
		{"webp fallback", []byte("not an image"), "webp", cost64x48, 36864, 36864},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := decodeCost(c.img, c.format, c.cfg, useThumb); got != c.thumb {
				t.Errorf("useThumb cost = %d, want %d", got, c.thumb)
			}
			if got := decodeCost(c.img, c.format, c.cfg, useJPEG); got != c.toJPEG {
				t.Errorf("useJPEG cost = %d, want %d", got, c.toJPEG)
			}
		})
	}
}

func TestDecodeCostNeverBelowOne(t *testing.T) {
	if got := decodeCost(nil, "webp", image.Config{}, useThumb); got != 1 {
		t.Errorf("empty image cost = %d, want 1", got)
	}
}

func TestJPEGFrame(t *testing.T) {
	base := encodeJPEG(t, image.NewRGBA(image.Rect(0, 0, 64, 48)))
	progressive, comps, ok := jpegFrame(base)
	if !ok || progressive {
		t.Fatalf("baseline: ok=%v progressive=%v, want true false", ok, progressive)
	}
	if len(comps) != 3 || comps[0] != [2]int{2, 2} || comps[1] != [2]int{1, 1} {
		t.Errorf("baseline comps = %v, want 4:2:0 (2x2, 1x1, 1x1)", comps)
	}
	progressive, _, ok = jpegFrame(progressiveOf(t, base))
	if !ok || !progressive {
		t.Errorf("SOF2: ok=%v progressive=%v, want true true", ok, progressive)
	}
	_, comps, ok = jpegFrame(readFixture(t, "cmyk-no-adobe.jpg"))
	if !ok || len(comps) != 4 {
		t.Errorf("CMYK: ok=%v comps=%v, want 4 components", ok, comps)
	}
}

func TestJPEGFrameMalformed(t *testing.T) {
	base := encodeJPEG(t, image.NewRGBA(image.Rect(0, 0, 64, 48)))
	cases := map[string][]byte{
		"nil":                    nil,
		"not a jpeg":             []byte("GIF89a......"),
		"SOI only":               {0xFF, 0xD8},
		"truncated":              base[:100],
		"EOI without frame":      {0xFF, 0xD8, 0xFF, 0xD9},
		"segment past end":       {0xFF, 0xD8, 0xFF, 0xE0, 0xFF, 0xFF},
		"segment length below 2": {0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x01},
		"short frame payload":    {0xFF, 0xD8, 0xFF, 0xC0, 0x00, 0x05, 1, 2, 3},
		"frame short of components": {
			0xFF, 0xD8, 0xFF, 0xC0, 0x00, 0x08, 0x08, 0x00, 0x08, 0x00, 0x08, 0x03,
		},
		"zero components": {
			0xFF, 0xD8, 0xFF, 0xC0, 0x00, 0x08, 0x08, 0x00, 0x08, 0x00, 0x08, 0x00,
		},
		"zero sampling": {
			0xFF, 0xD8, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x08, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00,
		},
	}
	for name, img := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, ok := jpegFrame(img); ok {
				t.Error("ok = true for malformed input")
			}
		})
	}
}

func TestDecodeLimitRefusesBeforeBudget(t *testing.T) {
	SetDecodeLimit(1000)
	t.Cleanup(func() { SetDecodeLimit(DefaultDecodeLimit) })
	page := encodePNG(t, image.NewNRGBA(image.Rect(0, 0, 64, 48)))
	ctx := context.Background()

	_, release, err := DecodeBudgeted(ctx, page)
	if !errors.Is(err, ErrDecodeTooLarge) || !errors.Is(err, ErrTooLarge) {
		t.Fatalf("DecodeBudgeted err = %v, want ErrDecodeTooLarge and ErrTooLarge", err)
	}
	if release != nil {
		t.Error("release is not nil on error")
	}
	if n := decodeHolders.Load(); n != 0 {
		t.Errorf("holders after refused decode = %d, want 0", n)
	}
	if _, err := Thumbnail(page, 32, 32, FormatJPEG); !errors.Is(err, ErrTooLarge) {
		t.Errorf("Thumbnail err = %v, want ErrTooLarge", err)
	}

	SetDecodeLimit(DefaultDecodeLimit)
	_, release, err = DecodeBudgeted(ctx, page)
	if err != nil {
		t.Fatalf("with the default limit: %v", err)
	}
	release()
}

// An RGB (not YCbCr) JPEG: image/jpeg decodes it into a new RGBA image while it
// still holds the three planes, so it costs 7 bytes a pixel, not 3. The
// fixture is a 64x48 4:4:4 JPEG without JFIF whose components are R, G and B.
func TestDecodeCostRGBJPEG(t *testing.T) {
	d, err := os.ReadFile("testdata/rgb-no-jfif.jpg")
	if err != nil {
		t.Fatal(err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(d))
	if err != nil {
		t.Fatal(err)
	}
	m, _, err := image.Decode(bytes.NewReader(d))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.(*image.RGBA); !ok {
		t.Fatalf("fixture decodes as %T, want *image.RGBA", m)
	}
	px := int64(cfg.Width * cfg.Height)
	if got, want := decodeCost(d, format, cfg, useThumb), 8*px; got != want {
		t.Errorf("thumb cost = %d, want %d", got, want)
	}
	if got, want := decodeCost(d, format, cfg, useJPEG), 12*px; got != want {
		t.Errorf("ToJPEG cost = %d, want %d", got, want)
	}
}

// scaleCost follows the CatmullRom pass: a page is scaled from its box-shrunk
// copy and costs little, while a 1-pixel-wide strip cannot be shrunk and its
// scaler buffers grow with its height.
func TestScaleCost(t *testing.T) {
	grayPNG := encodePNG(t, image.NewGray(image.Rect(0, 0, 4, 4)))
	page := image.Config{Width: 4000, Height: 6000}
	if got := scaleCost(grayPNG, "png", page, 400, 600); got <= 0 || got > 32<<20 {
		t.Errorf("page scale cost = %d, want under 32 MB (the 800x1200 shrunk copy)", got)
	}
	if got := scaleCost(grayPNG, "png", page, 0, 0); got != 0 {
		t.Errorf("no thumbnail: scale cost = %d, want 0", got)
	}
	strip := image.Config{Width: 1, Height: 200_000}
	// tmp 32*1*200000 plus about 4*333+2 contributions per output row.
	if got := scaleCost(grayPNG, "png", strip, 400, 600); got < 32*200_000+16*600*1300 {
		t.Errorf("strip scale cost = %d, want at least %d", got, 32*200_000+16*600*1300)
	}
}

// A strip whose decode is tiny is refused by its scaler cost alone.
func TestThumbnailRefusesExtremeStrip(t *testing.T) {
	t.Cleanup(func() { SetDecodeLimit(DefaultDecodeLimit) })
	strip := encodePNG(t, image.NewGray(image.Rect(0, 0, 1, 200_000)))
	cfg, format, err := image.DecodeConfig(bytes.NewReader(strip))
	if err != nil {
		t.Fatal(err)
	}
	if c := decodeCost(strip, format, cfg, useThumb); c > 1<<20 {
		t.Fatalf("decode cost %d: the test needs a cheap decode", c)
	}
	SetDecodeLimit(8 << 20)
	if _, err := Thumbnail(strip, 400, 600, FormatJPEG); !errors.Is(err, ErrDecodeTooLarge) {
		t.Fatalf("Thumbnail err = %v, want ErrDecodeTooLarge", err)
	}
	_, release, err := DecodeBudgeted(context.Background(), strip)
	if err != nil {
		t.Fatalf("DecodeBudgeted (no scaling) err = %v", err)
	}
	release()
}

// A conversion to PNG (DecodeBudgeted, used by Komga) holds the encoder's row
// buffers, which grow with the width alone: a 32,000,000 x 1 RGBA PNG decodes
// to 128 MB but needs about 2 GB more, so it is refused.
func TestPNGConversionCost(t *testing.T) {
	t.Cleanup(func() { SetDecodeLimit(DefaultDecodeLimit) })
	wide := encodePNG(t, image.NewNRGBA(image.Rect(0, 0, 64, 1)))
	binary.BigEndian.PutUint32(wide[16:], 32_000_000)
	binary.BigEndian.PutUint32(wide[29:], crc32.ChecksumIEEE(wide[12:29]))
	cfg, format, err := image.DecodeConfig(bytes.NewReader(wide))
	if err != nil {
		t.Fatal(err)
	}
	got := decodeCost(wide, format, cfg, usePNG)
	if want := int64(4*32_000_000 + pngRowBuffers*(32_000_000+1) + pngFixedBuffers); got != want {
		t.Errorf("wide PNG conversion cost = %d, want %d", got, want)
	}
	SetDecodeLimit(640 << 20)
	if _, _, err := DecodeBudgeted(context.Background(), wide); !errors.Is(err, ErrDecodeTooLarge) {
		t.Fatalf("wide PNG conversion err = %v, want ErrDecodeTooLarge", err)
	}

	// A page: the rows are small next to the pixels.
	page := image.Config{Width: 4000, Height: 6000}
	if got, want := decodeCost(encodeJPEG(t, image.NewRGBA(image.Rect(0, 0, 16, 16))), "jpeg", page, usePNG),
		int64(4000*6000*3/2+pngRowBuffers*4001+pngFixedBuffers); got != want {
		t.Errorf("page conversion cost = %d, want %d", got, want)
	}
}

// Conversions are admitted by their full cost, row buffers included: with a
// budget for one and a half, a second wide conversion waits for the first.
func TestPNGConversionsShareTheBudget(t *testing.T) {
	wide := encodePNG(t, image.NewNRGBA(image.Rect(0, 0, 20_000, 1)))
	cfg, format, err := image.DecodeConfig(bytes.NewReader(wide))
	if err != nil {
		t.Fatal(err)
	}
	cost := decodeCost(wide, format, cfg, usePNG)
	useBudget(t, cost*3/2)
	_, release, err := DecodeBudgeted(context.Background(), wide)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, _, err := DecodeBudgeted(ctx, wide); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second conversion err = %v, want it to wait for the budget", err)
	}
}

// withFrame returns a copy of a baseline JPEG whose frame header says w x h
// and the given sampling bytes (h<<4|v) for its three components. Only the
// header is meant to be read: the scan data no longer matches.
func withFrame(t *testing.T, base []byte, w, h int, sampling [3]byte) []byte {
	t.Helper()
	out := bytes.Clone(base)
	i := bytes.Index(out, []byte{0xFF, 0xC0})
	if i < 0 || out[i+9] != 3 {
		t.Fatal("no 3-component SOF0 in the base JPEG")
	}
	binary.BigEndian.PutUint16(out[i+5:], uint16(h))
	binary.BigEndian.PutUint16(out[i+7:], uint16(w))
	for k, b := range sampling {
		out[i+11+3*k] = b
	}
	return out
}

// A non-standard sampling layout makes image/jpeg ("flex mode") store all
// three planes at full resolution, so it costs 3 bytes a pixel however small
// its sampled planes are. Codex's example: Y 4x1, Cb 1x4, Cr 1x1 on a
// 16000 x 12480 page is about 571 MiB of planes, and is refused by a 320 MiB
// limit before any decode.
func TestDecodeCostFlexSampling(t *testing.T) {
	t.Cleanup(func() { SetDecodeLimit(DefaultDecodeLimit) })
	base := encodeJPEG(t, image.NewRGBA(image.Rect(0, 0, 64, 48)))
	flex := withFrame(t, base, 16000, 12480, [3]byte{0x41, 0x14, 0x11})
	cfg, format, err := image.DecodeConfig(bytes.NewReader(flex))
	if err != nil {
		t.Fatal(err)
	}
	px := int64(16000 * 12480)
	if got, want := decodeCost(flex, format, cfg, useThumb), 3*px+px; got != want {
		t.Errorf("flex cost = %d, want %d (three full planes + thumbnail)", got, want)
	}
	SetDecodeLimit(320 << 20)
	if _, err := Thumbnail(flex, 400, 600, FormatJPEG); !errors.Is(err, ErrDecodeTooLarge) {
		t.Fatalf("flex Thumbnail err = %v, want ErrDecodeTooLarge", err)
	}
}

// jpegDecodedBytes mirrors image/jpeg's allocations, padding included.
func TestJPEGDecodedBytes(t *testing.T) {
	cases := []struct {
		name        string
		w, h        int64
		comps       [][2]int
		progressive bool
		rgb         bool
		want        int64
	}{
		{"gray, padded to 8", 10, 10, [][2]int{{1, 1}}, false, false, 16 * 16},
		{"gray progressive", 16, 16, [][2]int{{1, 1}}, true, false, 256 + 4*256},
		{"4:2:0 padded to MCUs", 17, 17, [][2]int{{2, 2}, {1, 1}, {1, 1}}, false, false, 32*32 + 2*16*16},
		{"4:1:1", 64, 16, [][2]int{{4, 1}, {1, 1}, {1, 1}}, false, false, 64*16 + 2*16*16},
		{"4:4:4 RGB", 16, 16, [][2]int{{1, 1}, {1, 1}, {1, 1}}, false, true, 3*256 + 4*256},
		{"flex: Cb differs from Cr", 32, 32, [][2]int{{2, 2}, {1, 2}, {1, 1}}, false, false, 3 * 32 * 32},
		{"flex: Y not largest", 32, 32, [][2]int{{1, 1}, {2, 2}, {2, 2}}, false, false, 3 * 32 * 32},
		{"flex: ratio 3", 48, 16, [][2]int{{3, 1}, {1, 1}, {1, 1}}, false, false, 3 * 48 * 16},
		{"4:2:0 progressive", 16, 16, [][2]int{{2, 2}, {1, 1}, {1, 1}}, true, false, 256 + 2*64 + (4+1+1)*256},
		{"CMYK", 8, 8, [][2]int{{1, 1}, {1, 1}, {1, 1}, {1, 1}}, false, false, 3*64 + 64 + 4*64},
	}
	for _, c := range cases {
		got, ok := jpegDecodedBytes(c.w, c.h, c.comps, c.progressive, c.rgb)
		if !ok || got != c.want {
			t.Errorf("%s: got %d (ok %v), want %d", c.name, got, ok, c.want)
		}
	}
	if _, ok := jpegDecodedBytes(8, 8, [][2]int{{1, 1}, {1, 1}}, false, false); ok {
		t.Error("two components: want ok=false")
	}
}

// lateAdobe returns base with an Adobe APP14 segment (transform 0) inserted
// just before EOI, after the scan, where DecodeConfig never looks.
func lateAdobe(t *testing.T, base []byte) []byte {
	t.Helper()
	if !bytes.HasSuffix(base, []byte{0xFF, 0xD9}) {
		t.Fatal("base JPEG does not end with EOI")
	}
	out := bytes.Clone(base[:len(base)-2])
	out = append(out, adobeAPP14[:]...)
	return append(out, 0xFF, 0xD9)
}

// withJFIF returns base with a JFIF APP0 segment right after SOI.
func withJFIF(base []byte) []byte {
	app0 := []byte{0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 1, 1, 0, 0, 1, 0, 1, 0, 0}
	out := append(bytes.Clone(base[:2]), app0...)
	return append(out, base[2:]...)
}

// An Adobe transform-0 marker after the scan makes image/jpeg decode a
// three-component JPEG as RGB and convert it into a new RGBA image, though
// DecodeConfig (which stops at the first scan) reports YCbCr. The estimate
// reads the late marker and counts the RGBA output; a JFIF marker turns it
// back to YCbCr, as in the decoder. The fixture is a 64x48 4:4:4 JPEG without
// JFIF or Adobe markers.
func TestDecodeCostLateAdobeMarker(t *testing.T) {
	base, err := os.ReadFile("testdata/ycc444-no-jfif.jpg")
	if err != nil {
		t.Fatal(err)
	}
	late := lateAdobe(t, base)
	cfg, format, err := image.DecodeConfig(bytes.NewReader(late))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ColorModel == color.RGBAModel {
		t.Fatal("DecodeConfig already reports RGB: the fixture does not test a late marker")
	}
	m, _, err := image.Decode(bytes.NewReader(late))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.(*image.RGBA); !ok {
		t.Fatalf("late Adobe JPEG decodes as %T, want *image.RGBA", m)
	}
	px := int64(cfg.Width * cfg.Height)
	if got, want := decodeCost(late, format, cfg, useThumb), 3*px+4*px+px; got != want {
		t.Errorf("late Adobe cost = %d, want %d (planes + RGBA + thumbnail)", got, want)
	}
	if got, want := decodeCost(base, format, cfg, useThumb), 3*px+px; got != want {
		t.Errorf("plain 4:4:4 cost = %d, want %d", got, want)
	}

	jfif := withJFIF(late)
	if m, _, err := image.Decode(bytes.NewReader(jfif)); err != nil {
		t.Fatal(err)
	} else if _, ok := m.(*image.YCbCr); !ok {
		t.Fatalf("JFIF + late Adobe decodes as %T, want *image.YCbCr", m)
	}
	if jpegIsRGB(jfif) {
		t.Error("JFIF + late Adobe: jpegIsRGB = true, want false")
	}
	if !jpegIsRGB(late) || jpegIsRGB(base) {
		t.Error("jpegIsRGB: want true with the late marker only")
	}
}

// withJunkBeforeSOF returns base with one extraneous byte before its frame
// header, which image/jpeg skips silently.
func withJunkBeforeSOF(t *testing.T, base []byte) []byte {
	t.Helper()
	i := -1
	for _, m := range []byte{0xC0, 0xC2} {
		if j := bytes.Index(base, []byte{0xFF, m}); j >= 0 {
			i = j
			break
		}
	}
	if i < 0 {
		t.Fatal("no SOF0/SOF2 in the base JPEG")
	}
	out := append(bytes.Clone(base[:i]), 0x00)
	return append(out, base[i:]...)
}

// Extraneous bytes before the frame header no longer hide it: the estimate
// reads past them as the decoder does. A progressive Adobe CMYK page of
// 16000 x 12000 with a junk byte before SOF costs 24 bytes a pixel and is
// refused by the 2560 MiB limit of a 4 GiB container before any decode.
func TestDecodeCostJunkBeforeFrame(t *testing.T) {
	t.Cleanup(func() { SetDecodeLimit(DefaultDecodeLimit) })
	base, err := os.ReadFile("testdata/cmyk-adobe.jpg")
	if err != nil {
		t.Fatal(err)
	}
	junk := withJunkBeforeSOF(t, base)
	if _, _, err := image.Decode(bytes.NewReader(junk)); err != nil {
		t.Fatalf("image/jpeg rejects the junk byte: %v", err)
	}
	prog := bytes.Clone(junk)
	i := bytes.Index(prog, []byte{0xFF, 0xC0})
	if i < 0 {
		t.Fatal("no SOF0 in the CMYK fixture")
	}
	prog[i+1] = 0xC2
	binary.BigEndian.PutUint16(prog[i+5:], 12000)
	binary.BigEndian.PutUint16(prog[i+7:], 16000)
	cfg, format, err := image.DecodeConfig(bytes.NewReader(prog))
	if err != nil {
		t.Fatal(err)
	}
	if progressive, comps, ok := jpegFrame(prog); !ok || !progressive || len(comps) != 4 {
		t.Fatalf("jpegFrame behind junk = %v, %v, %v; want progressive 4 components", progressive, comps, ok)
	}
	px := int64(16000 * 12000)
	if got, want := decodeCost(prog, format, cfg, useThumb), 24*px+px; got != want {
		t.Errorf("junk + progressive CMYK cost = %d, want %d", got, want)
	}
	SetDecodeLimit(2560 << 20)
	if _, err := Thumbnail(prog, 400, 600, FormatJPEG); !errors.Is(err, ErrDecodeTooLarge) {
		t.Fatalf("Thumbnail err = %v, want ErrDecodeTooLarge", err)
	}

	// The CMYK fix of needsAdobeMarker sees through the junk too.
	noAdobe, err := os.ReadFile("testdata/cmyk-no-adobe.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if !needsAdobeMarker(withJunkBeforeSOF(t, noAdobe)) {
		t.Error("needsAdobeMarker missed a CMYK frame behind a junk byte")
	}
}

// A JPEG whose frame header cannot be read at all is estimated at the worst
// layout image/jpeg can allocate, not at the generic 12 bytes a pixel.
func TestDecodeCostUnreadableJPEGFrame(t *testing.T) {
	cfg := image.Config{Width: 100, Height: 100}
	got := decodeCost([]byte{0xFF, 0xD8, 0xFF, 0xD9}, "jpeg", cfg, useThumb)
	if want := int64(24*128*128 + 100*100); got != want {
		t.Errorf("unreadable frame cost = %d, want %d", got, want)
	}
}
