package archive

import (
	"context"
	"errors"
	"io"
)

// Limit bounds the work one walk may do. A small hostile archive can expand
// to gigabytes, and in solid RAR and 7z even the entries a caller skips must
// be decompressed, so the budget counts skipped bytes too.
type Limit struct {
	// Ctx stops the walk with Ctx.Err() once it ends. Nil means never.
	Ctx context.Context
	// MaxBytes caps the decompressed bytes read or skipped over the whole
	// walk. Zero means no cap.
	MaxBytes int64
}

// ErrTooMuchData means a walk went over Limit.MaxBytes.
var ErrTooMuchData = errors.New("archive: decompressed data over the limit")

// WalkLimited is Walk under lim.
func WalkLimited(a Archive, lim Limit, fn func(e Entry, r io.Reader) error) error {
	l, ok := a.(limitedWalker)
	if !ok {
		return a.Walk(fn)
	}
	return l.walkLimited(newBudget(lim), fn)
}

type limitedWalker interface {
	walkLimited(b *budget, fn func(e Entry, r io.Reader) error) error
}

type budget struct {
	ctx    context.Context
	left   int64
	capped bool
}

func newBudget(lim Limit) *budget {
	ctx := lim.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return &budget{ctx: ctx, left: lim.MaxBytes, capped: lim.MaxBytes > 0}
}

// reader counts what r yields against b and checks b's context on every read.
func (b *budget) reader(r io.Reader) io.Reader { return budgetReader{r, b} }

// drain decompresses and discards the rest of r within the budget.
func (b *budget) drain(r io.Reader) error {
	_, err := io.Copy(io.Discard, b.reader(r))
	return err
}

type budgetReader struct {
	r io.Reader
	b *budget
}

func (br budgetReader) Read(p []byte) (int, error) {
	if err := br.b.ctx.Err(); err != nil {
		return 0, err
	}
	if br.b.capped {
		if br.b.left < 0 {
			return 0, ErrTooMuchData
		}
		// One byte past the budget is enough to know it was exceeded.
		if int64(len(p)) > br.b.left+1 {
			p = p[:br.b.left+1]
		}
	}
	n, err := br.r.Read(p)
	br.b.left -= int64(n)
	if br.b.capped && br.b.left < 0 {
		return n, ErrTooMuchData
	}
	return n, err
}
