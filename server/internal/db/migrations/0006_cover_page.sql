-- The page a book's thumbnail is made from. 1 is the first page; an
-- administrator may pick another, and analysis keeps that choice.
ALTER TABLE books ADD COLUMN cover_page INTEGER NOT NULL DEFAULT 1;
