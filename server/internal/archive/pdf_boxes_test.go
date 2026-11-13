package archive

import (
	"fmt"
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
