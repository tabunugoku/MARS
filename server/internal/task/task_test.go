package task

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tabunugoku/mars/server/internal/db"
)

func newQueue(t *testing.T, workers int) (*Queue, context.Context) {
	t.Helper()
	d, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	// tasks.library_id references libraries.
	for _, id := range []string{"L0", "L1"} {
		if _, err := d.Exec(`INSERT INTO libraries (id, name, root, kind, created_at) VALUES (?, ?, ?, 'manga', '')`, id, id, "/"+id); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	q := New(d, workers)
	if err := q.Run(ctx); err != nil {
		t.Fatal(err)
	}
	return q, ctx
}

func waitState(t *testing.T, q *Queue, id, want string) Task {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		tk, err := q.get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if tk.State == want {
			return tk
		}
		time.Sleep(10 * time.Millisecond)
	}
	tk, _ := q.get(context.Background(), id)
	t.Fatalf("task %s state %q, want %q", id, tk.State, want)
	return tk
}

func TestRunsAndFinishes(t *testing.T) {
	q, ctx := newQueue(t, 2)
	tk, err := q.Submit(ctx, "scan", Text{English: "x"}, "", func(ctx context.Context, p func(float64)) error {
		p(0.5)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := waitState(t, q, tk.ID, "done")
	if got.Progress != 1 || got.StartedAt == nil || got.FinishedAt == nil {
		t.Fatalf("%+v", got)
	}
}

func TestCancelRunningAndQueued(t *testing.T) {
	q, ctx := newQueue(t, 1)
	started := make(chan struct{})
	running, err := q.Submit(ctx, "scan", Text{English: "a"}, "L1", func(ctx context.Context, _ func(float64)) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	queued, _ := q.Submit(ctx, "analyze", Text{English: "b"}, "L1", func(ctx context.Context, _ func(float64)) error {
		t.Error("queued task ran after cancel")
		return nil
	})
	if err := q.Cancel(ctx, queued.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.Cancel(ctx, running.ID); err != nil {
		t.Fatal(err)
	}
	waitState(t, q, running.ID, "cancelled")
	waitState(t, q, queued.ID, "cancelled")
	time.Sleep(50 * time.Millisecond) // give the worker a chance to (wrongly) run it
}

func TestDedupQueued(t *testing.T) {
	q, ctx := newQueue(t, 1)
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	started := make(chan struct{})
	q.Submit(ctx, "scan", Text{English: "busy"}, "L0", func(ctx context.Context, _ func(float64)) error {
		close(started)
		<-block
		return nil
	})
	<-started
	a, _ := q.Submit(ctx, "scan", Text{English: "s"}, "L1", func(context.Context, func(float64)) error { return nil })
	b, _ := q.Submit(ctx, "scan", Text{English: "s"}, "L1", func(context.Context, func(float64)) error { return nil })
	if a.ID != b.ID {
		t.Fatalf("not deduplicated: %s %s", a.ID, b.ID)
	}
}

func TestFailureAndPanic(t *testing.T) {
	q, ctx := newQueue(t, 1)
	tk, _ := q.Submit(ctx, "scan", Text{English: "p"}, "", func(context.Context, func(float64)) error { panic("boom") })
	got := waitState(t, q, tk.ID, "failed")
	if got.Error == "" {
		t.Fatal("no error text")
	}
	if _, err := q.get(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("got %v", err)
	}
}

// A request for a key that is running in the same library waits: it becomes
// one queued task that starts only after the running one ends, and further
// requests fold into it. Another library is not held back.
func TestSameKeyWaitsForRunning(t *testing.T) {
	q, ctx := newQueue(t, 3)
	release := make(chan struct{})
	var aDone atomic.Bool
	a, err := q.SubmitKey(ctx, "analyze", "analyze", Text{English: "a"}, "L1", func(ctx context.Context, _ func(float64)) error {
		<-release
		aDone.Store(true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, q, a.ID, "running")

	var bRuns atomic.Int32
	var bAfterA atomic.Bool
	follow := func(ctx context.Context, _ func(float64)) error {
		bRuns.Add(1)
		bAfterA.Store(aDone.Load())
		return nil
	}
	b, err := q.SubmitKey(ctx, "analyze", "analyze", Text{English: "b"}, "L1", follow)
	if err != nil {
		t.Fatal(err)
	}
	c, err := q.SubmitKey(ctx, "analyze", "analyze", Text{English: "c"}, "L1", follow)
	if err != nil {
		t.Fatal(err)
	}
	if b.ID == a.ID || c.ID != b.ID {
		t.Fatalf("ids a=%s b=%s c=%s: want one follow-up shared by b and c", a.ID, b.ID, c.ID)
	}
	other, _ := q.SubmitKey(ctx, "analyze", "analyze", Text{English: "o"}, "L0", func(context.Context, func(float64)) error { return nil })
	waitState(t, q, other.ID, "done")

	time.Sleep(100 * time.Millisecond) // the follow-up must not start meanwhile
	if got := waitState(t, q, b.ID, "queued"); got.StartedAt != nil {
		t.Fatalf("follow-up started while the first ran")
	}
	close(release)
	waitState(t, q, a.ID, "done")
	waitState(t, q, b.ID, "done")
	if n := bRuns.Load(); n != 1 {
		t.Fatalf("follow-up ran %d times, want 1", n)
	}
	if !bAfterA.Load() {
		t.Fatal("follow-up ran before the first ended")
	}
}

// Cancelling a held follow-up keeps it from running when the first ends.
func TestCancelHeldFollowUp(t *testing.T) {
	q, ctx := newQueue(t, 2)
	release := make(chan struct{})
	a, _ := q.SubmitKey(ctx, "scan", "scan", Text{English: "a"}, "L1", func(context.Context, func(float64)) error {
		<-release
		return nil
	})
	waitState(t, q, a.ID, "running")
	var ran atomic.Bool
	b, _ := q.SubmitKey(ctx, "scan", "scan", Text{English: "b"}, "L1", func(context.Context, func(float64)) error {
		ran.Store(true)
		return nil
	})
	if err := q.Cancel(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	waitState(t, q, b.ID, "cancelled")
	// A new request after the cancel gets a fresh follow-up, which runs.
	c, _ := q.SubmitKey(ctx, "scan", "scan", Text{English: "c"}, "L1", func(context.Context, func(float64)) error { return nil })
	if c.ID == b.ID {
		t.Fatal("cancelled follow-up was reused")
	}
	q.mu.Lock()
	_, stale := q.live[b.ID]
	q.mu.Unlock()
	if stale {
		t.Fatal("replaced follow-up left in the live set")
	}
	close(release)
	waitState(t, q, c.ID, "done")
	time.Sleep(50 * time.Millisecond)
	if ran.Load() {
		t.Fatal("cancelled follow-up ran")
	}
	if tk := waitState(t, q, b.ID, "cancelled"); tk.State != "cancelled" {
		t.Fatal(tk.State)
	}
}
