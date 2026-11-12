package archive

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// pdfObj is one PDF object. With stream set, body holds the dictionary
// entries and the stream bytes follow; otherwise body is the whole object.
// smask, when set, is written right after the object and referenced from
// its "SMASK" placeholder.
type pdfObj struct {
	body   string
	stream []byte
	smask  *pdfObj
}

// pdfPageSpec is one test page: its image XObjects in drawing order, and
// whether it also draws text.
type pdfPageSpec struct {
	images []pdfObj
	text   bool
}

// makePDF assembles a minimal PDF with a correct xref table. Pages are
// 200x200 points; images are drawn at x = 100*j.
func makePDF(pages []pdfPageSpec) []byte {
	objs := []pdfObj{
		{body: "<< /Type /Catalog /Pages 2 0 R >>"},
		{}, // 2: page tree, filled below
		{body: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"}, // 3
	}
	var kids string
	for _, pg := range pages {
		pageIdx := len(objs)
		objs = append(objs, pdfObj{}, pdfObj{}) // page and content placeholders
		kids += fmt.Sprintf("%d 0 R ", pageIdx+1)
		var xobjs, draw string
		for j, im := range pg.images {
			num := len(objs) + 1
			obj := im
			if im.smask != nil {
				obj.body = strings.ReplaceAll(im.body, "SMASK", fmt.Sprintf("%d 0 R", num+1))
			}
			objs = append(objs, obj)
			if im.smask != nil {
				objs = append(objs, *im.smask)
			}
			name := fmt.Sprintf("Im%d", j+1)
			xobjs += fmt.Sprintf("/%s %d 0 R ", name, num)
			draw += fmt.Sprintf("q 100 0 0 100 %d 0 cm /%s Do Q\n", 100*j, name)
		}
		if pg.text {
			draw += "BT /F1 12 Tf 10 180 Td (hello) Tj ET\n"
		}
		objs[pageIdx] = pdfObj{body: fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] "+
				"/Resources << /Font << /F1 3 0 R >> /XObject << %s>> >> /Contents %d 0 R >>",
			xobjs, pageIdx+2)}
		objs[pageIdx+1] = pdfObj{stream: []byte(draw)}
	}
	objs[1] = pdfObj{body: fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids, len(pages))}
	return buildPDF(objs)
}

// buildPDF writes objs as numbered objects with an xref table and trailer.
func buildPDF(objs []pdfObj) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n", i+1)
		if o.stream == nil {
			buf.WriteString(o.body)
		} else {
			fmt.Fprintf(&buf, "<< %s /Length %d >>\nstream\n", o.body, len(o.stream))
			buf.Write(o.stream)
			buf.WriteString("\nendstream")
		}
		buf.WriteString("\nendobj\n")
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offs {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return buf.Bytes()
}

// gradientRGB returns w*h RGB pixels, and the same picture as a JPEG.
func gradientRGB(w, h int) []byte {
	pix := make([]byte, 0, w*h*3)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			pix = append(pix, byte(x*255/w), byte(y*255/h), 128)
		}
	}
	return pix
}

func jpegObj(t *testing.T, w, h int) pdfObj {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	rgb := gradientRGB(w, h)
	for i := 0; i < w*h; i++ {
		img.Pix[i*4+0], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = rgb[i*3], rgb[i*3+1], rgb[i*3+2], 255
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return pdfObj{
		body: fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d "+
			"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode", w, h),
		stream: buf.Bytes(),
	}
}

// flateObj is an image stored as zlib-compressed raw pixels, which is what
// PNG-based pages carry once pdfcpu reads them.
func flateObj(w, h int, cs string, pix []byte, smask *pdfObj) pdfObj {
	var zb bytes.Buffer
	zw := zlib.NewWriter(&zb)
	zw.Write(pix)
	zw.Close()
	body := fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d "+
		"/ColorSpace %s /BitsPerComponent 8 /Filter /FlateDecode", w, h, cs)
	if smask != nil {
		body += " /SMask SMASK"
	}
	return pdfObj{body: body, stream: zb.Bytes(), smask: smask}
}

