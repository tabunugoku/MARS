// Package task runs background work (scans, analysis) on a fixed number of
// workers. Every task has a row in the tasks table so the admin screen can
// list and cancel it; the work itself is a closure held in memory, so tasks
// do not survive a restart (startup marks leftovers as failed or cancelled).
package task

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/tabunugoku/mars/server/internal/events"
	"github.com/tabunugoku/mars/server/internal/ids"
)

// Func is the work. It reports progress in 0..1 and must return promptly
// once ctx is cancelled.
type Func func(ctx context.Context, progress func(float64)) error

// Task is one row of the tasks table.
type Task struct {
	ID         string
	Kind       string // scan | analyze | thumbnail | trash
	Label      string // final text, or the English text when LabelCode is ""
	LabelCode  string // catalog code the API renders Label from
	LabelArg   string
	LibraryID  string // "" when not tied to a library
	State      string // queued | running | done | failed | cancelled
	Progress   float64
	Error      string // final text, or the English text when ErrorCode is ""
	ErrorCode  string // catalog code the API renders Error from
	ErrorArg   string
	CreatedAt  time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time
}

var ErrNotFound = errors.New("task: not found")

// Text is a task label: Code names a catalog entry the API renders in the
// request's language, filled with Arg; English is stored for logs and for
// rows without a code.
type Text struct {
	Code    string
	Arg     string
	English string
}

// CodedError is a task failure the API renders from a catalog code. Its
// Error text is English, for logs and the stored error.
type CodedError struct {
	Code string
	Arg  string
	Err  error
}

func (e *CodedError) Error() string { return e.Err.Error() }
func (e *CodedError) Unwrap() error { return e.Err }

// taskCols lists the columns scanTask reads, in its order.
const taskCols = `id, kind, label, COALESCE(library_id, ''), state, progress, error, created_at, started_at, finished_at, label_code, label_arg, error_code, error_arg`

type job struct {
	id     string
	kind   string
	key    string // what Submit deduplicates on, with libID
	libID  string
	fn     Func
	ctx    context.Context
	cancel context.CancelFunc
}

// Queue owns the workers. Create it with New, then call Run.
type Queue struct {
	db      *sql.DB
	workers int
	jobs    chan *job

	mu   sync.Mutex
	live map[string]*job // queued or running, by task id

	// Events, when set (before Run), receives a "task" event with the Task
	// as Data whenever a task is queued, starts, progresses or ends.
	Events *events.Hub
}

// publish sends the task's current row as an event.
func (q *Queue) publish(ctx context.Context, id string) {
	if q.Events == nil {
		return
	}
	if t, err := q.get(ctx, id); err == nil {
		q.publishTask(t)
	}
}

func (q *Queue) publishTask(t Task) {
	q.Events.Publish(events.Event{Type: "task", AdminOnly: true, LibraryID: t.LibraryID, Data: t})
}

