package archive

import (
	"bytes"
	"fmt"
	"image"
	"testing"
)

// TestPDFDrawnPageBoxes draws pages whose pixel size is not simply the
// MediaBox: rotated, cropped and fractional boxes. The served PNG must have
// the size EntrySize reported, or the reader's layout would be off.
func TestPDFDrawnPageBoxes(t *testing.T) {
	pages := []string{
		"/MediaBox [0 0 595 842] /Rotate 90",
		"/MediaBox [0 0 595 842] /Rotate 270",
		"/MediaBox [0 0 595 842] /CropBox [10 10 300.5 400.3]",
		"/MediaBox [0 0 612.3 791.7]",
		"/MediaBox [0 0 1000 3]",
	}
	objs := []pdfObj{
		{body: "<< /Type /Catalog /Pages 2 0 R >>"},
		{},
		{body: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"},
	}
	var kids string
	for _, box := range pages {
		n := len(objs) + 1
		kids += fmt.Sprintf("%d 0 R ", n)
		objs = append(objs,
			pdfObj{body: fmt.Sprintf("<< /Type /Page /Parent 2 0 R %s /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", box, n+1)},
			pdfObj{stream: []byte("BT /F1 12 Tf 20 20 Td (box) Tj ET\n")})
	}
	objs[1] = pdfObj{body: fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids, len(pages))}
	a, err := openPDFFile(t, buildPDF(objs))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for i, e := range a.Entries() {
		t.Run(pages[i], func(t *testing.T) { checkDrawnEntry(t, a, e.Name) })
	}
}

// TestPDFDrawnPageShowsAnnotations draws a page whose only marks are an
// annotation's appearance (a filled black square, as a stamp would be). The
// annotation must be in the PNG, not dropped.
func TestPDFDrawnPageShowsAnnotations(t *testing.T) {
	ap := "0 0 0 rg 0 0 160 160 re f\n"
	objs := []pdfObj{
		{body: "<< /Type /Catalog /Pages 2 0 R >>"},
		{body: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		{body: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << >> /Annots [5 0 R] /Contents 4 0 R >>"},
		{stream: []byte("")},
		{body: "<< /Type /Annot /Subtype /Square /Rect [20 20 180 180] /F 4 /AP << /N 6 0 R >> >>"},
		{body: "/Type /XObject /Subtype /Form /BBox [0 0 160 160]", stream: []byte(ap)},
	}
	a, err := openPDFFile(t, buildPDF(objs))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	checkEntryNames(t, a, "0001-rendered.png")
	b := checkDrawnEntry(t, a, "0001-rendered.png")
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	// The square covers 20..180 of 200 points, so the centre is black.
	cx, cy := img.Bounds().Dx()/2, img.Bounds().Dy()/2
	if r, g, bl, _ := img.At(cx, cy).RGBA(); r > 0x2000 || g > 0x2000 || bl > 0x2000 {
		t.Errorf("centre pixel = %v, want black: the annotation was not drawn", img.At(cx, cy))
	}
}
