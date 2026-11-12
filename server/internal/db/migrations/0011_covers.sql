-- Covers: a series may carry an uploaded or local-artwork cover file, may
-- name the book its cover is taken from, and a book may carry an uploaded
-- thumbnail instead of one made from its cover page.
ALTER TABLE series ADD COLUMN thumbnail TEXT NOT NULL DEFAULT '';
ALTER TABLE series ADD COLUMN thumbnail_source TEXT NOT NULL DEFAULT '' CHECK (thumbnail_source IN ('', 'upload', 'local'));
ALTER TABLE series ADD COLUMN artwork_sig TEXT NOT NULL DEFAULT '';
ALTER TABLE series ADD COLUMN cover_book_id TEXT REFERENCES books(id) ON DELETE SET NULL;
ALTER TABLE books ADD COLUMN thumbnail_uploaded INTEGER NOT NULL DEFAULT 0;
