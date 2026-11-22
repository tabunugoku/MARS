package library

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tabunugoku/mars/server/internal/task"
)

// A library delete requested right after the admin cancels the running delete
// task must not be folded into the cancelled task: it gets a task of its own,
// and its library is removed by that task, not left until the next restart.

// TestDeleteAfterCancelGetsNewTask cancels a drain that is blocked part way
// through library a's books, then requests library b. The request must come
// back as a new task, and b must be gone once that task finishes.
func TestDeleteAfterCancelGetsNewTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := openDeleteTestDB(t)
	seedDeleteLibrary(t, conn, "a", 1, 2, 2)
	seedDeleteLibrary(t, conn, "b", 0, 1, 2)
	q := task.New(conn, 2)
	if err := q.Run(ctx); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: conn, Queue: q, deleteBatch: 1}
	// Block the drain inside a's delete, after its first committed batch.
	blocked := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	s.deleteHook = func() {
		once.Do(func() {
			close(blocked)
			<-release
		})
	}
	first, err := s.QueueDelete(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-blocked:
	case <-time.After(10 * time.Second):
		t.Fatal("the drain never reached a batch boundary")
	}

	if err := q.Cancel(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`UPDATE libraries SET deleting = 1 WHERE id = 'b'`); err != nil {
		t.Fatal(err)
	}
	second, err := s.QueueDelete(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatalf("delete of b folded into the cancelled task %s", first.ID)
	}
	close(release)

	waitUntil(t, "the new delete task to end", func() bool {
		return countRows(t, conn, `SELECT count(*) FROM tasks WHERE id = ? AND state IN ('done', 'failed', 'cancelled')`, second.ID) == 1
	})
	waitUntil(t, "the cancelled task to end", func() bool {
		return countRows(t, conn, `SELECT count(*) FROM tasks WHERE id = ? AND state = 'cancelled'`, first.ID) == 1
	})
	if n := countRows(t, conn, `SELECT count(*) FROM tasks WHERE id = ? AND state = 'done'`, second.ID); n != 1 {
		t.Errorf("the new delete task did not finish as done")
	}
	for _, c := range []struct {
		what, query string
	}{
		{"library b row", `SELECT count(*) FROM libraries WHERE id = 'b'`},
		{"books of b", `SELECT count(*) FROM books WHERE library_id = 'b'`},
		{"series of b", `SELECT count(*) FROM series WHERE library_id = 'b'`},
	} {
		if n := countRows(t, conn, c.query); n != 0 {
			t.Errorf("%s left after the new task: %d", c.what, n)
		}
	}
}

// TestDeleteAfterCancelBetweenLibraries cancels the drain while it is between
// its last check and its final one (drainIdle), then requests library b. The
// same rule holds: b gets a new task and is removed by it.
func TestDeleteAfterCancelBetweenLibraries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := openDeleteTestDB(t)
	seedDeleteLibrary(t, conn, "a", 1, 1, 1)
	seedDeleteLibrary(t, conn, "b", 0, 1, 1)
	q := task.New(conn, 2)
	if err := q.Run(ctx); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: conn, Queue: q}
	idle := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	s.drainIdle = func() {
		once.Do(func() {
			close(idle)
			<-release
		})
	}
	first, err := s.QueueDelete(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-idle:
	case <-time.After(10 * time.Second):
		t.Fatal("the drain never reached its final check")
	}

	if err := q.Cancel(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`UPDATE libraries SET deleting = 1 WHERE id = 'b'`); err != nil {
		t.Fatal(err)
	}
	second, err := s.QueueDelete(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatalf("delete of b folded into the cancelled task %s", first.ID)
	}
	close(release)

	waitUntil(t, "the new delete task to end", func() bool {
		return countRows(t, conn, `SELECT count(*) FROM tasks WHERE id = ? AND state IN ('done', 'failed', 'cancelled')`, second.ID) == 1
	})
	if n := countRows(t, conn, `SELECT count(*) FROM libraries WHERE id = 'b'`); n != 0 {
		t.Errorf("library b left after the new task: %d rows", n)
	}
}
