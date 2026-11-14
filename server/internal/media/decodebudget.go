package media

import (
	"bytes"
	"context"
	"image"
	"io"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/semaphore"
)

// Bytes per decoded pixel, used to weigh one full decode against the process
// budget. Measured on a 1600x2400 realistic page: a JPEG page thumbnail
// allocates about 38 MB (about 10 bytes a pixel) and the same page as AVIF
// about 193 MB (about 50 bytes a pixel, the wasm decoder). The constants round
// those figures up.
const (
	bytesPerPixel     = 12
	bytesPerPixelAVIF = 52
)

// DefaultDecodeBudget is the decode budget when the server does not set one.
const DefaultDecodeBudget = 256 << 20

// decodeBudget is one budget: its size and the semaphore that holds it.
type decodeBudget struct {
	size int64
	sem  *semaphore.Weighted
}

// decodeState is the budget in use. SetDecodeBudget replaces it at startup.
var decodeState atomic.Pointer[decodeBudget]

// decodeHolders counts the decodes that hold budget right now, and
// decodeHoldersPeak is the most that were at once. Tests read them.
var (
	decodeHolders     atomic.Int64
	decodeHoldersPeak atomic.Int64
)

func init() {
	decodeState.Store(newDecodeBudget(DefaultDecodeBudget))
}

func newDecodeBudget(n int64) *decodeBudget {
	return &decodeBudget{size: n, sem: semaphore.NewWeighted(n)}
}

// SetDecodeBudget sets the process-wide budget for full image decodes, in
// bytes. A value of zero or less keeps DefaultDecodeBudget. Call it at startup,
// before any decode.
func SetDecodeBudget(n int64) {
	if n <= 0 {
		n = DefaultDecodeBudget
	}
	decodeState.Store(newDecodeBudget(n))
}

// decodeWeight is the estimated bytes one decode of cfg (in format) needs. A
// weight over the whole budget is clamped to it, so such an image still runs,
// alone.
func decodeWeight(cfg image.Config, format string, budget int64) int64 {
	bpp := int64(bytesPerPixel)
	if format == "avif" {
		bpp = bytesPerPixelAVIF
	}
	w := int64(cfg.Width) * int64(cfg.Height) * bpp
	return min(max(w, 1), budget)
}

// acquireDecode waits until weight bytes of budget st are free, or ctx ends.
// The returned release gives them back; calling it more than once is safe.
func acquireDecode(ctx context.Context, st *decodeBudget, weight int64) (func(), error) {
	if err := st.sem.Acquire(ctx, weight); err != nil {
		return nil, err
	}
	n := decodeHolders.Add(1)
	for {
		p := decodeHoldersPeak.Load()
		if n <= p || decodeHoldersPeak.CompareAndSwap(p, n) {
			break
		}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			decodeHolders.Add(-1)
			st.sem.Release(weight)
		})
	}, nil
}

// DecodeBudgeted decodes img in full under the process decode budget. It waits
// until the budget has room for the image's estimated size, or until ctx ends.
// The caller checks the pixel limit first (CheckPixels). On success the caller
// must call release once the image is no longer used; on error release is nil.
func DecodeBudgeted(ctx context.Context, img []byte) (image.Image, func(), error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(img))
	if err != nil {
		return nil, nil, err
	}
	st := decodeState.Load()
	release, err := acquireDecode(ctx, st, decodeWeight(cfg, format, st.size))
	if err != nil {
		return nil, nil, err
	}
	var r io.Reader = bytes.NewReader(img)
	if format == "jpeg" {
		r = jpegReader(img)
	}
	src, _, err := image.Decode(r)
	if err != nil {
		release()
		return nil, nil, err
	}
	return src, release, nil
}
