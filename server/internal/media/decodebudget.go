package media

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/semaphore"
)

// DefaultDecodeBudget is the decode budget when the server does not set one.
const DefaultDecodeBudget = 256 << 20

// DefaultDecodeLimit is the most memory one decode may need when the server
// does not set one. See SetDecodeLimit.
const DefaultDecodeLimit = 1 << 30

// decodeBudget is one budget: its size and the semaphore that holds it.
type decodeBudget struct {
	size int64
	sem  *semaphore.Weighted
}

// decodeState is the budget in use. SetDecodeBudget replaces it at startup.
var decodeState atomic.Pointer[decodeBudget]

// decodeLimit is the most memory one decode may need, in bytes, as estimated
// by decodeCost. SetDecodeLimit replaces it at startup.
var decodeLimit atomic.Int64

// decodeHolders counts the decodes that hold budget right now, and
// decodeHoldersPeak is the most that were at once. Tests read them.
var (
	decodeHolders     atomic.Int64
	decodeHoldersPeak atomic.Int64
)

func init() {
	decodeState.Store(newDecodeBudget(DefaultDecodeBudget))
	decodeLimit.Store(DefaultDecodeLimit)
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

// SetDecodeLimit sets the most memory one decode may need, in bytes, as
// estimated from the image header (decodeCost). An image estimated above it is
// not decoded: it fails with ErrDecodeTooLarge, before any wait for the budget.
// A value of zero or less keeps DefaultDecodeLimit. Call it at startup, before
// any decode.
func SetDecodeLimit(n int64) {
	if n <= 0 {
		n = DefaultDecodeLimit
	}
	decodeLimit.Store(n)
}

// decodeWeight is the budget weight of a decode that needs cost bytes: cost,
// at least 1 and at most the whole budget. A cost over the budget is clamped to
// it, so such an image still runs, alone.
func decodeWeight(cost, budget int64) int64 {
	return min(max(cost, 1), budget)
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

// DecodeBudgeted decodes img in full under the process decode budget, for a
// caller that encodes the result as PNG (the Komga page conversion). It first
// estimates the peak memory of the decode and the PNG encoder's row buffers
// (decodeCost, usePNG); the budget is held for both until release. An image over the decode limit (SetDecodeLimit) fails with ErrDecodeTooLarge,
// which also matches ErrTooLarge, without waiting for the budget. Otherwise it
// waits until the budget has room for the estimate, or until ctx ends. The
// caller checks the pixel limit first (CheckPixels). On success the caller must
// call release once the image is no longer used; on error release is nil.
func DecodeBudgeted(ctx context.Context, img []byte) (image.Image, func(), error) {
	return decodeFor(ctx, img, usePNG, image.Point{})
}

// decodeFor is DecodeBudgeted for a given use: it checks the estimated cost
// against the decode limit, then waits for the budget and decodes. thumb, when
// not zero, is the thumbnail bounds the image will be scaled into, whose
// scaler buffers count too (scaleCost).
func decodeFor(ctx context.Context, img []byte, use decodeUse, thumb image.Point) (image.Image, func(), error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(img))
	if err != nil {
		return nil, nil, err
	}
	cost := decodeCost(img, format, cfg, use) + scaleCost(img, format, cfg, thumb.X, thumb.Y)
	if limit := decodeLimit.Load(); cost > limit {
		return nil, nil, fmt.Errorf("%w: needs about %d MiB, limit %d MiB", ErrDecodeTooLarge, cost>>20, limit>>20)
	}
	st := decodeState.Load()
	release, err := acquireDecode(ctx, st, decodeWeight(cost, st.size))
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
