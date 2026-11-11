package archive

import (
	"crypto/sha256"
	"io"
	"path/filepath"
	"testing"
)

// TestRealRar reads compressed RAR4 and RAR5 archives made by WinRAR (from
// libarchive's test suite, see testdata/libarchive). rardecode checks each
// entry's checksum at its end, so reading every entry without an error
// proves the decompression is right.
func TestRealRar(t *testing.T) {
	for _, name := range []string{
		"rar_compress_normal.rar",       // RAR4, LZ
		"rar_compress_best.rar",         // RAR4, best compression
		"rar_filter.rar",                // RAR4 with E8/delta filters
		"rar_ppmd_lzss_conversion.rar",  // RAR4, PPMd and LZ blocks
		"rar5_compressed.rar",           // RAR5
		"rar5_solid.rar",                // RAR5 solid
		"rar5_multiple_files_solid.rar", // RAR5 solid, several files
	} {
		t.Run(name, func(t *testing.T) {
			a, f, err := Open(filepath.Join("testdata", "libarchive", name))
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if f != CBR {
				t.Fatalf("format %s", f)
			}
			entries := a.Entries()
			if len(entries) == 0 {
				t.Fatal("no entries")
			}
			walked := map[string][32]byte{}
			err = a.Walk(func(e Entry, r io.Reader) error {
				sum, n, err := digest(r)
				if err != nil {
					return err
				}
				if e.Size >= 0 && n != e.Size {
					t.Errorf("%s: read %d bytes, header says %d", e.Name, n, e.Size)
				}
				walked[e.Name] = sum
				return nil
			})
			if err != nil {
				t.Fatalf("walk: %v", err)
			}
			if len(walked) != len(entries) {
				t.Fatalf("walked %d of %d entries", len(walked), len(entries))
			}
			// Open must give the same bytes, also for the last entry of a
			// solid archive (which decodes everything before it).
			last := entries[len(entries)-1]
			rc, err := a.Open(last.Name)
			if err != nil {
				t.Fatal(err)
			}
			sum, n, err := digest(rc)
			rc.Close()
			if err != nil || sum != walked[last.Name] {
				t.Fatalf("Open(%s): %d bytes, err %v", last.Name, n, err)
			}
			t.Logf("%d entries, last %s (%d bytes)", len(entries), last.Name, n)
		})
	}
}

func digest(r io.Reader) ([32]byte, int64, error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, n, err
}
