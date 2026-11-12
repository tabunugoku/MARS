-- Search rows move into a plain table that can be looked up by (kind, ref);
-- search_index becomes an external-content FTS5 index over it, kept in step
-- by the triggers below. Before, a scan or an edit deleted and re-inserted
-- every search row of the library in one write transaction (library_id was
-- UNINDEXED, so even the DELETE read the whole index): seconds of write lock
-- at 37,000 books. Now only rows whose text changed are rewritten.
--
-- The text expressions must match seriesText / bookText in
-- internal/scan/search.go.
DROP TABLE search_index;

CREATE TABLE search_rows (
  id         INTEGER PRIMARY KEY,
  kind       TEXT NOT NULL,   -- 'series' | 'book'
  ref        TEXT NOT NULL,   -- series or book id
  library_id TEXT NOT NULL,
  text       TEXT NOT NULL,
  UNIQUE (kind, ref)
);
CREATE INDEX search_rows_library ON search_rows(library_id);

CREATE VIRTUAL TABLE search_index USING fts5(
  kind UNINDEXED,
  ref UNINDEXED,
  library_id UNINDEXED,
  text,
  content = 'search_rows',
  content_rowid = 'id',
  tokenize = 'trigram'
);

CREATE TRIGGER search_rows_insert AFTER INSERT ON search_rows BEGIN
  INSERT INTO search_index (rowid, kind, ref, library_id, text)
  VALUES (new.id, new.kind, new.ref, new.library_id, new.text);
END;
CREATE TRIGGER search_rows_delete AFTER DELETE ON search_rows BEGIN
  INSERT INTO search_index (search_index, rowid, kind, ref, library_id, text)
  VALUES ('delete', old.id, old.kind, old.ref, old.library_id, old.text);
END;
CREATE TRIGGER search_rows_update AFTER UPDATE ON search_rows BEGIN
  INSERT INTO search_index (search_index, rowid, kind, ref, library_id, text)
  VALUES ('delete', old.id, old.kind, old.ref, old.library_id, old.text);
  INSERT INTO search_index (rowid, kind, ref, library_id, text)
  VALUES (new.id, new.kind, new.ref, new.library_id, new.text);
END;

INSERT INTO search_rows (kind, ref, library_id, text)
SELECT 'series', s.id, s.library_id,
  s.title || ' ' || s.alternate_titles || ' ' || COALESCE((SELECT group_concat(json_extract(a.value, '$.name'), ' ') FROM json_each(s.authors) a), '')
FROM series s WHERE s.deleted_at IS NULL;

INSERT INTO search_rows (kind, ref, library_id, text)
SELECT 'book', b.id, b.library_id,
  b.title || ' ' || b.isbn || ' ' || COALESCE((SELECT group_concat(json_extract(a.value, '$.name'), ' ') FROM json_each(b.authors) a), '')
FROM books b WHERE b.deleted_at IS NULL;