// New makes a queue with the given number of workers (at least 1).
func New(db *sql.DB, workers int) *Queue {
	if workers < 1 {
		workers = 1
	}
	return &Queue{db: db, workers: workers, jobs: make(chan *job, 1024), live: map[string]*job{}}
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// Run marks tasks left over from a previous process, then starts the
// workers. It returns at once; workers stop when ctx is cancelled.
func (q *Queue) Run(ctx context.Context) error {
	if _, err := q.db.ExecContext(ctx,
		`UPDATE tasks SET state = 'failed', error = 'Interrupted by a server restart', error_code = 'task.interrupted_restart', finished_at = ? WHERE state = 'running'`, now()); err != nil {
		return err
	}
	if _, err := q.db.ExecContext(ctx,
		`UPDATE tasks SET state = 'cancelled', finished_at = ? WHERE state = 'queued'`, now()); err != nil {
		return err
	}
	for range q.workers {
		go q.worker(ctx)
	}
	return nil
}

// Submit queues fn. When a task of the same kind for the same library is
// still queued (not yet running), that task is returned instead, so repeated
// scan requests do not pile up.
func (q *Queue) Submit(ctx context.Context, kind string, label Text, libraryID string, fn Func) (Task, error) {
	return q.SubmitKey(ctx, kind, kind, label, libraryID, fn)
}

// SubmitKey is Submit that deduplicates on key instead of kind, for work of
// one kind that must not be folded into another queued task of that kind
// (a deep analysis into a normal one, one series' thumbnails into another's).
func (q *Queue) SubmitKey(ctx context.Context, kind, key string, label Text, libraryID string, fn Func) (Task, error) {
	q.mu.Lock()
	for _, j := range q.live {
		if j.key == key && j.libID == libraryID && libraryID != "" {
			if t, err := q.get(ctx, j.id); err == nil && t.State == "queued" {
				q.mu.Unlock()
				return t, nil
			}
		}
	}
	jctx, cancel := context.WithCancel(context.Background())
	j := &job{id: ids.New(), kind: kind, key: key, libID: libraryID, fn: fn, ctx: jctx, cancel: cancel}
	var lib any
	if libraryID != "" {
		lib = libraryID
	}
	if _, err := q.db.ExecContext(ctx,
		`INSERT INTO tasks (id, kind, label, label_code, label_arg, library_id, state, created_at) VALUES (?, ?, ?, ?, ?, ?, 'queued', ?)`,
		j.id, kind, label.English, label.Code, label.Arg, lib, now()); err != nil {
		q.mu.Unlock()
		cancel()
		return Task{}, err
	}
	q.live[j.id] = j
	q.mu.Unlock()
	q.publish(ctx, j.id)

	select {
	case q.jobs <- j:
	default:
		// The buffer is full: hand it over without blocking the caller.
		go func() { q.jobs <- j }()
	}
	return q.get(ctx, j.id)
}

// Cancel stops a queued or running task. Cancelling a finished task is a
// no-op; an unknown id is ErrNotFound.
func (q *Queue) Cancel(ctx context.Context, id string) error {
	q.mu.Lock()
	j := q.live[id]
	q.mu.Unlock()
	if j == nil {
		if _, err := q.get(ctx, id); err != nil {
			return err
		}
		return nil
	}
	j.cancel()
	// A queued task never reaches a worker's run; mark it here. A running
	// task is marked by its worker when fn returns.
	res, err := q.db.ExecContext(ctx,
		`UPDATE tasks SET state = 'cancelled', finished_at = ? WHERE id = ? AND state = 'queued'`, now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		q.publish(ctx, id)
	}
	return nil
}

func (q *Queue) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-q.jobs:
			q.run(ctx, j)
		}
	}
}

