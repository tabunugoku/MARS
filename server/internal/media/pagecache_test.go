package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tabunugoku/mars/server/internal/archive"
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

// readPage opens page n through Open and returns its bytes, closing the file.
func readPage(ctx context.Context, c *PageCache, bookID, key, archivePath string, pages []Page, n int) ([]byte, error) {
	f, err := c.Open(ctx, bookID, key, archive.PathOf(archivePath), pages, n)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func TestPageCacheExtractAndHit(t *testing.T) {
	ctx := context.Background()
	path, pages, want := bookFixture(t)
	c := &PageCache{Dir: t.TempDir(), MaxBytes: 1 << 30}

	for n := 1; n <= len(pages); n++ {
		got, err := readPage(ctx, c, "book1", "v1", path, pages, n)
		if err != nil {
			t.Fatalf("Open(%d): %v", n, err)
		}
		if !bytes.Equal(got, want[n]) {
			t.Errorf("page %d bytes differ from archive entry", n)
		}
	}

	// A sentinel added after extraction must survive a hit: the second call
	// must not re-extract the book.
	sentinel := filepath.Join(c.Dir, "book1", "sentinel")
	if err := os.WriteFile(sentinel, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readPage(ctx, c, "book1", "v1", path, pages, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Errorf("sentinel gone after cache hit: %v", err)
	}

	// A different key re-extracts, which removes the sentinel.
	if _, err := readPage(ctx, c, "book1", "v2", path, pages, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Errorf("sentinel still present after key change: %v", err)
	}
}

func TestPageCacheRejectsBadInput(t *testing.T) {
	ctx := context.Background()
	path, pages, _ := bookFixture(t)
	dir := t.TempDir()
	c := &PageCache{Dir: dir, MaxBytes: 1 << 30}

	if _, err := readPage(ctx, c, "../x", "v1", path, pages, 1); err == nil {
		t.Error("traversal book id accepted")
	}
	if _, err := readPage(ctx, c, "", "v1", path, pages, 1); err == nil {
		t.Error("empty book id accepted")
	}
	if _, err := readPage(ctx, c, "book1", "v1", path, pages, 0); err == nil {
		t.Error("page 0 accepted")
	}
	if _, err := readPage(ctx, c, "book1", "v1", path, pages, len(pages)+1); err == nil {
		t.Error("page past end accepted")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("rejected calls left %d entries in cache dir", len(entries))
	}
}

func TestPageCacheEvictsLeastRecentlyUsed(t *testing.T) {
	ctx := context.Background()
	path, pages, _ := bookFixture(t)
	c := &PageCache{Dir: t.TempDir(), MaxBytes: 1}

	if _, err := readPage(ctx, c, "aaa", "v1", path, pages, 1); err != nil {
		t.Fatal(err)
	}
	// Make book aaa clearly older than the book about to be extracted.
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(c.Dir, "aaa", "key"), old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := readPage(ctx, c, "bbb", "v1", path, pages, 1); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(c.Dir, "aaa")); !os.IsNotExist(err) {
		t.Errorf("older book aaa not evicted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c.Dir, "bbb")); err != nil {
		t.Errorf("just-extracted book bbb removed: %v", err)
	}
}

func TestPageCacheEvictionSkipsLockedBook(t *testing.T) {
	ctx := context.Background()
	path, pages, _ := bookFixture(t)
	c := &PageCache{Dir: t.TempDir(), MaxBytes: 1}

	if _, err := readPage(ctx, c, "aaa", "v1", path, pages, 1); err != nil {
		t.Fatal(err)
	}
	// aaa is the oldest book, so it is the first eviction candidate.
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(c.Dir, "aaa", "key"), old, old); err != nil {
		t.Fatal(err)
	}
	// Simulate another request using aaa.
	unlock, ok := c.tryLockBook("aaa")
	if !ok {
		t.Fatal("could not lock aaa")
	}
	defer unlock()

	if _, err := readPage(ctx, c, "bbb", "v1", path, pages, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(c.Dir, "aaa")); err != nil {
		t.Errorf("locked book aaa was evicted: %v", err)
	}
}

func TestPageCacheCancelledBeforeOpenLeavesNothing(t *testing.T) {
	path, pages, _ := bookFixture(t)
	dir := t.TempDir()
	c := &PageCache{Dir: dir, MaxBytes: 1 << 30}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := readPage(ctx, c, "book1", "v1", path, pages, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	// A missing cache dir is fine too: nothing was created either way.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("cancelled Open left %d entries in cache dir", len(entries))
	}
}

func TestPageCacheSweepsStaleTempDirs(t *testing.T) {
	ctx := context.Background()
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

	if _, err := readPage(ctx, c, "book1", "v1", path, pages, 1); err != nil {
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
			b, err := readPage(context.Background(), c, "same", "v1", path, pages, 2)
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

func TestPageCacheConcurrentDifferentBooks(t *testing.T) {
	path, pages, want := bookFixture(t)
	c := &PageCache{Dir: t.TempDir(), MaxBytes: 1 << 30, MaxExtractions: 2}

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("book%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, err := readPage(context.Background(), c, id, "v1", path, pages, 1)
			if err != nil {
				errs <- fmt.Errorf("%s: %w", id, err)
				return
			}
			if !bytes.Equal(b, want[1]) {
				errs <- fmt.Errorf("%s: page 1 bytes differ", id)
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
	if len(entries) != 8 {
		t.Errorf("cache dir has %d entries, want 8 books", len(entries))
	}
}
