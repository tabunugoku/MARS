package db

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"time"
)

// writeMu takes turns between the long writers (Batch and Unit) in Go instead
// of leaving them to race for SQLite's write lock. That lock is not fair: a
// writer that busy_timeout keeps waiting loses nearly every race to
// back-to-back batches. A writer releases it before it rests, so the short
// writers that do not take it (a progress write, touching a user, an API
// request) get SQLite's lock during the rest.
var writeMu sync.Mutex

// Batch runs a long job of small writes as a series of short write
// transactions, so other writers (a reader saving progress) wait at most
// one batch instead of the whole job. Each unit of work that must be atomic
// is written through Tx, then Step is called; Step commits once the open
// transaction has run MaxStatements units or has been open MaxDuration.
//
// A job written through Batch must be safe to stop at any commit and to
// run again: an interrupted scan is finished by the next one.
//
// From its first Tx until Flush or Close, a Batch holds writeMu. So a
// goroutine must not call Unit or begin another Batch while its own Batch has
// an open transaction: it would wait for a lock it holds itself.
type Batch struct {
	DB            *sql.DB
	MaxStatements int           // units per transaction; 0 means 1000
	MaxDuration   time.Duration // 0 means 50ms

	tx      *sql.Tx
	locked  bool // holds writeMu while tx is open
	begun   time.Time
	units   int
	stmts   map[string]*sql.Stmt
	Commits int           // transactions committed so far
	Longest time.Duration // longest time a transaction was open
}

// Tx returns the open transaction, beginning one if needed. The transaction
// holds writeMu until Flush or Close.
func (b *Batch) Tx(ctx context.Context) (*sql.Tx, error) {
	if b.tx == nil {
		writeMu.Lock()
		tx, err := b.DB.BeginTx(ctx, nil)
		if err != nil {
			writeMu.Unlock()
			return nil, err
		}
		b.tx, b.locked, b.begun, b.units, b.stmts = tx, true, time.Now(), 0, map[string]*sql.Stmt{}
	}
	return b.tx, nil
}

// Exec runs one statement in the open transaction, prepared once per
// transaction.
func (b *Batch) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	tx, err := b.Tx(ctx)
	if err != nil {
		return nil, err
	}
	st := b.stmts[query]
	if st == nil {
		if st, err = tx.PrepareContext(ctx, query); err != nil {
			return nil, err
		}
		b.stmts[query] = st
	}
	return st.ExecContext(ctx, args...)
}

// Step ends a unit of work and commits when the transaction is full.
func (b *Batch) Step(ctx context.Context) error {
	if b.tx == nil {
		return nil
	}
	b.units++
	max, dur := b.MaxStatements, b.MaxDuration
	if max <= 0 {
		max = 1000
	}
	if dur <= 0 {
		dur = 50 * time.Millisecond
	}
	if b.units >= max || time.Since(b.begun) >= dur {
		return b.Flush()
	}
	return nil
}

// Flush commits the open transaction, if any.
func (b *Batch) Flush() error {
	if b.tx == nil {
		return nil
	}
	tx := b.tx
	b.tx = nil
	for _, st := range b.stmts {
		st.Close()
	}
	err := tx.Commit()
	held := time.Since(b.begun)
	b.unlock()
	if err != nil {
		return err
	}
	b.Commits++
	b.Longest = max(b.Longest, held)
	rest(held)
	return nil
}

// unlock releases writeMu if this Batch holds it.
func (b *Batch) unlock() {
	if b.locked {
		b.locked = false
		writeMu.Unlock()
	}
}

// rest leaves the write lock free for as long as a transaction held it.
// SQLite's lock is not fair: a writer kept waiting (busy_timeout) retries
// every 100ms or so, and back-to-back batches won nearly every race, so a
// progress write waited 2.4s behind a stream of 50ms transactions.
func rest(held time.Duration) { time.Sleep(held) }

// Unit runs fn in one short write transaction on d, retrying it while the
// database is busy (see RetryBusy), then rests as Flush does. fn must be safe
// to run again. Use it for each batch of a long delete, so another writer
// waits at most one batch. As for Batch, a goroutine must not call Unit while
// its own Batch has an open transaction.
func Unit(ctx context.Context, d *sql.DB, fn func(tx *sql.Tx) error) error {
	return RetryBusy(ctx, func() error {
		held, err := unitTx(ctx, d, fn)
		if err != nil {
			return err
		}
		rest(held)
		return nil
	})
}

// unitTx runs fn in one transaction and commits it. It holds writeMu from the
// begin to the commit or rollback, and returns how long the transaction was
// open. The rest is left to the caller, so the lock is free while it runs.
func unitTx(ctx context.Context, d *sql.DB, fn func(tx *sql.Tx) error) (time.Duration, error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	begun := time.Now()
	if err := fn(tx); err != nil {
		tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return time.Since(begun), nil
}

// Close rolls back the open transaction, if any. Call it (deferred) on
// every path; after Flush it does nothing.
func (b *Batch) Close() {
	if b.tx != nil {
		for _, st := range b.stmts {
			st.Close()
		}
		b.tx.Rollback()
		b.tx = nil
	}
	b.unlock()
}

// IsBusy reports whether err is SQLite's "database is locked" (SQLITE_BUSY): a
// write that waited out busy_timeout behind another writer.
func IsBusy(err error) bool {
	return err != nil && strings.Contains(err.Error(), "database is locked")
}

// RetryBusy runs fn again, for up to about 30 seconds, while it fails with
// SQLITE_BUSY: a write that waited out busy_timeout behind a long writer is
// retried whole, so fn must be safe to run again (a transaction that began
// and failed, for instance). Other errors, and ctx ending, return at once.
func RetryBusy(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 1; attempt <= 6; attempt++ {
		if err = fn(); err == nil || !IsBusy(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * 2 * time.Second):
		}
	}
	return err
}
