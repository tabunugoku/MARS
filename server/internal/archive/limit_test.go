package archive

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
)

func openT(t *testing.T, path string) Archive {
	t.Helper()
	a, _, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

func readAll(_ Entry, r io.Reader) error {
	_, err := io.Copy(io.Discard, r)
	return err
}

func skipAll(Entry, io.Reader) error { return nil }

// A 177 KB RAR that expands to 242 MB stops at the cap.
func TestLimitCapsReads(t *testing.T) {
	a := openT(t, filepath.Join("testdata", "libarchive", "rar_ppmd_lzss_conversion.rar"))
	err := WalkLimited(a, Limit{MaxBytes: 1 << 20}, readAll)
	if !errors.Is(err, ErrTooMuchData) {
		t.Fatalf("err %v, want ErrTooMuchData", err)
	}
}

// In solid archives, entries the caller skips are still decoded and count.
func TestLimitCountsSkippedSolidData(t *testing.T) {
	for _, p := range []string{
		filepath.Join("testdata", "libarchive", "rar5_multiple_files_solid.rar"), // 7 x 4 KB
		filepath.Join("testdata", "solid.cb7"),                                   // "alpha" + "charlie"
	} {
		a := openT(t, p)
		if err := WalkLimited(a, Limit{MaxBytes: 6}, skipAll); !errors.Is(err, ErrTooMuchData) {
			t.Errorf("%s: err %v, want ErrTooMuchData", p, err)
		}
		if err := WalkLimited(a, Limit{MaxBytes: 1 << 20}, skipAll); err != nil {
			t.Errorf("%s within budget: %v", p, err)
		}
	}
}

func TestLimitContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, p := range []string{
		filepath.Join("testdata", "libarchive", "rar_compress_normal.rar"),
		filepath.Join("testdata", "solid.cb7"),
		zipFixture(t),
	} {
		a := openT(t, p)
		if err := WalkLimited(a, Limit{Ctx: ctx}, readAll); !errors.Is(err, context.Canceled) {
			t.Errorf("%s: err %v, want context.Canceled", p, err)
		}
	}
}
