package auth

import (
	"sync"
	"time"
)

// Sign-in throttling policy: after limitMaxFails failures within limitWindow
// for a key, the key is blocked until limitBlock after the failure that
// reached the threshold.
const (
	limitMaxFails = 5
	limitWindow   = 15 * time.Minute
	limitBlock    = 15 * time.Minute
	limitMaxKeys  = 10000
)

// Limiter is an in-memory sign-in throttle. The zero value is ready to use.
type Limiter struct {
	mu   sync.Mutex
	keys map[string]*failState
}

type failState struct {
	fails        []time.Time // failures inside limitWindow, oldest first
	blockedUntil time.Time
	pending      int // attempts reserved by Begin and not yet settled
}

// Begin reserves an attempt for key before the expensive password check.
// Attempts still in flight count against the budget, so a burst of parallel
// requests cannot all pass before the first failure is recorded. Every
// successful Begin must be settled with End.
func (l *Limiter) Begin(key string, now time.Time) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	st := l.state(key, now)
	if now.Before(st.blockedUntil) {
		return false, st.blockedUntil.Sub(now)
	}
	st.fails = pruneTimes(st.fails, now)
	if len(st.fails)+st.pending >= limitMaxFails {
		return false, time.Second
	}
	st.pending++
	return true, 0
}

// End settles an attempt reserved by Begin. A failure is recorded; a
// success clears the key's failures.
func (l *Limiter) End(key string, failed bool, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	st := l.state(key, now)
	if st.pending > 0 {
		st.pending--
	}
	if failed {
		st.fail(now)
		return
	}
	st.fails, st.blockedUntil = nil, time.Time{}
	if st.pending == 0 {
		delete(l.keys, key)
	}
}

// state returns key's record, creating it. The caller must hold l.mu.
func (l *Limiter) state(key string, now time.Time) *failState {
	if l.keys == nil {
		l.keys = make(map[string]*failState)
	}
	st, found := l.keys[key]
	if !found {
		if len(l.keys) >= limitMaxKeys {
			l.prune(now)
		}
		st = &failState{}
		l.keys[key] = st
	}
	return st
}

func (st *failState) fail(now time.Time) {
	st.fails = append(pruneTimes(st.fails, now), now)
	// Only the failure that reaches the threshold starts the block. Failures
	// that arrive during a block (requests already in flight) do not extend it.
	if len(st.fails) == limitMaxFails {
		st.blockedUntil = now.Add(limitBlock)
	}
}

// Allow reports whether an attempt for key may proceed. When it may not,
// retryAfter says how long to wait.
func (l *Limiter) Allow(key string, now time.Time) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	st, found := l.keys[key]
	if !found || !now.Before(st.blockedUntil) {
		return true, 0
	}
	return false, st.blockedUntil.Sub(now)
}

// Fail records a failed attempt for key.
func (l *Limiter) Fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.state(key, now).fail(now)
}

// Reset clears all failures and blocks for key, for example after a successful sign-in.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.keys, key)
}

// prune drops keys that have no failures in the window and no active block.
// The caller must hold l.mu.
func (l *Limiter) prune(now time.Time) {
	for k, st := range l.keys {
		st.fails = pruneTimes(st.fails, now)
		if len(st.fails) == 0 && st.pending == 0 && !now.Before(st.blockedUntil) {
			delete(l.keys, k)
		}
	}
}

// pruneTimes keeps the failures that are still inside the window.
func pruneTimes(fails []time.Time, now time.Time) []time.Time {
	i := 0
	for i < len(fails) && now.Sub(fails[i]) >= limitWindow {
		i++
	}
	return fails[i:]
}
