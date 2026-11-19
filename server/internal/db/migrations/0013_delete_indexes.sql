-- Full indexes on the columns that deletes and their cascades search by. The
-- partial indexes (WHERE deleted_at IS NULL) stay for browsing, but a foreign
-- key cascade and a delete batch (SELECT id FROM books WHERE library_id = ?
-- LIMIT ?) cannot use them, so each batch scanned the whole table. Deleting a
-- library took minutes and held the write lock through most of them.
CREATE INDEX books_series_all ON books(series_id);
CREATE INDEX books_library_all ON books(library_id);
CREATE INDEX series_library_all ON series(library_id);
CREATE INDEX progress_writers_book ON progress_writers(book_id);
CREATE INDEX user_libraries_library ON user_libraries(library_id);
CREATE INDEX tasks_library ON tasks(library_id);
