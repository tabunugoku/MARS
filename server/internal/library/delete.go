package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/tabunugoku/mars/server/internal/db"
	"github.com/tabunugoku/mars/server/internal/task"
)

// Rows one delete transaction removes, at most. A book brings its 200 page
// rows with it, so books go in smaller batches than series.
const (
	deleteBookBatch   = 50
	deleteSeriesBatch = 200
)

// deleteLock lets one library delete run at a time. It is made on first use,
// because tests build Service literals. The lock stays even though one drain
// is the only task that deletes: a drain past its final check may still be
// returning when a new one starts (see QueueDelete), and both must not delete
// at once.
func (s *Service) deleteLock() ctxMutex {
	s.deleteOnce.Do(func() { s.deleteMu = newCtxMutex() })
	return s.deleteMu
}

// QueueDelete queues the removal of a library. The caller has already set its
// deleting flag, which hides the library at once. One delete task removes every
// library marked deleting, one after another (see drainDeletes), so while its
// drain is open this returns that task and submits nothing: the drain reaches
// this library because the flag is already set. Otherwise a task for this
// library is submitted, with the queue key per library as before.
//
// A drain stops being open at its final check (drainOpen is then false), so a
// request that arrives after that is submitted as a new task: a drain that is
// returning must not take the library, or it would wait for a tick. A queued
// drain that is cancelled before it runs leaves drainOpen true, but Live then
// returns not ok, so the next request submits as well.
//
// The check and the submit run under deleteQueueMu, so two requests cannot
// both submit a task.
func (s *Service) QueueDelete(ctx context.Context, libraryID string) (task.Task, error) {
	var name string
	err := s.DB.QueryRowContext(ctx, `SELECT name FROM libraries WHERE id = ? AND deleting = 1`, libraryID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return task.Task{}, ErrNotFound
	}
	if err != nil {
		return task.Task{}, err
	}
	label := task.Text{Code: "task.delete_library", Arg: name, English: "Delete " + name}

	s.deleteQueueMu.Lock()
	defer s.deleteQueueMu.Unlock()
	live, ok, err := s.Queue.Live(ctx, "delete")
	if err != nil {
		return task.Task{}, err
	}
	if ok && s.drainOpen {
		return live, nil
	}
	t, err := s.Queue.SubmitKey(ctx, "delete", "delete:"+libraryID, label, libraryID, func(ctx context.Context, progress func(float64)) error {
		return s.drainDeletes(ctx, libraryID, progress)
	})
	if err != nil {
		return task.Task{}, err
	}
	s.drainOpen = true
	s.drainTask = t.ID
	return t, nil
}

// drainDeletes is the work of a delete task. It removes libraryID, then every
// library still marked deleting, one at a time, until none is left. One task
// serves all of them, so deletes waiting for their turn do not hold the task
// workers that scans and analyses need.
//
// A library marked after the last query below is still found: the final check
// runs under deleteQueueMu, the lock QueueDelete holds while it decides to fold
// a request into this drain. A request that gets the lock after drainOpen is
// cleared is not folded in, and submits a new drain instead (see QueueDelete).
// So a library marked during the drain never waits for a tick.
//
// A library whose delete fails is logged and left marked deleting, and the
// drain goes on with the others. When the drain ends with failures it returns
// an error naming how many libraries failed, so the task shows as failed. A
// failure does not restart within this task: the tick does not restart a
// failed drain (see queueOrphanDeletes).
func (s *Service) drainDeletes(ctx context.Context, libraryID string, progress func(float64)) error {
	var (
		skip   []string // libraries whose delete failed; the queries leave them out
		failed int
		first  error
	)
	id := libraryID
	for {
		if id != libraryID {
			s.relabelDrain(ctx, id)
		}
		if err := s.deleteLibrary(ctx, id, progress); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Error("delete library", "library", id, "err", err)
			failed++
			if first == nil {
				first = err
			}
			skip = append(skip, id)
		}
		next, err := s.nextDelete(ctx, skip)
		if err != nil {
			return err
		}
		if next == "" {
			if s.drainIdle != nil {
				s.drainIdle()
			}
			s.deleteQueueMu.Lock()
			next, err = s.nextDelete(ctx, skip)
			if err != nil {
				s.deleteQueueMu.Unlock()
				return err
			}
			if next == "" {
				s.drainOpen = false
				s.deleteQueueMu.Unlock()
				break
			}
			s.deleteQueueMu.Unlock()
		}
		id = next
	}
	if failed > 0 {
		return fmt.Errorf("%d libraries: %w", failed, first)
	}
	return nil
}

