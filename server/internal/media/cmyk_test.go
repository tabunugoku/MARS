package media

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"testing"
)

// cmykWant is the RGB a Pillow reader gives every pixel of the CMYK fixtures.
var cmykWant = color.RGBA{255, 102, 0, 255}

func near(a, b uint8, d int) bool {
	x := int(a) - int(b)
	return x >= -d && x <= d
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestNeedsAdobeMarker(t *testing.T) {
	if !needsAdobeMarker(readFixture(t, "cmyk-no-adobe.jpg")) {
		t.Error("cmyk-no-adobe.jpg: want true")
	}
	if needsAdobeMarker(readFixture(t, "cmyk-adobe.jpg")) {
		t.Error("cmyk-adobe.jpg: want false")
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	if needsAdobeMarker(buf.Bytes()) {
		t.Error("RGB jpeg: want false")
	}
	for _, in := range [][]byte{
		nil,
		{0xFF, 0xD8, 0xFF},
		{0xFF, 0xD8, 0xFF, 0xC0, 0x00, 0x20},
		{0xFF, 0xD8, 0xFF, 0xC0, 0x00, 0x02},
		[]byte("garbage data"),
		readFixture(t, "cmyk-no-adobe.jpg")[:40],
	} {
		if needsAdobeMarker(in) {
			t.Errorf("malformed input %x: want false", in)
		}
	}
}

func TestThumbnailCMYKNoAdobe(t *testing.T) {
	out, err := Thumbnail(readFixture(t, "cmyk-no-adobe.jpg"), 16, 16, FormatJPEG)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	c := color.RGBAModel.Convert(img.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2)).(color.RGBA)
	if !near(c.R, cmykWant.R, 12) || !near(c.G, cmykWant.G, 12) || !near(c.B, cmykWant.B, 12) {
		t.Errorf("centre pixel %v, want about %v", c, cmykWant)
	}
}

func TestDecodeBudgetedCMYKNoAdobe(t *testing.T) {
	src, release, err := DecodeBudgeted(context.Background(), readFixture(t, "cmyk-no-adobe.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	c := color.RGBAModel.Convert(src.At(5, 5)).(color.RGBA)
	if !near(c.R, cmykWant.R, 12) || !near(c.G, cmykWant.G, 12) || !near(c.B, cmykWant.B, 12) {
		t.Errorf("pixel (5,5) %v, want about %v", c, cmykWant)
	}
}

func TestShrinkCMYK(t *testing.T) {
	want := color.CMYK{C: 0, M: 128, Y: 255, K: 0}
	s := image.NewCMYK(image.Rect(0, 0, 4, 4))
	for i := 0; i < len(s.Pix); i += 4 {
		s.Pix[i], s.Pix[i+1], s.Pix[i+2], s.Pix[i+3] = want.C, want.M, want.Y, want.K
	}
	out := shrinkCMYK(s, 2)
	if out.Bounds() != image.Rect(0, 0, 2, 2) {
		t.Fatalf("bounds %v, want 2x2", out.Bounds())
	}
	r, g, b := color.CMYKToRGB(want.C, want.M, want.Y, want.K)
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			if c := out.RGBAAt(x, y); c != (color.RGBA{r, g, b, 255}) {
				t.Errorf("pixel (%d,%d) = %v, want %v", x, y, c, color.RGBA{r, g, b, 255})
			}
		}
	}
}
