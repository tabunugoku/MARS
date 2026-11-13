-- A library being deleted keeps this flag until its rows are gone: its books
-- and series are removed in the background in small batches (see
-- library.Service.QueueDelete). Until then the library is hidden from every
-- read and cannot be scanned or changed.
ALTER TABLE libraries ADD COLUMN deleting INTEGER NOT NULL DEFAULT 0;

-- Deleting a series sets the cover of any series that named the book.
CREATE INDEX series_cover_book ON series(cover_book_id);

-- Task kind 'delete' was not in the CHECK of kind, and SQLite cannot change a
-- CHECK in place, so the table is rebuilt. Nothing refers to tasks.
CREATE TABLE tasks_new (
  id          TEXT PRIMARY KEY,
  kind        TEXT NOT NULL CHECK (kind IN ('scan','analyze','thumbnail','hash','trash','delete')),
  label       TEXT NOT NULL,
  library_id  TEXT REFERENCES libraries(id) ON DELETE CASCADE,
  state       TEXT NOT NULL CHECK (state IN ('running','queued','done','failed','cancelled')),
  progress    REAL NOT NULL DEFAULT 0,
  error       TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL,
  started_at  TEXT,
  finished_at TEXT,
  label_code  TEXT NOT NULL DEFAULT '',
  label_arg   TEXT NOT NULL DEFAULT '',
  error_code  TEXT NOT NULL DEFAULT '',
  error_arg   TEXT NOT NULL DEFAULT ''
);
INSERT INTO tasks_new (id, kind, label, library_id, state, progress, error, created_at, started_at, finished_at, label_code, label_arg, error_code, error_arg)
  SELECT id, kind, label, library_id, state, progress, error, created_at, started_at, finished_at, label_code, label_arg, error_code, error_arg FROM tasks;
DROP TABLE tasks;
ALTER TABLE tasks_new RENAME TO tasks;
CREATE INDEX tasks_state ON tasks(state, created_at);