// nextDelete returns the first library marked deleting that is not in skip, or
// "" when there is none.
func (s *Service) nextDelete(ctx context.Context, skip []string) (string, error) {
	q := `SELECT id FROM libraries WHERE deleting = 1`
	if len(skip) > 0 {
		q += ` AND id NOT IN (` + placeholders(skip) + `)`
	}
	q += ` ORDER BY id LIMIT 1`
	var next string
	err := s.DB.QueryRowContext(ctx, q, anys(skip)...).Scan(&next)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return next, err
}

// relabelDrain shows the library the drain is about to delete as the task's
// label. A failure leaves the old label up, so it is only logged.
func (s *Service) relabelDrain(ctx context.Context, libraryID string) {
	var name string
	if err := s.DB.QueryRowContext(ctx, `SELECT name FROM libraries WHERE id = ?`, libraryID).Scan(&name); err != nil {
		slog.Warn("delete label", "library", libraryID, "err", err)
		return
	}
	s.deleteQueueMu.Lock()
	id := s.drainTask
	s.deleteQueueMu.Unlock()
	if id == "" {
		return
	}
	label := task.Text{Code: "task.delete_library", Arg: name, English: "Delete " + name}
	if err := s.Queue.Relabel(ctx, id, label); err != nil {
		slog.Warn("delete label", "task", id, "err", err)
	}
}

// resumeDeletes queues the removal of every library still marked deleting. It
// runs once, when Schedule starts, so a deletion cut short by a restart goes on.
// The first QueueDelete submits the task and the rest fold into it. Run resets
// the tasks left by a restart before Schedule calls this, so none is left behind.
func (s *Service) resumeDeletes(ctx context.Context) {
	s.queueDeletes(ctx, `SELECT id FROM libraries WHERE deleting = 1 ORDER BY id`)
}

// queueOrphanDeletes is the check on each tick. It queues only a library marked
// deleting that has no delete task row at all: the case where the handler's
// queue request failed, or the library was marked after the draining task's
// last check. Libraries folded into a drain have no row of their own, so the
// tick does nothing while the most recent delete task is cancelled or failed:
// the admin stopped the deletes, or a library failed, and restarting them
// here would run the same drain again each minute. A cancel or failure stops
// deletes until the next server start or the next delete request, whose drain
// removes every library marked deleting, including these.
func (s *Service) queueOrphanDeletes(ctx context.Context) {
	var last string
	err := s.DB.QueryRowContext(ctx, `SELECT state FROM tasks WHERE kind = 'delete' ORDER BY created_at DESC LIMIT 1`).Scan(&last)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		slog.Error("queue orphan deletes", "err", err)
		return
	}
	if last == "cancelled" || last == "failed" {
		return
	}
	s.queueDeletes(ctx, `SELECT id FROM libraries l WHERE deleting = 1 AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.library_id = l.id AND t.kind = 'delete') ORDER BY id`)
}

