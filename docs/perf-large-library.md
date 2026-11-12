# Large-library performance (37,000 books)

How ComiNavi's SQLite usage holds up at the size of a real migrated Komga
library: about 37,000 books, 2,500 series, 7.4 million page rows. It records
what was measured, what broke, what changed, and the numbers before and
after.

## Setup

- **Machine:** Windows 11 on a Snapdragon X Elite (X1E80100, 12 cores,
  16 GB RAM), NVMe SSD. A NAS is slower, so read every time here as a lower
  bound. Nothing in this document was measured on ZFS. The 16K record size
  in `docs/install/truenas.md` versus SQLite's 4 KB pages is **untested**.
- **Library:** made by `TestPerfGenerate`. It has 2,499 series and 37,000
  CBZ files of 180–220 tiny PNG pages each (7.4 M pages, 1.5 GB on disk).
  Every file has a `ComicInfo.xml` with a Japanese title, writer, penciller,
  publisher, genres and a summary of about 250 characters. Series lengths
  vary: half have 1–5 books, about a third 6–20, and the rest 21–100.
- **Run:** `TestPerfLargeLibrary` starts the real API in-process on a fresh
  data folder and creates the library through `POST /libraries`, which runs
  the real walk, sync and analysis. During every scan phase, this load runs
  the whole time:
  - 4 readers send `PUT /books/{id}/progress` (with `writer`/`seq`) every
    50 ms;
  - 2 readers send `GET /books/{id}/pages` every 100 ms;
  - a probe on its own connection takes and releases the write lock
    (`BEGIN IMMEDIATE`) every 20 ms. How long it waits is how long some
    other write transaction held the lock.

  Before the reads, the test user is given a reading history like a
  migrated Komga user's: about 4,700 books finished and 140 in progress.
- **Reproduce** (from `server/`):

  ```sh
  PERF_LIB=/tmp/perf/lib go test -tags perf -run TestPerfGenerate -timeout 1h ./internal/httpapi
  PERF_LIB=/tmp/perf/lib PERF_DATA=/tmp/perf/data PERF_OUT=/tmp/perf/report.md \
    go test -tags perf -run TestPerfLargeLibrary -timeout 2h -v ./internal/httpapi
  # query timings and EXPLAIN QUERY PLAN on the database left in PERF_DATA
  PERF_DATA=/tmp/perf/data go test -tags perf -run TestPerfExplain -v ./internal/httpapi
  # first Sync alone, with the write-lock probe
  PERF_LIB=/tmp/perf/lib go test -tags perf -run TestPerfSync -v ./internal/scan
  ```

  The `perf` build tag keeps all of this out of `go test ./...`.

## Results

### Reads

20 sequential requests each, on an idle server. Times are milliseconds,
measured at the HTTP handler (JSON included).

| request | before p50 | after p50 | after p95 |
|---|---:|---:|---:|
| `GET /series` (all series; what the web app asks) | 32,910 | 116 | 125 |
| `GET /series?libraryId=…` | 33,025 | 118 | 138 |
| `GET /series?limit=50` | 693 | 4 | 6 |
| `GET /series?sort=bookCount&limit=50` | 3,408 | 42 | 47 |
| `GET /series?sort=releaseDate&limit=50` | 2,934 | 68 | 77 |
| `GET /series?readState=unread&limit=50` | 48,164 | 105 | 115 |
| `GET /series?readState=inProgress&limit=50` | 57,281 | 141 | 154 |
| `GET /series?readState=read&limit=50` | 49,390 | 158 | 178 |
| `GET /series?genres=SF&limit=50` | 564 | 7 | 15 |
| `GET /series?search=騎士` | 1,703 | 39 | 48 |
| `GET /dashboard` | ≈ 59,000 ¹ | 149 | 152 |
| `GET /search?q=竜` (1 character) | — ² | 50 | 57 |
| `GET /search?q=騎士` (2 characters) | — ² | 49 | 55 |
| `GET /search?q=魔法使い` | — ² | 21 | 25 |
| `GET /search?q=山田太郎` (author) | — ² | 45 | 54 |
| `GET /series/{id}` | — ² | 0 | 1 |
| `GET /series/{id}/books` (100 books) | — ² | 1 | 3 |
| `GET /books/{id}` | — ² | 0 | 1 |
| `GET /books/{id}/pages` | — ² | 0.5 | 1 |
| `GET /books/{id}/adjacent` | — ² | 0 | 1 |
| `GET /libraries` | — ² | 7 | 12 |
| `PATCH /series/{id}` | — ² | 2 | |
| `PATCH /books/{id}` | — ² | 0.3 | |

