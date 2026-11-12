package library

import (
	"context"
	"sync"
)

// ctxMutex is a mutex whose Lock gives up when ctx ends, so a cancelled
// task or request never waits behind a slow holder.
type ctxMutex chan struct{}

func newCtxMutex() ctxMutex { return make(ctxMutex, 1) }

func (m ctxMutex) Lock(ctx context.Context) error {
	select {
	case m <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m ctxMutex) Unlock() { <-m }

// scanGate lets any number of scan syncs run at once, or one purge with
// no scan running. A waiting purge holds new scans off, so a busy server
// cannot starve it. It covers every library, including ones created while
// the purge waits.
type scanGate struct {
	once   sync.Once
	entry  ctxMutex // held by a purge; taken briefly by a scan to enter
	mu     sync.Mutex
	active int
	idle   chan struct{} // closed when active drops to 0
}

func (g *scanGate) init() { g.once.Do(func() { g.entry = newCtxMutex() }) }

// enter waits until no purge holds the gate and counts a scan in.
func (g *scanGate) enter(ctx context.Context) error {
	g.init()
	if err := g.entry.Lock(ctx); err != nil {
		return err
	}
	g.mu.Lock()
	g.active++
	g.mu.Unlock()
	g.entry.Unlock()
	return nil
}

func (g *scanGate) leave() {
	g.mu.Lock()
	g.active--
	if g.active == 0 && g.idle != nil {
		close(g.idle)
		g.idle = nil
	}
	g.mu.Unlock()
}

// pause takes the gate and waits for running scans to leave. On a
// cancelled ctx it lets go of everything it took.
func (g *scanGate) pause(ctx context.Context) (func(), error) {
	g.init()
	if err := g.entry.Lock(ctx); err != nil {
		return nil, err
	}
	for {
		g.mu.Lock()
		if g.active == 0 {
			g.mu.Unlock()
			return g.entry.Unlock, nil
		}
		if g.idle == nil {
			g.idle = make(chan struct{})
		}
		idle := g.idle
		g.mu.Unlock()
		select {
		case <-idle:
		case <-ctx.Done():
			g.entry.Unlock()
			return nil, ctx.Err()
		}
	}
}
