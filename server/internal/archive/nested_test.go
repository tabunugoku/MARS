package archive

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

// nestedFile is one entry of a fixture zip.
type nestedFile struct {
	name string
	data []byte
}

// nestedZip returns a zip holding files, in order.
func nestedZip(t *testing.T, files []nestedFile) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(f.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// nestedBook writes data as a book file called name in a new temp dir.
func nestedBook(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// nestedOpen opens path through OpenPathContext. The caller closes the result.
func nestedOpen(t *testing.T, path string) Archive {
	t.Helper()
	a, _, err := OpenPathContext(context.Background(), PathOf(path))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// nestedRead walks a and returns each entry's bytes by name. An entry walked
// twice fails the test.
func nestedRead(t *testing.T, a Archive) map[string]string {
	t.Helper()
	got := make(map[string]string)
	err := a.Walk(func(e Entry, r io.Reader) error {
		b, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		if _, dup := got[e.Name]; dup {
			t.Errorf("entry %q walked twice", e.Name)
		}
		got[e.Name] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	return got
}

// nestedSizes maps each listed entry name to its size.
func nestedSizes(es []Entry) map[string]int64 {
	out := make(map[string]int64, len(es))
	for _, e := range es {
		out[e.Name] = e.Size
	}
	return out
}

func TestNestedZipInZip(t *testing.T) {
	SetTempDir(t.TempDir())
	v2 := nestedZip(t, []nestedFile{{"a.jpg", []byte("v2-a")}, {"b.jpg", []byte("v2-b")}})
	v1 := nestedZip(t, []nestedFile{{"001.jpg", []byte("v1-001")}, {"002.png", []byte("v1-002")}})
	book := nestedZip(t, []nestedFile{{"v2.zip", v2}, {"v1.zip", v1}, {"cover.jpg", []byte("cover")}})
	a := nestedOpen(t, nestedBook(t, "book.zip", book))
	defer a.Close()

	if a.RandomAccess() {
		t.Error("RandomAccess = true, want false for an archive that holds volumes")
	}

	wantSizes := map[string]int64{
		"cover.jpg": 5, "v1.zip/001.jpg": 6, "v1.zip/002.png": 6, "v2.zip/a.jpg": 4, "v2.zip/b.jpg": 4,
	}
	if got := nestedSizes(a.Entries()); !maps.Equal(got, wantSizes) {
		t.Errorf("Entries = %v, want %v", got, wantSizes)
	}

	rc, err := a.Open("v2.zip/b.jpg")
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "v2-b" {
		t.Errorf("Open(v2.zip/b.jpg) = %q, want %q", b, "v2-b")
	}

	wantBytes := map[string]string{
		"cover.jpg": "cover", "v1.zip/001.jpg": "v1-001", "v1.zip/002.png": "v1-002",
		"v2.zip/a.jpg": "v2-a", "v2.zip/b.jpg": "v2-b",
	}
	if got := nestedRead(t, a); !maps.Equal(got, wantBytes) {
		t.Errorf("Walk = %v, want %v", got, wantBytes)
	}
}

func TestNestedRarAndSevenZipInZip(t *testing.T) {
	SetTempDir(t.TempDir())
	rarVol := buildRar4([]rarFixtureEntry{
		{name: "001.jpg", data: []byte("rar-001")},
		{name: "002.jpg", data: []byte("rar-002")},
	})
	sevenVol, err := os.ReadFile(filepath.Join("testdata", "plain.cb7"))
	if err != nil {
		t.Fatal(err)
	}
	book := nestedZip(t, []nestedFile{{"x.cbr", rarVol}, {"x.cb7", sevenVol}})
	a := nestedOpen(t, nestedBook(t, "book.zip", book))
	defer a.Close()

	got := nestedRead(t, a)
	if got["x.cbr/001.jpg"] != "rar-001" || got["x.cbr/002.jpg"] != "rar-002" {
		t.Errorf("rar volume entries = %v", got)
	}

	plain, _, err := Open(filepath.Join("testdata", "plain.cb7"))
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	want := nestedRead(t, plain)
	if len(want) == 0 {
		t.Fatal("testdata/plain.cb7 has no entries")
	}
	for name, data := range want {
		if got["x.cb7/"+name] != data {
			t.Errorf("x.cb7/%s = %q, want %q", name, got["x.cb7/"+name], data)
		}
	}
	if len(got) != 2+len(want) {
		t.Errorf("got %d entries, want %d", len(got), 2+len(want))
	}
}

func TestNestedZipInRar(t *testing.T) {
	SetTempDir(t.TempDir())
	vol := nestedZip(t, []nestedFile{{"001.jpg", []byte("zip-001")}})
	book := buildRar4([]rarFixtureEntry{
		{name: "v1.zip", data: vol},
		{name: "cover.jpg", data: []byte("cover")},
	})
	a := nestedOpen(t, nestedBook(t, "book.cbr", book))
	defer a.Close()

	want := map[string]string{"cover.jpg": "cover", "v1.zip/001.jpg": "zip-001"}
	if got := nestedRead(t, a); !maps.Equal(got, want) {
		t.Errorf("Walk = %v, want %v", got, want)
	}
}

func TestNestedOnlyOneLevel(t *testing.T) {
	SetTempDir(t.TempDir())
	deep := nestedZip(t, []nestedFile{{"x.jpg", []byte("deep")}})
	v1 := nestedZip(t, []nestedFile{{"001.jpg", []byte("v1")}, {"deep.zip", deep}})
	book := nestedZip(t, []nestedFile{{"v1.zip", v1}})
	a := nestedOpen(t, nestedBook(t, "book.zip", book))
	defer a.Close()

	// deep.zip is an ordinary entry here, not expanded.
	wantSizes := map[string]int64{"v1.zip/001.jpg": 2, "v1.zip/deep.zip": int64(len(deep))}
	if got := nestedSizes(a.Entries()); !maps.Equal(got, wantSizes) {
		t.Errorf("Entries = %v, want %v", got, wantSizes)
	}
	wantBytes := map[string]string{"v1.zip/001.jpg": "v1", "v1.zip/deep.zip": string(deep)}
	if got := nestedRead(t, a); !maps.Equal(got, wantBytes) {
		t.Errorf("Walk gave %d entries, want the two above", len(got))
	}
}

func TestNoVolumeKeepsPlainArchive(t *testing.T) {
	SetTempDir(t.TempDir())
	book := nestedZip(t, []nestedFile{{"001.jpg", []byte("a")}, {"002.jpg", []byte("b")}})
	a, f, err := OpenPathContext(context.Background(), PathOf(nestedBook(t, "book.zip", book)))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	if _, ok := a.(*nestedArchive); ok {
		t.Error("an archive without volumes was wrapped")
	}
	if f != CBZ {
		t.Errorf("format = %q, want %q", f, CBZ)
	}
	if !a.RandomAccess() {
		t.Error("RandomAccess = false, want true for a plain zip")
	}
}

func TestNestedBudget(t *testing.T) {
	SetTempDir(t.TempDir())
	// Random bytes do not compress, so the volume is about as big as its data.
	payload := make([]byte, 4096)
	rand.New(rand.NewSource(1)).Read(payload)
	vol := nestedZip(t, []nestedFile{{"001.jpg", payload}})
	book := nestedZip(t, []nestedFile{{"v1.zip", vol}})
	a := nestedOpen(t, nestedBook(t, "book.zip", book))
	defer a.Close()

	err := WalkLimited(a, Limit{MaxBytes: 1024}, func(e Entry, r io.Reader) error {
		_, err := io.Copy(io.Discard, r)
		return err
	})
	if !errors.Is(err, ErrTooMuchData) {
		t.Fatalf("err = %v, want ErrTooMuchData", err)
	}
}

func TestNestedSkipRest(t *testing.T) {
	SetTempDir(t.TempDir())
	v1 := nestedZip(t, []nestedFile{{"001.jpg", []byte("a")}, {"002.jpg", []byte("b")}})
	v2 := nestedZip(t, []nestedFile{{"001.jpg", []byte("c")}})
	book := nestedZip(t, []nestedFile{{"v1.zip", v1}, {"v2.zip", v2}})
	a := nestedOpen(t, nestedBook(t, "book.zip", book))
	defer a.Close()

	calls := 0
	err := a.Walk(func(e Entry, r io.Reader) error {
		calls++
		return SkipRest
	})
	if err != nil {
		t.Fatalf("Walk = %v, want nil after SkipRest", err)
	}
	if calls != 1 {
		t.Errorf("fn ran %d times, want 1", calls)
	}
}

func TestNestedCloseRemovesSpills(t *testing.T) {
	dir := t.TempDir()
	SetTempDir(dir)
	vol := nestedZip(t, []nestedFile{{"001.jpg", []byte("a")}})
	book := nestedZip(t, []nestedFile{{"v1.zip", vol}})
	a := nestedOpen(t, nestedBook(t, "book.zip", book))

	if _, err := ListEntries(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if n := nestedTempCount(t, dir); n == 0 {
		t.Fatal("no volume was spilled to the temp dir")
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if n := nestedTempCount(t, dir); n != 0 {
		t.Errorf("%d files left in the temp dir after Close", n)
	}
}

// nestedTempCount counts the files left in dir.
func nestedTempCount(t *testing.T, dir string) int {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(es)
}

func TestNestedCorruptVolume(t *testing.T) {
	SetTempDir(t.TempDir())
	junk := bytes.Repeat([]byte("not a zip archive "), 64)
	book := nestedZip(t, []nestedFile{{"bad.zip", junk}})
	a := nestedOpen(t, nestedBook(t, "book.zip", book))
	defer a.Close()

	err := a.Walk(func(e Entry, r io.Reader) error { return nil })
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("Walk err = %v, want ErrUnsupported", err)
	}
	if _, err := ListEntries(context.Background(), a); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ListEntries err = %v, want ErrUnsupported", err)
	}
}

func TestNestedMacOSResourceForksAreNotVolumes(t *testing.T) {
	SetTempDir(t.TempDir())
	v1 := nestedZip(t, []nestedFile{{"001.jpg", []byte("v1")}})
	v2 := nestedZip(t, []nestedFile{{"002.jpg", []byte("v2")}})
	junk := []byte("not a zip archive")
	book := nestedZip(t, []nestedFile{
		{"vol1.zip", v1}, {"vol2.zip", v2}, {"__MACOSX/._vol1.zip", junk},
	})
	a := nestedOpen(t, nestedBook(t, "book.zip", book))
	defer a.Close()

	// The fork is an ordinary outer entry: listed, but never opened as a volume.
	wantSizes := map[string]int64{
		"vol1.zip/001.jpg": 2, "vol2.zip/002.jpg": 2, "__MACOSX/._vol1.zip": int64(len(junk)),
	}
	es, err := ListEntries(context.Background(), a)
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if got := nestedSizes(es); !maps.Equal(got, wantSizes) {
		t.Errorf("Entries = %v, want %v", got, wantSizes)
	}
	wantBytes := map[string]string{"vol1.zip/001.jpg": "v1", "vol2.zip/002.jpg": "v2"}
	if got := nestedRead(t, a); !maps.Equal(got, wantBytes) {
		t.Errorf("Walk = %v, want %v", got, wantBytes)
	}
}

func TestNestedVolumeComicInfoIsLeftOut(t *testing.T) {
	SetTempDir(t.TempDir())
	vol := nestedZip(t, []nestedFile{
		{"ComicInfo.xml", []byte("<ComicInfo>vol1</ComicInfo>")}, {"001.jpg", []byte("v1")},
	})
	book := nestedZip(t, []nestedFile{
		{"vol1.zip", vol}, {"ComicInfo.xml", []byte("<ComicInfo>book</ComicInfo>")},
	})
	a := nestedOpen(t, nestedBook(t, "book.zip", book))
	defer a.Close()

	// A volume's ComicInfo.xml describes that volume, not the book, so only the
	// outer one is listed and walked.
	wantSizes := map[string]int64{"ComicInfo.xml": int64(len("<ComicInfo>book</ComicInfo>")), "vol1.zip/001.jpg": 2}
	if got := nestedSizes(a.Entries()); !maps.Equal(got, wantSizes) {
		t.Errorf("Entries = %v, want %v", got, wantSizes)
	}
	wantBytes := map[string]string{"ComicInfo.xml": "<ComicInfo>book</ComicInfo>", "vol1.zip/001.jpg": "v1"}
	if got := nestedRead(t, a); !maps.Equal(got, wantBytes) {
		t.Errorf("Walk = %v, want %v", got, wantBytes)
	}
}

// countingArchive counts the full walks of the archive it wraps.
type countingArchive struct {
	Archive
	walks int
}

func (c *countingArchive) Walk(fn func(e Entry, r io.Reader) error) error {
	c.walks++
	return c.Archive.Walk(fn)
}

func TestNestedSolidOuterIsNotWalkedTwice(t *testing.T) {
	SetTempDir(t.TempDir())
	vol := nestedZip(t, []nestedFile{{"001.jpg", []byte("a")}})
	book := nestedZip(t, []nestedFile{{"v1.zip", vol}, {"v2.zip", vol}})
	outer, _, err := openPlain(context.Background(), PathOf(nestedBook(t, "book.zip", book)))
	if err != nil {
		t.Fatal(err)
	}
	counted := &countingArchive{Archive: outer}
	w, ok := newNestedArchive(context.Background(), counted)
	if !ok {
		t.Fatal("outer archive with volumes was not wrapped")
	}
	defer w.Close()

	if _, err := ListEntries(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	if counted.walks != 1 {
		t.Fatalf("listing walked the outer archive %d times, want 1", counted.walks)
	}
	counted.walks = 0
	wantBytes := map[string]string{"v1.zip/001.jpg": "a", "v2.zip/001.jpg": "a"}
	if got := nestedRead(t, w); !maps.Equal(got, wantBytes) {
		t.Errorf("Walk = %v, want %v", got, wantBytes)
	}
	if counted.walks != 0 {
		t.Errorf("walk after the listing read the outer archive %d times, want 0", counted.walks)
	}
}

func TestNestedHeaderSkipCheap(t *testing.T) {
	SetTempDir(t.TempDir())
	vol := nestedZip(t, []nestedFile{{"001.jpg", []byte("a")}})
	zipBook := nestedZip(t, []nestedFile{{"v1.zip", vol}})
	zipIn := nestedOpen(t, nestedBook(t, "zip-in-zip.zip", zipBook))
	defer zipIn.Close()
	if !HeaderSkipCheap(zipIn) {
		t.Error("HeaderSkipCheap = false for a zip holding CBZ volumes, want true")
	}

	rarVol := buildRar4([]rarFixtureEntry{{name: "001.jpg", data: []byte("rar-001")}})
	rarBook := nestedZip(t, []nestedFile{{"x.cbr", rarVol}})
	rarIn := nestedOpen(t, nestedBook(t, "rar-in-zip.zip", rarBook))
	defer rarIn.Close()
	if HeaderSkipCheap(rarIn) {
		t.Error("HeaderSkipCheap = true for a zip holding a RAR volume, want false")
	}
}

func TestNestedHeaderOnlyWalk(t *testing.T) {
	SetTempDir(t.TempDir())
	v1 := nestedZip(t, []nestedFile{{"001.jpg", []byte("first-volume-page")}, {"002.jpg", []byte("second-page")}})
	v2 := nestedZip(t, []nestedFile{{"001.jpg", []byte("v2-page")}})
	book := nestedZip(t, []nestedFile{{"v1.zip", v1}, {"v2.zip", v2}})
	a := nestedOpen(t, nestedBook(t, "book.zip", book))
	defer a.Close()

	// fn reads only the first two bytes of every page. The rest of each zip
	// entry is left unread, and the walk must still reach every entry.
	var names []string
	err := a.Walk(func(e Entry, r io.Reader) error {
		head := make([]byte, 2)
		if _, err := io.ReadFull(r, head); err != nil {
			return err
		}
		names = append(names, e.Name+"="+string(head))
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	want := []string{"v1.zip/001.jpg=fi", "v1.zip/002.jpg=se", "v2.zip/001.jpg=v2"}
	if !slices.Equal(names, want) {
		t.Errorf("walked %v, want %v", names, want)
	}
}

func TestNestedSpillSlotsBoundConcurrentSpills(t *testing.T) {
	SetTempDir(t.TempDir())
	// Fill both slots as if two other books were spilling.
	spillSlots <- struct{}{}
	spillSlots <- struct{}{}
	var once sync.Once
	releaseSlots := func() {
		once.Do(func() {
			<-spillSlots
			<-spillSlots
		})
	}
	defer releaseSlots()

	vol := nestedZip(t, []nestedFile{{"001.jpg", []byte("a")}})
	book := nestedZip(t, []nestedFile{{"v1.zip", vol}})
	path := nestedBook(t, "book.zip", book)

	// The wrapper's context is the one OpenPathContext stores, so a short
	// deadline bounds the wait for a slot.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	blocked, _, err := OpenPathContext(ctx, PathOf(path))
	if err != nil {
		t.Fatal(err)
	}
	defer blocked.Close()
	if _, err := ListEntries(context.Background(), blocked); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ListEntries with both slots taken: err = %v, want context.DeadlineExceeded", err)
	}

	// With the slots free again, a new book spills and lists.
	releaseSlots()
	fresh := nestedOpen(t, path)
	defer fresh.Close()
	es, err := ListEntries(context.Background(), fresh)
	if err != nil {
		t.Fatalf("ListEntries after release: %v", err)
	}
	if len(es) != 1 || es[0].Name != "v1.zip/001.jpg" {
		t.Errorf("Entries = %v, want v1.zip/001.jpg", es)
	}
}