// writePDFFile puts data in a temp dir as book.pdf and returns its path.
func writePDFFile(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func openPDFFile(t *testing.T, data []byte) (Archive, error) {
	t.Helper()
	a, format, err := Open(writePDFFile(t, data))
	if err == nil && format != PDF {
		t.Fatalf("format = %q, want pdf", format)
	}
	return a, err
}

func readEntry(t *testing.T, a Archive, name string) []byte {
	t.Helper()
	rc, err := a.Open(name)
	if err != nil {
		t.Fatalf("Open(%q): %v", name, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read %q: %v", name, err)
	}
	return b
}

func TestPDFJPEGPages(t *testing.T) {
	sizes := [][2]int{{32, 40}, {48, 24}, {16, 16}}
	var pages []pdfPageSpec
	var sources [][]byte
	for _, s := range sizes {
		o := jpegObj(t, s[0], s[1])
		sources = append(sources, o.stream)
		pages = append(pages, pdfPageSpec{images: []pdfObj{o}})
	}
	a, err := openPDFFile(t, makePDF(pages))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	if a.RandomAccess() {
		t.Error("RandomAccess = true, want false")
	}
	names := []string{"0001.jpg", "0002.jpg", "0003.jpg"}
	var got []string
	for _, e := range a.Entries() {
		got = append(got, e.Name)
		if e.Size != int64(len(sources[len(got)-1])) {
			t.Errorf("%s Size = %d, want %d", e.Name, e.Size, len(sources[len(got)-1]))
		}
	}
	if strings.Join(got, ",") != strings.Join(names, ",") {
		t.Fatalf("entries = %v, want %v", got, names)
	}

	i := 0
	err = a.Walk(func(e Entry, r io.Reader) error {
		b, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		if !bytes.Equal(b, sources[i]) {
			t.Errorf("%s: bytes differ from the embedded JPEG", e.Name)
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
		if err != nil || format != "jpeg" || cfg.Width != sizes[i][0] || cfg.Height != sizes[i][1] {
			t.Errorf("%s: decode = %v %q %dx%d, want jpeg %dx%d",
				e.Name, err, format, cfg.Width, cfg.Height, sizes[i][0], sizes[i][1])
		}
		i++
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if i != len(sizes) {
		t.Fatalf("Walk visited %d pages, want %d", i, len(sizes))
	}
	if b := readEntry(t, a, "0002.jpg"); !bytes.Equal(b, sources[1]) {
		t.Error("Open(0002.jpg) bytes differ from the embedded JPEG")
	}
	if _, err := a.Open("0009.jpg"); !errors.Is(err, ErrNotExist) {
		t.Errorf("Open(missing) err = %v, want ErrNotExist", err)
	}
}

func TestPDFFlatePageAsPNG(t *testing.T) {
	const w, h = 30, 20
	pix := gradientRGB(w, h)
	a, err := openPDFFile(t, makePDF([]pdfPageSpec{{images: []pdfObj{
		flateObj(w, h, "/DeviceRGB", pix, nil),
	}}}))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	if es := a.Entries(); len(es) != 1 || es[0].Name != "0001.png" || es[0].Size != -1 {
		t.Fatalf("entries = %+v, want one 0001.png with Size -1", es)
	}
	// Walk twice: the second pass must re-render, not reuse dropped pixels.
	for pass := 0; pass < 2; pass++ {
		err := a.Walk(func(e Entry, r io.Reader) error {
			b, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
			if err != nil || format != "png" || cfg.Width != w || cfg.Height != h {
				t.Errorf("pass %d: decode = %v %q %dx%d, want png %dx%d",
					pass, err, format, cfg.Width, cfg.Height, w, h)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("Walk pass %d: %v", pass, err)
		}
	}
	if b := readEntry(t, a, "0001.png"); len(b) == 0 {
		t.Error("Open(0001.png) returned no bytes")
	}
}

func TestPDFUnsupportedPages(t *testing.T) {
	const w, h = 8, 8
	rgb := gradientRGB(w, h)
	cmyk := bytes.Repeat([]byte{10, 20, 30, 40}, w*h)
	cases := []struct {
		name  string
		pages []pdfPageSpec
		want  string
	}{
		{"two images on a page", []pdfPageSpec{
			{images: []pdfObj{jpegObj(t, w, h)}},
			{images: []pdfObj{jpegObj(t, w, h), jpegObj(t, w+1, h)}}, // identical images would be merged by pdfcpu
		}, "PDF page 2 is not a single image"},
		{"text-only page", []pdfPageSpec{
			{images: []pdfObj{jpegObj(t, w, h)}},
			{text: true},
		}, "PDF page 2 is not a single image"},
		{"CMYK image", []pdfPageSpec{
			{images: []pdfObj{flateObj(w, h, "/DeviceCMYK", cmyk, nil)}},
		}, "PDF page 1 uses CMYK colour"},
		{"JPEG 2000 image", []pdfPageSpec{
			{images: []pdfObj{{body: fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d "+
				"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /JPXDecode", w, h), stream: rgb}}},
		}, "PDF page 1 uses an unsupported image encoding"},
		{"same image drawn twice", []pdfPageSpec{
			{images: []pdfObj{jpegObj(t, w, h), jpegObj(t, w, h)}}, // pdfcpu merges these into one object
		}, "PDF page 1 is not a single image"},
		{"DCT image with Decode array", []pdfPageSpec{
			{images: []pdfObj{withDict(jpegObj(t, w, h), "/Decode [1 0 1 0 1 0]")}},
		}, "PDF page 1 uses an unsupported image encoding"},
		{"DCT image with SMask", []pdfPageSpec{
			{images: []pdfObj{jpegWithSMask(t, w, h)}},
		}, "PDF page 1 uses an unsupported image encoding"},
		{"Flate image with Decode array", []pdfPageSpec{
			{images: []pdfObj{withDict(flateObj(w, h, "/DeviceRGB", rgb, nil), "/Decode [1 0 1 0 1 0]")}},
		}, "PDF page 1 uses an unsupported image encoding"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := openPDFFile(t, makePDF(tc.pages))
			if a != nil {
				a.Close()
			}
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("err = %v, want ErrUnsupported", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestPDFAlphaImageIsUnsupported(t *testing.T) {
	// pdfcpu's PNG output would drop the alpha channel, so an image with an
	// SMask is refused rather than shown without its transparency.
	const w, h = 8, 8
	alpha := bytes.Repeat([]byte{200}, w*h)
	mask := flateObj(w, h, "/DeviceGray", alpha, nil)
	img := flateObj(w, h, "/DeviceRGB", gradientRGB(w, h), &mask)
	a, err := openPDFFile(t, makePDF([]pdfPageSpec{{images: []pdfObj{img}}}))
	if a != nil {
		a.Close()
	}
	if !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "PDF page 1 uses an unsupported image encoding") {
		t.Fatalf("err = %v, want unsupported image encoding", err)
	}
}

func TestPDFEncrypted(t *testing.T) {
	plain := makePDF([]pdfPageSpec{{images: []pdfObj{jpegObj(t, 8, 8)}}})
	for _, tc := range []struct {
		name, user, owner string
	}{
		{"user password", "secret", "owner"},
		{"owner password only", "", "owner"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			conf := model.NewAESConfiguration(tc.user, tc.owner, 256)
			if err := api.Encrypt(context.Background(), bytes.NewReader(plain), &out, conf); err != nil {
				t.Fatalf("encrypt: %v", err)
			}
			a, err := openPDFFile(t, out.Bytes())
			if a != nil {
				a.Close()
			}
			if !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "encrypted PDF") {
				t.Fatalf("err = %v, want unsupported encrypted PDF", err)
			}
		})
	}
}

func TestPDFFormatOf(t *testing.T) {
	for _, name := range []string{"x.pdf", "x.PDF", "Sub/X.Pdf"} {
		if f, ok := FormatOf(name); !ok || f != PDF {
			t.Errorf("FormatOf(%q) = %q, %v; want pdf, true", name, f, ok)
		}
	}
	if _, ok := FormatOf("x.pdf.txt"); ok {
		t.Error("FormatOf(x.pdf.txt) ok = true, want false")
	}
}

func TestPDFWalkLimit(t *testing.T) {
	a, err := openPDFFile(t, makePDF([]pdfPageSpec{
		{images: []pdfObj{jpegObj(t, 64, 64)}},
		{images: []pdfObj{jpegObj(t, 64, 64)}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	err = WalkLimited(a, Limit{MaxBytes: 16}, func(e Entry, r io.Reader) error {
		_, err := io.Copy(io.Discard, r)
		return err
	})
	if !errors.Is(err, ErrTooMuchData) {
		t.Fatalf("err = %v, want ErrTooMuchData", err)
	}
}

func TestPDFWalkSkipRest(t *testing.T) {
	a, err := openPDFFile(t, makePDF([]pdfPageSpec{
		{images: []pdfObj{jpegObj(t, 8, 8)}},
		{images: []pdfObj{jpegObj(t, 8, 8)}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	visited := 0
	err = a.Walk(func(e Entry, r io.Reader) error {
		visited++
		return SkipRest
	})
	if err != nil || visited != 1 {
		t.Fatalf("Walk with SkipRest: visited %d, err %v; want 1, nil", visited, err)
	}
}

// withDict appends dictionary entries to an image object.
func withDict(o pdfObj, extra string) pdfObj {
	o.body += " " + extra
	return o
}

// jpegWithSMask is a JPEG image with an alpha SMask object.
func jpegWithSMask(t *testing.T, w, h int) pdfObj {
	t.Helper()
	mask := flateObj(w, h, "/DeviceGray", bytes.Repeat([]byte{200}, w*h), nil)
	o := jpegObj(t, w, h)
	o.body += " /SMask SMASK"
	o.smask = &mask
	return o
}

// lowerPDFImageLimit sets maxPDFImageBytes for the rest of the test.
func lowerPDFImageLimit(t *testing.T, n int64) {
	t.Helper()
	old := maxPDFImageBytes
	maxPDFImageBytes = n
	t.Cleanup(func() { maxPDFImageBytes = old })
}

func TestPDFFlateBombIsBounded(t *testing.T) {
	// A 1x1 RGB image whose stream inflates to 8 MiB. The decode stops at the
	// 3 bytes the dictionary declares, so the page is a valid 1x1 PNG.
	lowerPDFImageLimit(t, 1<<20)
	bomb := make([]byte, 8<<20)
	a, err := openPDFFile(t, makePDF([]pdfPageSpec{{images: []pdfObj{
		flateObj(1, 1, "/DeviceRGB", bomb, nil),
	}}}))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b := readEntry(t, a, "0001.png")
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || format != "png" || cfg.Width != 1 || cfg.Height != 1 {
		t.Fatalf("decode = %v %q %dx%d, want png 1x1", err, format, cfg.Width, cfg.Height)
	}
}

func TestPDFImageTooLarge(t *testing.T) {
	lowerPDFImageLimit(t, 1<<20)
	cases := []struct {
		name string
		img  pdfObj
	}{
		// 2000x1000 RGB is 6 MB of pixels, over the 1 MiB limit.
		{"over the byte limit", flateObj(2000, 1000, "/DeviceRGB", nil, nil)},
		// A side over maxPDFImageSide is refused on the dimensions alone.
		{"side over 20000", flateObj(20001, 1, "/DeviceGray", nil, nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := openPDFFile(t, makePDF([]pdfPageSpec{{images: []pdfObj{tc.img}}}))
			if a != nil {
				a.Close()
			}
			if !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "image is too large") {
				t.Fatalf("err = %v, want unsupported image is too large", err)
			}
		})
	}
}

func TestPDFTruncatedFlateImage(t *testing.T) {
	// The dictionary declares 100x100 RGB, but the stream holds 8 bytes.
	a, err := openPDFFile(t, makePDF([]pdfPageSpec{{images: []pdfObj{
		flateObj(100, 100, "/DeviceRGB", make([]byte, 8), nil),
	}}}))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err := a.Open("0001.png"); !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "cannot be decoded") {
		t.Fatalf("Open err = %v, want unsupported cannot be decoded", err)
	}
}

func TestPDFGrayAndIndexedImages(t *testing.T) {
	const w, h = 16, 8
	gray := make([]byte, w*h)
	for i := range gray {
		gray[i] = byte(i * 16)
	}
	idx := make([]byte, w*h)
	for i := range idx {
		idx[i] = byte(i % 2)
	}
	checkPNG := func(t *testing.T, b []byte) {
		t.Helper()
		cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
		if err != nil || format != "png" || cfg.Width != w || cfg.Height != h {
			t.Fatalf("decode = %v %q %dx%d, want png %dx%d", err, format, cfg.Width, cfg.Height, w, h)
		}
	}
	t.Run("gray", func(t *testing.T) {
		a, err := openPDFFile(t, makePDF([]pdfPageSpec{{images: []pdfObj{
			flateObj(w, h, "/DeviceGray", gray, nil),
		}}}))
		if err != nil {
			t.Fatalf("gray image: %v", err)
		}
		defer a.Close()
		checkPNG(t, readEntry(t, a, "0001.png"))
	})
	t.Run("indexed", func(t *testing.T) {
		// Indexed is either rendered as a valid PNG or refused as unsupported.
		a, err := openPDFFile(t, makePDF([]pdfPageSpec{{images: []pdfObj{
			flateObj(w, h, "[/Indexed /DeviceRGB 1 <000000ffffff>]", idx, nil),
		}}}))
		if err != nil {
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("indexed image: err = %v, want a PNG or ErrUnsupported", err)
			}
			return
		}
		defer a.Close()
		checkPNG(t, readEntry(t, a, "0001.png"))
	})
}

func TestOpenPathContextCanceled(t *testing.T) {
	path := writePDFFile(t, makePDF([]pdfPageSpec{{images: []pdfObj{jpegObj(t, 8, 8)}}}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a, _, err := OpenPathContext(ctx, PathOf(path))
	if a != nil {
		a.Close()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
