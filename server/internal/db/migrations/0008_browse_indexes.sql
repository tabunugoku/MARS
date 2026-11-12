-- The dashboard's "recently added / updated" shelves read the newest 20 live
-- rows. Indexes in exactly that order (with the id tie-break, live rows only)
-- let SQLite stop after 20 rows instead of building every row and sorting:
-- 80-110 ms at 37,000 books before (docs/perf-large-library.md).
CREATE INDEX books_recent ON books(created_at, id) WHERE deleted_at IS NULL;
DROP INDEX series_created;
CREATE INDEX series_created ON series(created_at, id) WHERE deleted_at IS NULL;
DROP INDEX series_updated;
CREATE INDEX series_updated ON series(updated_at, id) WHERE deleted_at IS NULL;

-- Books of a series in reading order (number_sort, then path): with path in
-- the index, "first unread book of the series" (dashboard on-deck) and the
-- previous/next book stop at the first match instead of sorting the series.
DROP INDEX books_series;
CREATE INDEX books_series ON books(series_id, number_sort, path) WHERE deleted_at IS NULL;

-- Library cards count and sum the live books of each library; this index
-- holds both columns, so that is an index-only scan.
CREATE INDEX books_library_live ON books(library_id, size_bytes) WHERE deleted_at IS NULL;
