-- ComiNavi schema, version 1. IDs are opaque text (ULID-like, sortable by
-- creation). Times are RFC 3339 UTC text. Booleans are 0/1 integers.

CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL -- JSON
);

CREATE TABLE libraries (
  id                   TEXT PRIMARY KEY,
  name                 TEXT NOT NULL,
  root                 TEXT NOT NULL UNIQUE,
  kind                 TEXT NOT NULL CHECK (kind IN ('manga','webtoon','comics','books')),
  scan_interval        TEXT NOT NULL DEFAULT 'daily' CHECK (scan_interval IN ('off','hourly','daily','weekly')),
  watch                INTEGER NOT NULL DEFAULT 0,
  import_comicinfo     INTEGER NOT NULL DEFAULT 1,
  import_local_artwork INTEGER NOT NULL DEFAULT 1,
  exclude              TEXT NOT NULL DEFAULT '[]', -- JSON array of glob patterns
  last_scan            TEXT,
  created_at           TEXT NOT NULL
);

CREATE TABLE series (
  id                 TEXT PRIMARY KEY,
  library_id         TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
  dir                TEXT NOT NULL,          -- folder relative to the library root ('' for root oneshots)
  oneshot_file       TEXT NOT NULL DEFAULT '', -- set when a file at the root is its own series ('' for folders;
                                             -- not NULL, since NULLs never conflict in UNIQUE)
  title              TEXT NOT NULL,
  title_sort         TEXT NOT NULL,
  alternate_titles   TEXT NOT NULL DEFAULT '[]',
  status             TEXT NOT NULL DEFAULT 'ongoing' CHECK (status IN ('ongoing','ended','hiatus','abandoned')),
  summary            TEXT NOT NULL DEFAULT '',
  publisher          TEXT NOT NULL DEFAULT '',
  reading_direction  TEXT NOT NULL DEFAULT 'rtl' CHECK (reading_direction IN ('ltr','rtl','vertical','webtoon')),
  age_rating         INTEGER,
  language           TEXT NOT NULL DEFAULT '',
  genres             TEXT NOT NULL DEFAULT '[]',
  tags               TEXT NOT NULL DEFAULT '[]',
  authors            TEXT NOT NULL DEFAULT '[]', -- JSON [{name, role}]
  total_book_count   INTEGER,
  locked_fields      TEXT NOT NULL DEFAULT '[]',
  deleted_at         TEXT,                   -- set when the folder disappeared (trash)
  created_at         TEXT NOT NULL,
  updated_at         TEXT NOT NULL,
  UNIQUE (library_id, dir, oneshot_file)
);
CREATE INDEX series_library ON series(library_id) WHERE deleted_at IS NULL;

CREATE TABLE books (
  id            TEXT PRIMARY KEY,
  series_id     TEXT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  library_id    TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
  path          TEXT NOT NULL,               -- relative to the library root
  format        TEXT NOT NULL CHECK (format IN ('cbz','cbr','cb7','pdf','epub')),
  size_bytes    INTEGER NOT NULL,
  mtime         TEXT NOT NULL,               -- file modification time, for change detection
  title         TEXT NOT NULL,
  number        REAL NOT NULL,
  number_sort   REAL NOT NULL,
  number_source TEXT NOT NULL DEFAULT 'file' CHECK (number_source IN ('file','position','comicinfo')),
                                             -- 'position' numbers are recomputed on every scan
  summary       TEXT NOT NULL DEFAULT '',
  release_date  TEXT,
  isbn          TEXT NOT NULL DEFAULT '',
  authors       TEXT NOT NULL DEFAULT '[]',
  tags          TEXT NOT NULL DEFAULT '[]',
  locked_fields TEXT NOT NULL DEFAULT '[]',
  page_count    INTEGER NOT NULL DEFAULT 0,
  media_status  TEXT NOT NULL DEFAULT 'outdated' CHECK (media_status IN ('ready','error','unsupported','outdated')),
  media_error   TEXT NOT NULL DEFAULT '',
  thumbnail     TEXT NOT NULL DEFAULT '',    -- file name under DATA/thumbnails, '' until made
  deleted_at    TEXT,
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL,
  UNIQUE (library_id, path)
);
CREATE INDEX books_series ON books(series_id, number_sort) WHERE deleted_at IS NULL;
CREATE INDEX books_created ON books(created_at);