¹ The baseline HTTP run did not get this far: it was stopped while still on
the series filters. This figure is the sum of the six dashboard queries,
timed one by one on the baseline database with `TestPerfExplain`:
keep reading 0.5 ms, on deck 5,162 ms, recently added books 113 ms,
recently added series 27,015 ms, recently read 10 ms, recently updated
series 27,108 ms.
² Not reached in the baseline run. Their queries were already indexed
lookups in `TestPerfExplain` (search 8–23 ms, page list < 1 ms), except
the library cards at 36 ms. Each edit used to rebuild the library's whole
search index inside its transaction (see below).

### Scans, with progress writes running

| | before | after |
|---|---|---|
| First scan: walk + sync / analysis | 2.3 s / 7 m 46 s | 7.5 s / 7 m 8 s |
| First scan: longest write-lock wait (probe) | 1,536 ms | 80 ms |
| First scan: slowest progress PUT | 1,838 ms | 235 ms |
| Rescan, nothing changed: longest wait / slowest PUT | 834 ms / 840 ms | 5 ms / 14 ms |
| Rescan, one book changed: longest wait / slowest PUT | 734 ms / 1,541 ms | 5 ms / 15 ms |
| Failed progress writes (`database is locked`, 5xx) | 0 | 0 |
| Database after the first scan / WAL | 538 MB / 30 MB | 547 MB / 14 MB |
| Peak RSS of the whole test process | 189 MB | 191 MB |
| Migration of an existing 37,000-book database to this schema | | 0.8 s |

No progress write failed even before the fixes: `busy_timeout(5000)` was
just enough here. But the longest stall was already 1.5–1.8 s on this
machine. On a NAS several times slower it would pass 5 s and fail. The fixed
version keeps every write transaction short, so the margin no longer
depends on the hardware.

## What was slow, and the fixes

### Series lists and the dashboard: 30–60 s → 0.1 s

Every series row carries the caller's read and in-progress counts. These
came from correlated subqueries of the form `books b JOIN read_progress rp`.
The database has no statistics (`ANALYZE` never runs), so for every series
SQLite started from all of the user's progress rows (about 4,700) and
looked each one up in `books`. That is 2,500 × 4,700 ≈ 12 M lookups per
request:

```
CORRELATED SCALAR SUBQUERY 2
  SEARCH rp USING PRIMARY KEY (user_id=?)
  SEARCH b USING INDEX sqlite_autoindex_books_1 (id=?)
```

Any query that returned series paid this cost: `/series`, the readState
filters, the dashboard's recently added and recently updated series, and
search. A `LIMIT 20` did not help, because the rows were built before being
sorted.

**Fix:** these subqueries now use `books b CROSS JOIN read_progress rp`
(`httpapi/query.go`, `browse.go`). SQLite keeps the order of a CROSS JOIN,
so each count walks the series' own books through `books_series` and looks
up each one's progress by primary key:

```
CORRELATED SCALAR SUBQUERY 2
  SEARCH b USING INDEX books_series (series_id=?)
  SEARCH rp USING PRIMARY KEY (user_id=? AND book_id=?)
```

### Dashboard "on deck": 5.2 s → 0.1 s

The on-deck query tested every unread book in the library (about 32,000)
against three nested subqueries. It now starts from the caller's finished
books: for each series with one, it takes that series' first unread book,
then checks that the series has nothing in progress and that the book is
not the series' first. The old "the book right before it is finished" test
followed from the other conditions and was dropped.
`TestDashboard` still covers the rules.

### Indexes (migration `0008_browse_indexes.sql`)

- `books_recent (created_at, id)` and new `series_created` / `series_updated`
  with `id` added, all partial on live rows. They are in the dashboard
  shelves' sort order, so a `LIMIT 20` reads 20 rows instead of sorting the
  library: 80–113 ms → < 1 ms.
- `books_series` gains `path` (`series_id, number_sort, path`). "First
  unread book", previous/next and a series' book list then stop at the
  first match without sorting.
- `books_library_live (library_id, size_bytes)`: the library cards' count
  and total size become an index-only scan: the query went from 36 ms to
  2 ms (`GET /libraries` answers in 7 ms).

### Write lock during scans

