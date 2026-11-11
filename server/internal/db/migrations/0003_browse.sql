-- Indexes for the M3 browsing queries (shelves, membership lookups).
CREATE INDEX series_created ON series(created_at) WHERE deleted_at IS NULL;
CREATE INDEX series_updated ON series(updated_at) WHERE deleted_at IS NULL;
CREATE INDEX collection_series_series ON collection_series(series_id);
CREATE INDEX read_list_books_book ON read_list_books(book_id);
CREATE INDEX read_progress_book ON read_progress(book_id);