// queueDeletes runs the query, which lists library ids, and queues each one's
// delete (see QueueDelete).
func (s *Service) queueDeletes(ctx context.Context, query string) {
	rows, err := s.DB.QueryContext(ctx, query)
	if err != nil {
		slog.Error("queue deletes", "err", err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			slog.Error("queue deletes", "err", err)
			return
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		// ErrNotFound: the library went while the list was read.
		if _, err := s.QueueDelete(ctx, id); err != nil && !errors.Is(err, ErrNotFound) {
			slog.Error("queue delete", "library", id, "err", err)
		}
	}
}

// deleteLibrary removes a library that is marked deleting: its books, then its
// series, each in batches of one short write transaction (db.Unit), then the
// search rows left, the collections the series emptied and the library row.
// Every step is safe to run again, so a task cut short (cancelled, or the
// server stopped) leaves the library marked deleting and the next start
// finishes it. Thumbnail files are not removed (the handler never removed
// them either).
func (s *Service) deleteLibrary(ctx context.Context, id string, progress func(float64)) error {
	if s.deleteFail != nil {
		if err := s.deleteFail(id); err != nil {
			return err
		}
	}
	// Deletes take turns (see deleteLock): the batches of one leave the write
	// lock free for the others, and deleted libraries go one after another.
	dl := s.deleteLock()
	if err := dl.Lock(ctx); err != nil {
		return err
	}
	defer dl.Unlock()
	// Scans hold this lock too (scanLibrary): one that began before the flag
	// was set must not write rows back into the library while it goes.
	mu := s.scanLock(id)
	if err := mu.Lock(ctx); err != nil {
		return err
	}
	defer mu.Unlock()

	bookSize, seriesSize := s.deleteBatch, s.deleteBatch
	if bookSize <= 0 {
		bookSize = deleteBookBatch
	}
	if seriesSize <= 0 {
		seriesSize = deleteSeriesBatch
	}
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM books WHERE library_id = ?`, id).Scan(&total); err != nil {
		return err
	}
	// Collections are looked at before the series go: a series row cannot
	// say afterwards which collections it was in.
	var emptied []string
	if err := db.Unit(ctx, s.DB, func(tx *sql.Tx) error {
		on, err := emptyCollectionsOn(ctx, tx)
		if err != nil || !on {
			return err
		}
		emptied, err = db.CollectionsLosing(ctx, tx, `SELECT id FROM series WHERE library_id = ?`, id)
		return err
	}); err != nil {
		return err
	}

	done := 0
	for {
		var n int
		err := db.Unit(ctx, s.DB, func(tx *sql.Tx) error {
			ids, err := idsOf(ctx, tx, `SELECT id FROM books WHERE library_id = ? LIMIT ?`, id, bookSize)
			if err != nil {
				return err
			}
			n = len(ids)
			if n == 0 {
				return nil
			}
			// Search rows first, as the trash purge does: they are matched by
			// the book ids, which the book delete removes.
			if _, err := tx.ExecContext(ctx, `DELETE FROM search_rows WHERE kind = 'book' AND ref IN (`+placeholders(ids)+`)`, anys(ids)...); err != nil {
				return err
			}
			// Pages go first, explicitly: the cascade would remove them too,
			// but the explicit delete keeps the plan obvious. Progress and
			// read-list entries go by cascade.
			if _, err := tx.ExecContext(ctx, `DELETE FROM pages WHERE book_id IN (`+placeholders(ids)+`)`, anys(ids)...); err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `DELETE FROM books WHERE id IN (`+placeholders(ids)+`)`, anys(ids)...)
			return err
		})
		if err != nil {
			return err
		}
		if n == 0 {
			break
		}
		done += n
		if total > 0 {
			progress(float64(min(done, total)) / float64(total))
		}
		s.deleteBatchDone()
	}

	for {
		var n int
		err := db.Unit(ctx, s.DB, func(tx *sql.Tx) error {
			ids, err := idsOf(ctx, tx, `SELECT id FROM series WHERE library_id = ? LIMIT ?`, id, seriesSize)
			if err != nil {
				return err
			}
			n = len(ids)
			if n == 0 {
				return nil
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM search_rows WHERE kind = 'series' AND ref IN (`+placeholders(ids)+`)`, anys(ids)...); err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `DELETE FROM series WHERE id IN (`+placeholders(ids)+`)`, anys(ids)...)
			return err
		})
		if err != nil {
			return err
		}
		if n == 0 {
			break
		}
		s.deleteBatchDone()
	}

	// Search rows of the library that no book or series row matched (none, in
	// a healthy database), in series-sized batches.
	for {
		var n int
		err := db.Unit(ctx, s.DB, func(tx *sql.Tx) error {
			res, err := tx.ExecContext(ctx, `DELETE FROM search_rows WHERE id IN (SELECT id FROM search_rows WHERE library_id = ? LIMIT ?)`, id, seriesSize)
			if err != nil {
				return err
			}
			n64, _ := res.RowsAffected()
			n = int(n64)
			return nil
		})
		if err != nil {
			return err
		}
		if n == 0 {
			break
		}
		s.deleteBatchDone()
	}

	if err := db.Unit(ctx, s.DB, func(tx *sql.Tx) error {
		if err := db.DropEmptiedCollections(ctx, tx, emptied); err != nil {
			return err
		}
		// The delete task's own row would go with the library by cascade.
		// It is kept, detached, so the task list shows the delete as done.
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET library_id = NULL WHERE library_id = ? AND kind = 'delete'`, id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM libraries WHERE id = ? AND deleting = 1`, id)
		return err
	}); err != nil {
		return err
	}
	progress(1)
	return nil
}

// deleteBatchDone runs the test hook after a committed batch.
func (s *Service) deleteBatchDone() {
	if s.deleteHook != nil {
		s.deleteHook()
	}
}

// emptyCollectionsOn reports the deleteEmptyCollections setting, on by default.
// The server stores its settings as JSON under the key 'server'.
func emptyCollectionsOn(ctx context.Context, tx *sql.Tx) (bool, error) {
	var raw string
	err := tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'server'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	var v struct {
		DeleteEmptyCollections *bool `json:"deleteEmptyCollections"`
	}
	if json.Unmarshal([]byte(raw), &v) != nil || v.DeleteEmptyCollections == nil {
		return true, nil
	}
	return *v.DeleteEmptyCollections, nil
}

// idsOf reads the first column of query as ids.
func idsOf(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// placeholders returns "?, ?, ..." with one mark per id.
func placeholders(ids []string) string {
	return strings.TrimSuffix(strings.Repeat("?, ", len(ids)), ", ")
}

func anys(ids []string) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}
