package media

import (
	"bytes"
	"context"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"testing"
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
