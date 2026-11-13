package media

import (
	"bytes"
	"context"
	"image"
	"sync"
	"testing"
	"time"
)

// useBudget sets the process budget for one test and restores the default.
func useBudget(t *testing.T, n int64) {
	t.Helper()
	SetDecodeBudget(n)
	decodeHoldersPeak.Store(0)
	t.Cleanup(func() { SetDecodeBudget(DefaultDecodeBudget) })
}

func TestAcquireDecodeSerialisesOverBudget(t *testing.T) {
	useBudget(t, 1000)
	ctx := context.Background()

	first, err := acquireDecode(ctx, decodeState.Load(), 600)
	if err != nil {
		t.Fatal(err)
	}
	// The second weight does not fit beside the first: it waits.
	acquired := make(chan func(), 1)
	go func() {
		r, err := acquireDecode(ctx, decodeState.Load(), 600)
		if err != nil {
			t.Error(err)
		}
		acquired <- r
	}()
	select {
	case <-acquired:
		t.Fatal("second decode acquired while the first still holds the budget")
	case <-time.After(50 * time.Millisecond):
	}
	first()
	select {
	case second := <-acquired:
		second()
	case <-time.After(5 * time.Second):
		t.Fatal("second decode did not acquire after the first released")
	}
	if n := decodeHolders.Load(); n != 0 {
		t.Errorf("holders after both released = %d, want 0", n)
	}
	if peak := decodeHoldersPeak.Load(); peak != 1 {
		t.Errorf("peak holders = %d, want 1", peak)
	}
}

func TestAcquireDecodeReleaseIsIdempotent(t *testing.T) {
	useBudget(t, 1000)
	ctx := context.Background()
	release, err := acquireDecode(ctx, decodeState.Load(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	release()
	release() // a second call must not give back more than was taken
	tctx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err := acquireDecode(tctx, decodeState.Load(), 1001); err == nil {
		t.Fatal("weight over the budget was acquired")
	}
	fresh, cancel2 := context.WithTimeout(ctx, time.Second)
	defer cancel2()
	again, err := acquireDecode(fresh, decodeState.Load(), 1000)
	if err != nil {
		t.Fatalf("full budget after release: %v", err)
	}
	again()
}

func TestAcquireDecodeRespectsContext(t *testing.T) {
	useBudget(t, 1000)
	hold, err := acquireDecode(context.Background(), decodeState.Load(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer hold()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := acquireDecode(ctx, decodeState.Load(), 500); err == nil {
		t.Fatal("acquire succeeded with the budget full")
	}
}

func TestDecodesRunOneAtATimeOverBudget(t *testing.T) {
	page := jpegBytes(t, 64, 64)
	cfg, format, err := image.DecodeConfig(bytes.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	// The budget holds exactly one such decode.
	useBudget(t, decodeWeight(cfg, format, 1<<40))

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := Thumbnail(page, 32, 32, FormatJPEG); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if peak := decodeHoldersPeak.Load(); peak != 1 {
		t.Errorf("peak concurrent decodes = %d, want 1", peak)
	}
	if n := decodeHolders.Load(); n != 0 {
		t.Errorf("holders after all finished = %d, want 0", n)
	}
}

func TestDecodeWeightClampsToBudget(t *testing.T) {
	huge := image.Config{Width: 20000, Height: 20000}
	if got := decodeWeight(huge, "jpeg", 1<<20); got != 1<<20 {
		t.Errorf("oversized weight = %d, want the budget %d", got, 1<<20)
	}
	small := image.Config{Width: 10, Height: 10}
	if got := decodeWeight(small, "png", 1<<20); got != 10*10*bytesPerPixel {
		t.Errorf("small weight = %d, want %d", got, 10*10*bytesPerPixel)
	}
}

func TestDecodeWeightUsesAVIFFactor(t *testing.T) {
	avifPage := encodedAs(t, FormatAVIF, 64, 96)
	cfg, format, err := image.DecodeConfig(bytes.NewReader(avifPage))
	if err != nil {
		t.Fatal(err)
	}
	if format != "avif" {
		t.Fatalf("format = %q, want avif", format)
	}
	if got, want := decodeWeight(cfg, format, 1<<40), int64(64*96*bytesPerPixelAVIF); got != want {
		t.Errorf("AVIF weight = %d, want %d", got, want)
	}
	if got, want := decodeWeight(cfg, "jpeg", 1<<40), int64(64*96*bytesPerPixel); got != want {
		t.Errorf("non-AVIF weight = %d, want %d", got, want)
	}
}

func TestOversizedDecodeRunsAlone(t *testing.T) {
	// A 1000x1000 page weighs about 12 MB; the budget is 1 MB. It must still
	// decode (clamped to the whole budget), not wait forever.
	useBudget(t, 1<<20)
	page := jpegBytes(t, 1000, 1000)
	done := make(chan error, 1)
	go func() {
		_, err := Thumbnail(page, 100, 100, FormatJPEG)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("oversized decode did not finish")
	}
}
