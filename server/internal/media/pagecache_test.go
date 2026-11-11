package media

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// bookFixture writes a sample zip and returns its path, pages and the bytes
// each page should come back with.
func bookFixture(t *testing.T) (string, []Page, map[int][]byte) {
	t.Helper()
	dir := t.TempDir()
	path, entries := sampleBook(t, dir)
	a, err := Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	want := make(map[int][]byte)
	for _, p := range a.Pages {
		want[p.Number] = entryData(entries, p.Entry)
	}
	return path, a.Pages, want
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPageCacheExtractAndHit(t *testing.T) {
	path, pages, want := bookFixture(t)
	c := &PageCache{Dir: t.TempDir(), MaxBytes: 1 << 30}

	for n := 1; n <= len(pages); n++ {
		p, err := c.Page("book1", "v1", path, pages, n)
		if err != nil {
			t.Fatalf("Page(%d): %v", n, err)
		}
		if got := readFile(t, p); !bytes.Equal(got, want[n]) {
			t.Errorf("page %d bytes differ from archive entry", n)
		}
	}

	// A sentinel added after extraction must survive a hit: the second call
	// must not re-extract the book.
	sentinel := filepath.Join(c.Dir, "book1", "sentinel")
	if err := os.WriteFile(sentinel, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Page("book1", "v1", path, pages, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Errorf("sentinel gone after cache hit: %v", err)
	}

	// A different key re-extracts, which removes the sentinel.
	if _, err := c.Page("book1", "v2", path, pages, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Errorf("sentinel still present after key change: %v", err)
	}
}

func TestPageCacheRejectsBadInput(t *testing.T) {
	path, pages, _ := bookFixture(t)
	dir := t.TempDir()
	c := &PageCache{Dir: dir, MaxBytes: 1 << 30}

	if _, err := c.Page("../x", "v1", path, pages, 1); err == nil {
		t.Error("traversal book id accepted")
	}
	if _, err := c.Page("", "v1", path, pages, 1); err == nil {
		t.Error("empty book id accepted")
	}
	if _, err := c.Page("book1", "v1", path, pages, 0); err == nil {
		t.Error("page 0 accepted")
	}
	if _, err := c.Page("book1", "v1", path, pages, len(pages)+1); err == nil {
		t.Error("page past end accepted")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("rejected calls left %d entries in cache dir", len(entries))
	}
}

func TestPageCacheEvictsLeastRecentlyUsed(t *testing.T) {
	path, pages, _ := bookFixture(t)
	c := &PageCache{Dir: t.TempDir(), MaxBytes: 1}

	if _, err := c.Page("aaa", "v1", path, pages, 1); err != nil {
		t.Fatal(err)
	}
	// Make book aaa clearly older than the book about to be extracted.
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(c.Dir, "aaa", "key"), old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Page("bbb", "v1", path, pages, 1); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(c.Dir, "aaa")); !os.IsNotExist(err) {
		t.Errorf("older book aaa not evicted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c.Dir, "bbb")); err != nil {
		t.Errorf("just-extracted book bbb removed: %v", err)
	}
}

func TestPageCacheSweepsStaleTempDirs(t *testing.T) {
	path, pages, _ := bookFixture(t)
	c := &PageCache{Dir: t.TempDir(), MaxBytes: 1 << 30}
	stale := filepath.Join(c.Dir, "old.tmp-stale")
	fresh := filepath.Join(c.Dir, "new.tmp-busy")
	for _, d := range []string{stale, fresh} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, past, past); err != nil {
		t.Fatal(err)
	}

	if _, err := c.Page("book1", "v1", path, pages, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale temp dir not removed: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("young temp dir removed: %v", err)
	}
}

func TestPageCacheConcurrentSameBook(t *testing.T) {
	path, pages, want := bookFixture(t)
	c := &PageCache{Dir: t.TempDir(), MaxBytes: 1 << 30}

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := c.Page("same", "v1", path, pages, 2)
			if err != nil {
				errs <- err
				return
			}
			b, err := os.ReadFile(p)
			if err != nil {
				errs <- err
				return
			}
			if !bytes.Equal(b, want[2]) {
				errs <- errors.New("page 2 bytes differ")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	entries, err := os.ReadDir(c.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "same" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("cache dir = %v, want only [same]", names)
	}
}
