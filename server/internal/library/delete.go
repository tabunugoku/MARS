package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
// because tests build Service literals.
func (s *Service) deleteLock() ctxMutex {
	s.deleteOnce.Do(func() { s.deleteMu = newCtxMutex() })
	return s.deleteMu
}

// QueueDelete queues the removal of a library. The caller has already set its
// deleting flag, which hides the library at once; the task then removes the
// rows in batches (see deleteLibrary) and the library row last. The queue
// key is per library, so a second request folds into the first.
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
	return s.Queue.SubmitKey(ctx, "delete", "delete:"+libraryID, label, libraryID, func(ctx context.Context, progress func(float64)) error {
		return s.deleteLibrary(ctx, libraryID, progress)
	})
}

// queueDeletes queues the removal of every library still marked deleting, so
// a deletion cut short by a restart goes on, and one whose task ended without
// removing the library is tried again (see Schedule). A library with a delete
// queued or running is skipped: that task is doing the work. Run resets the
// tasks left by a restart before Schedule calls this, so none is left behind.
func (s *Service) queueDeletes(ctx context.Context) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM libraries l WHERE deleting = 1 AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.library_id = l.id AND t.kind = 'delete' AND t.state IN ('queued','running')) ORDER BY id`)
	if err != nil {
		slog.Error("resume deletes", "err", err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			slog.Error("resume deletes", "err", err)
			return
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		// ErrNotFound: the library went while the list was read.
		if _, err := s.QueueDelete(ctx, id); err != nil && !errors.Is(err, ErrNotFound) {
			slog.Error("resume delete", "library", id, "err", err)
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
