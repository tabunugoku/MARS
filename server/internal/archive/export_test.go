package archive

import (
	"context"
	"testing"

	"github.com/klippa-app/go-pdfium"
)

// HoldPDFRenderers takes every pdfium instance from the pool, so the next
// render has to wait, and returns the function that gives them back. For
// tests in this directory only.
func HoldPDFRenderers(t testing.TB) (release func()) {
	t.Helper()
	pool, err := pdfiumGet()
	if err != nil {
		t.Fatal(err)
	}
	var held []pdfium.Pdfium
	for i := 0; i < pdfRenderSlots; i++ {
		inst, err := pool.GetInstanceWithContext(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, inst)
	}
	return func() {
		for _, inst := range held {
			_ = inst.Close()
		}
	}
}

// MakeTextPDF returns a PDF of n pages that only draw text, so every page is
// drawn by pdfium.
func MakeTextPDF(n int) []byte {
	pages := make([]pdfPageSpec, n)
	for i := range pages {
		pages[i] = pdfPageSpec{text: true}
	}
	return makePDF(pages)
}
