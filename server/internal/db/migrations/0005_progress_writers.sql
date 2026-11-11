-- Ordering per writer. 0004 kept one writer and seq on the progress row, so
-- a write from another tab replaced them and a late stale write from the
-- first tab slipped through (A seq 2, B seq 1, then A seq 1 won). Each
-- writer now keeps its own high-water mark.
CREATE TABLE progress_writers (
  user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  book_id    TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
  writer     TEXT NOT NULL,
  seq        INTEGER NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (user_id, book_id, writer)
) WITHOUT ROWID;

ALTER TABLE read_progress DROP COLUMN writer;
ALTER TABLE read_progress DROP COLUMN seq;
