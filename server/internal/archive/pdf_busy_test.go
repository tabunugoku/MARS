package archive_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tabunugoku/mars/server/internal/archive"
	"github.com/tabunugoku/mars/server/internal/media"
)

// TestPDFAnalysisWithBusyRenderers analyses a drawn PDF while every renderer
// is taken. The wait ends with the caller's deadline; the error must say the
// renderer was busy, never that the PDF is unsupported, so the library does
// not record the book as unsupported. Once a renderer is free, the same
// analysis succeeds.
func TestPDFAnalysisWithBusyRenderers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.pdf")
	if err := os.WriteFile(path, archive.MakeTextPDF(2), 0o600); err != nil {
		t.Fatal(err)
	}
	release := archive.HoldPDFRenderers(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	_, err := media.AnalyzeContext(ctx, archive.PathOf(path))
	cancel()
	release()
	if !errors.Is(err, archive.ErrRendererBusy) {
		t.Fatalf("err = %v, want ErrRendererBusy", err)
	}
	if errors.Is(err, archive.ErrUnsupported) {
		t.Fatalf("err = %v: a busy renderer must not make the PDF unsupported", err)
	}

	a, err := media.AnalyzeContext(context.Background(), archive.PathOf(path))
	if err != nil {
		t.Fatalf("analysis with a free renderer: %v", err)
	}
	if len(a.Pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(a.Pages))
	}
}

// TestPDFRenderWithBusyRenderers is the same for drawing one page, as the
// page cache does.
func TestPDFRenderWithBusyRenderers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.pdf")
	if err := os.WriteFile(path, archive.MakeTextPDF(1), 0o600); err != nil {
		t.Fatal(err)
	}
	release := archive.HoldPDFRenderers(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := archive.RenderPDFPage(ctx, archive.PathOf(path), 1)
	if !errors.Is(err, archive.ErrRendererBusy) || errors.Is(err, archive.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrRendererBusy and not ErrUnsupported", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want it to wrap the caller's deadline", err)
	}
}