- **Every signed-in request waited for the write lock.** `touchUser` ran
  `UPDATE users SET last_active …` on every request. That statement wrote
  nothing within a minute of the previous one, but an UPDATE takes the
  write lock anyway, so every page view, GET included, queued behind a scan
  transaction. If that took longer than `busy_timeout`, the request failed
  with 500. `last_active` and an API key's `last_used` are now read first
  and written only when stale, and a failed write is logged, not returned
  (`auth/store.go`). In the after run, page-list GETs during scans peaked
  at 35 ms. Before, they peaked at 1,541 ms.
- **`scan.Sync` was one transaction over the whole library.** It now reads
  the current rows without a transaction and writes through `db.Batch`:
  short transactions of at most 1,000 writes or 50 ms. Each write is safe
  to repeat and re-checks the row it changes. For example, a renumber by
  position skips a book whose number was locked meanwhile. A Sync that
  stops part way is therefore finished by the next scan.
  `TestSyncOneWritePerTransaction` reruns the sync tests with a commit
  after every write.
- **The search index was rebuilt whole on every scan, analysis and edit.**
  `RebuildSearch` deleted and reinserted every search row of the library
  inside the caller's transaction. Since `library_id` was an UNINDEXED FTS5
  column, even the DELETE read the whole index. This was most of the
  0.7–0.8 s stall on a rescan with nothing changed, and every metadata edit
  paid it too. Migration `0007_search_rows.sql` moves the rows into a plain
  table `search_rows (kind, ref)`. `search_index` becomes an
  external-content FTS5 index over that table, kept in step by triggers.
  `scan.SyncSearch` compares stored and current text with a read, then
  rewrites only the rows that differ, in batches. Edits and analysis update
  only their own row (`IndexSeries`, `IndexBook`). Queries against
  `search_index` are unchanged.
- **The analysis results were applied in one transaction.** The
  series-level ComicInfo for every series and the search rebuild now go
  through `db.Batch` as well.
- **Back-to-back batches starved other writers.** With batches alone, the
  first scan still showed a 2.4 s wait. No single transaction was long, but
  each batch began again the moment the previous one committed. SQLite's
  lock is not fair: a waiting writer sleeps between retries (up to 100 ms)
  and kept losing the race. `Batch` now waits, after each commit, for as
  long as the transaction held the lock. The first sync takes longer
  (2.3 s → 7.5 s, once per library), and the longest wait dropped to 80 ms.
- Page rows are inserted with one prepared statement per book.

## Left as they are

- **`cache_size` / `mmap_size`.** Measured on the main queries with
  `TestPerfExplain` (`PERF_PRAGMAS`):

  | setting | `/series` all | readState=read | on deck |
  |---|---:|---:|---:|
  | default (2 MB per connection) | 94 ms | 69 ms | 114 ms |
  | `cache_size` 8–32 MB | 93–108 ms | 72–88 ms | 105–118 ms |
  | `cache_size` 64 MB | 61 ms | 38 ms | 101 ms |
  | `mmap_size` 256 MB | 83 ms | 51 ms | 120 ms |

  Only a 64 MB cache helps, once it holds the whole `books` table. The
  cache is per connection, so with 8 connections that is up to 512 MB of
  RSS on a NAS whose page cache (the ZFS ARC) already holds those pages. Not
  worth it. The DSN is unchanged.
- **Database size.** At 547 MB for 37,000 books, the size is mostly the
  `pages` table: 7.4 M rows, each repeating the 26-character book id.
  Keying pages by an integer book id would roughly halve it, but means
  rewriting the table. It is not needed for speed: a book's page list is an
  index range read in under 1 ms.
- **The first analysis takes about 7 minutes here** (about 12 ms per book,
  sequential). That time is reading the archives, not the database: each
  book commits its own short transaction. On a NAS it will be longer, and
  it is limited by disk throughput.
- **The web app loads every series at once.** `GET /series` without
  `limit` is called by the dashboard, search and media-issues screens (for
  series titles and genre names), and twice by the library screen, which
  renders all 2,500 cards. Each response is 1.6 MB of JSON, now built in
  about 120 ms. Paging the library grid, and giving books their series
  title so the other screens need no series list, are follow-ups: they
  change the API contract and the UI.

## Not measured

- ZFS, the 16K record size, and a spinning or single-disk pool.
- Several users at once. Progress writes were tested only with a single
  user's four writers and two readers.
- A library split across several root folders or libraries. The work is
  per library, so this should scale linearly.
