package library

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCtxMutexCancel(t *testing.T) {
	m := newCtxMutex()
	if err := m.Lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := m.Lock(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting on a held lock: %v", err)
	}
	m.Unlock()
	if err := m.Lock(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestScanGate(t *testing.T) {
	var g scanGate
	bg := context.Background()
	if err := g.enter(bg); err != nil {
		t.Fatal(err)
	}

	// A purge waits for the running scan; cancelling it releases the gate.
	ctx, cancel := context.WithTimeout(bg, 20*time.Millisecond)
	defer cancel()
	if _, err := g.pause(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pause with a scan running: %v", err)
	}
	if err := g.enter(bg); err != nil { // the cancelled purge holds nothing
		t.Fatal(err)
	}
	g.leave()

	// A waiting purge holds new scans off and runs once the scan leaves.
	got := make(chan func(), 1)
	go func() {
		resume, err := g.pause(bg)
		if err != nil {
			t.Error(err)
		}
		got <- resume
	}()
	time.Sleep(20 * time.Millisecond)
	short, cancel2 := context.WithTimeout(bg, 20*time.Millisecond)
	defer cancel2()
	if err := g.enter(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("scan entered while a purge waited: %v", err)
	}
	g.leave()
	var resume func()
	select {
	case resume = <-got:
	case <-time.After(time.Second):
		t.Fatal("purge never ran")
	}
	short2, cancel3 := context.WithTimeout(bg, 20*time.Millisecond)
	defer cancel3()
	if err := g.enter(short2); err == nil {
		t.Fatal("scan entered during a purge")
	}
	resume()
	if err := g.enter(bg); err != nil {
		t.Fatal(err)
	}
	g.leave()
}