func (q *Queue) run(parent context.Context, j *job) {
	defer func() {
		q.mu.Lock()
		delete(q.live, j.id)
		q.mu.Unlock()
		j.cancel()
	}()
	if j.ctx.Err() != nil {
		return // cancelled while queued; Cancel already marked it
	}
	db := context.WithoutCancel(parent)
	res, err := q.execRetry(db,
		`UPDATE tasks SET state = 'running', started_at = ? WHERE id = ? AND state = 'queued'`, now(), j.id)
	if err != nil {
		// The row stays 'queued' with nothing behind it; record that.
		slog.Error("task start", "id", j.id, "err", err)
		q.execRetry(db, `UPDATE tasks SET state = 'failed', error = 'Could not start', error_code = 'task.start_failed', finished_at = ? WHERE id = ?`, now(), j.id)
		q.publish(db, j.id)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return
	}
	// The row as it started; progress events change only Progress.
	started, startErr := q.get(db, j.id)
	if startErr == nil && q.Events != nil {
		q.publishTask(started)
	}

	// Stop the work when the server shuts down, too.
	ctx, stop := context.WithCancel(j.ctx)
	defer stop()
	go func() {
		select {
		case <-parent.Done():
			stop()
		case <-ctx.Done():
		}
	}()

	var last time.Time
	progress := func(p float64) {
		// Throttle writes: progress is cosmetic.
		if time.Since(last) < 500*time.Millisecond && p < 1 {
			return
		}
		last = time.Now()
		p = min(max(p, 0), 1)
		if _, err := q.db.ExecContext(db, `UPDATE tasks SET progress = ? WHERE id = ?`, p, j.id); err != nil {
			slog.Warn("task progress", "id", j.id, "err", err)
		}
		if startErr == nil && q.Events != nil {
			t := started
			t.Progress = p
			q.publishTask(t)
		}
	}

	err = safeRun(ctx, j.fn, progress)
	state, msg, code, arg := "done", "", "", ""
	switch {
	case err == nil:
	case ctx.Err() != nil:
		state = "cancelled"
	default:
		state, msg = "failed", err.Error()
		var ce *CodedError
		if errors.As(err, &ce) {
			code, arg = ce.Code, ce.Arg
		}
		slog.Error("task failed", "id", j.id, "kind", j.kind, "err", err)
	}
	if _, err := q.execRetry(db,
		`UPDATE tasks SET state = ?, error = ?, error_code = ?, error_arg = ?, progress = CASE WHEN ? = 'done' THEN 1 ELSE progress END, finished_at = ? WHERE id = ?`,
		state, msg, code, arg, state, now(), j.id); err != nil {
		slog.Error("task finish", "id", j.id, "err", err)
	}
	q.publish(db, j.id)
}

// execRetry runs a task-state write, retrying for about 30 seconds: the
// database may be held by a long scan transaction past busy_timeout, and a
// lost state write would strand the row as queued or running.
func (q *Queue) execRetry(ctx context.Context, query string, args ...any) (sql.Result, error) {
	var err error
	for attempt := 1; attempt <= 6; attempt++ {
		var res sql.Result
		if res, err = q.db.ExecContext(ctx, query, args...); err == nil {
			return res, nil
		}
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}
	return nil, err
}

// safeRun turns a panic in fn into an error so one bad book cannot take the
// server down.
func safeRun(ctx context.Context, fn Func, progress func(float64)) (err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("task panic", "panic", r)
			err = &CodedError{Code: "task.internal_error", Err: errors.New("Stopped by an internal error")}
		}
	}()
	return fn(ctx, progress)
}

// List returns queued and running tasks, then the 50 most recent others.
func (q *Queue) List(ctx context.Context) ([]Task, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT * FROM (
		  SELECT `+taskCols+`
		  FROM tasks WHERE state IN ('running', 'queued')
		  UNION ALL
		  SELECT * FROM (
		    SELECT `+taskCols+`
		    FROM tasks WHERE state NOT IN ('running', 'queued') ORDER BY created_at DESC LIMIT 50))
		ORDER BY CASE state WHEN 'running' THEN 0 WHEN 'queued' THEN 1 ELSE 2 END, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (q *Queue) get(ctx context.Context, id string) (Task, error) {
	row := q.db.QueryRowContext(ctx, `SELECT `+taskCols+` FROM tasks WHERE id = ?`, id)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

type scanner interface{ Scan(...any) error }

func scanTask(s scanner) (Task, error) {
	var t Task
	var created string
	var started, finished sql.NullString
	if err := s.Scan(&t.ID, &t.Kind, &t.Label, &t.LibraryID, &t.State, &t.Progress, &t.Error, &created, &started, &finished,
		&t.LabelCode, &t.LabelArg, &t.ErrorCode, &t.ErrorArg); err != nil {
		return t, err
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	t.StartedAt = parseNull(started)
	t.FinishedAt = parseNull(finished)
	return t, nil
}

func parseNull(s sql.NullString) *time.Time {
	if !s.Valid {
		return nil
	}
	v, err := time.Parse(time.RFC3339Nano, s.String)
	if err != nil {
		return nil
	}
	return &v
}