CREATE TABLE pages (
  book_id    TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
  number     INTEGER NOT NULL,              -- 1-based, natural name order
  entry      TEXT NOT NULL,                 -- name inside the archive
  media_type TEXT NOT NULL,
  width      INTEGER NOT NULL,
  height     INTEGER NOT NULL,
  PRIMARY KEY (book_id, number)
) WITHOUT ROWID;

CREATE TABLE users (
  id            TEXT PRIMARY KEY,
  name          TEXT NOT NULL,
  email         TEXT NOT NULL UNIQUE COLLATE NOCASE,
  password_hash TEXT NOT NULL,
  role          TEXT NOT NULL CHECK (role IN ('admin','user')),
  can_download  INTEGER NOT NULL DEFAULT 1,
  can_stream    INTEGER NOT NULL DEFAULT 1,
  all_libraries INTEGER NOT NULL DEFAULT 1,
  last_active   TEXT,
  created_at    TEXT NOT NULL
);

-- Libraries a user may see when all_libraries = 0.
CREATE TABLE user_libraries (
  user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
  PRIMARY KEY (user_id, library_id)
) WITHOUT ROWID;

CREATE TABLE sessions (
  token_hash TEXT PRIMARY KEY,             -- sha256 of the cookie value
  user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX sessions_user ON sessions(user_id);

CREATE TABLE api_keys (
  key_hash   TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE read_progress (
  user_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  book_id   TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
  page      INTEGER NOT NULL,
  completed INTEGER NOT NULL DEFAULT 0,
  read_at   TEXT NOT NULL,
  PRIMARY KEY (user_id, book_id)
) WITHOUT ROWID;
CREATE INDEX read_progress_recent ON read_progress(user_id, read_at);

-- Reader settings: scope 'global' (target ''), 'series' or 'book'.
CREATE TABLE reader_prefs (
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  scope   TEXT NOT NULL CHECK (scope IN ('global','series','book')),
  target  TEXT NOT NULL DEFAULT '',
  prefs   TEXT NOT NULL, -- JSON
  PRIMARY KEY (user_id, scope, target)
) WITHOUT ROWID;

CREATE TABLE collections (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  ordered    INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);
CREATE TABLE collection_series (
  collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
  series_id     TEXT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  position      INTEGER NOT NULL,
  PRIMARY KEY (collection_id, series_id)
) WITHOUT ROWID;

CREATE TABLE read_lists (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  summary    TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE TABLE read_list_books (
  read_list_id TEXT NOT NULL REFERENCES read_lists(id) ON DELETE CASCADE,
  book_id      TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
  position     INTEGER NOT NULL,
  PRIMARY KEY (read_list_id, book_id)
) WITHOUT ROWID;

CREATE TABLE tasks (
  id          TEXT PRIMARY KEY,
  kind        TEXT NOT NULL CHECK (kind IN ('scan','analyze','thumbnail','hash','trash')),
  label       TEXT NOT NULL,
  library_id  TEXT REFERENCES libraries(id) ON DELETE CASCADE,
  state       TEXT NOT NULL CHECK (state IN ('running','queued','done','failed','cancelled')),
  progress    REAL NOT NULL DEFAULT 0,
  error       TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL,
  started_at  TEXT,
  finished_at TEXT
);
CREATE INDEX tasks_state ON tasks(state, created_at);

-- Full-text search over titles and authors. trigram matches substrings, which
-- Japanese titles need (no word boundaries). Kept in sync by the app.
CREATE VIRTUAL TABLE search_index USING fts5(
  kind UNINDEXED,   -- 'series' | 'book'
  ref UNINDEXED,    -- series or book id
  library_id UNINDEXED,
  text,
  tokenize = 'trigram'
);
