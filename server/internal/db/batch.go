package db

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// Batch runs a long job of small writes as a series of short write
// transactions, so other writers (a reader saving progress) wait at most
// one batch instead of the whole job. Each unit of work that must be atomic
// is written through Tx, then Step is called; Step commits once the open
// transaction has run MaxStatements units or has been open MaxDuration.
//
// A job written through Batch must be safe to stop at any commit and to
// run again: an interrupted scan is finished by the next one.
type Batch struct {
	DB            *sql.DB
	MaxStatements int           // units per transaction; 0 means 1000
	MaxDuration   time.Duration // 0 means 50ms

	tx      *sql.Tx
	begun   time.Time
	units   int
	stmts   map[string]*sql.Stmt
	Commits int           // transactions committed so far
	Longest time.Duration // longest time a transaction was open
}

// Tx returns the open transaction, beginning one if needed.
func (b *Batch) Tx(ctx context.Context) (*sql.Tx, error) {
	if b.tx == nil {
		tx, err := b.DB.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		b.tx, b.begun, b.units, b.stmts = tx, time.Now(), 0, map[string]*sql.Stmt{}
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
	if err := tx.Commit(); err != nil {
		return err
	}
	held := time.Since(b.begun)
	b.Commits++
	b.Longest = max(b.Longest, held)
	rest(held)
	return nil
}

// rest leaves the write lock free for as long as a transaction held it.
// SQLite's lock is not fair: a writer kept waiting (busy_timeout) retries
// every 100ms or so, and back-to-back batches won nearly every race, so a
// progress write waited 2.4s behind a stream of 50ms transactions.
func rest(held time.Duration) { time.Sleep(held) }

// Unit runs fn in one short write transaction on d, retrying it while the
// database is busy (see RetryBusy), then rests as Flush does. fn must be safe
// to run again. Use it for each batch of a long delete, so another writer
// waits at most one batch.
func Unit(ctx context.Context, d *sql.DB, fn func(tx *sql.Tx) error) error {
	return RetryBusy(ctx, func() error {
		tx, err := d.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		begun := time.Now()
		if err := fn(tx); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		rest(time.Since(begun))
		return nil
	})
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
}

// RetryBusy runs fn again, for up to about 30 seconds, while it fails with
// SQLITE_BUSY: a write that waited out busy_timeout behind a long writer is
// retried whole, so fn must be safe to run again (a transaction that began
// and failed, for instance). Other errors, and ctx ending, return at once.
func RetryBusy(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 1; attempt <= 6; attempt++ {
		if err = fn(); err == nil || !strings.Contains(err.Error(), "database is locked") {
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
