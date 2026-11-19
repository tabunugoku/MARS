package db

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

var errStop = errors.New("stop")

func TestIsBusy(t *testing.T) {
	if !IsBusy(errors.New("database is locked (5) (SQLITE_BUSY)")) {
		t.Error("a locked-database error is not busy")
	}
	if IsBusy(nil) || IsBusy(errors.New("no such table: books")) {
		t.Error("an error that is not SQLITE_BUSY reads as busy")
	}
}

// within runs fn and fails the test when it does not return in time: a write
// lock left behind by a batch or unit would block the next writer for good.
func within(t *testing.T, what string, fn func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not return: the write lock is still held", what)
	}
}

// Every way a Batch or Unit ends must release the write lock: a Close, a
// Flush, and a unit whose function fails.
func TestBatchAndUnitReleaseTheWriteLock(t *testing.T) {
	ctx := context.Background()
	d, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	setting := func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO settings (key, value) VALUES ('k', '1')`)
		return err
	}
	unit := func() error { return Unit(ctx, d, setting) }

	b := &Batch{DB: d}
	if _, err := b.Tx(ctx); err != nil {
		t.Fatal(err)
	}
	b.Close()
	within(t, "a unit after a closed batch", unit)

	b = &Batch{DB: d}
	if _, err := b.Tx(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.Flush(); err != nil {
		t.Fatal(err)
	}
	within(t, "a unit after a flushed batch", unit)

	within(t, "a unit whose function fails", func() error {
		if err := Unit(ctx, d, func(*sql.Tx) error { return errStop }); !errors.Is(err, errStop) {
			return errors.New("unit error is not the function's error")
		}
		return nil
	})
	within(t, "a unit after a failed unit", unit)
}
