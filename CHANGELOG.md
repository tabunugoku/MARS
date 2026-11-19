# Changelog

All notable changes to MARS are listed here. Versions follow
[Semantic Versioning](https://semver.org/). Docker images are published as
`ghcr.io/tabunugoku/mars` with the tags `X.Y.Z`, `X.Y`, `X` and `latest`
(0.x releases get no `X` tag).

## Unreleased

### Changed

- The MARS name in the sidebar and on the sign-in and setup pages is now the MARS wordmark: Unbounded cut once like a gutter between manga panels.

### Fixed

- Deleting several libraries at once no longer makes other writes fail with "database is locked (SQLITE_BUSY)". Deletes now run one after another in smaller batches, long writes take turns, and new indexes let each batch find its rows without reading whole tables. In a test with 37,000 books and 7.4 million page rows, six libraries were deleted in about 30 seconds while another write waited 25 ms at most (2.2 s before).
- A library delete whose task could not be queued (the database was busy) is now queued again by the scheduler within a minute. Before, the library stayed hidden but undeleted until the server restarted.
- A finished library delete stays in the task list; before, its task row was removed with the library.
- Covers no longer blink while thumbnails are being made. A book without a thumbnail keeps its title and number until the thumbnail exists, a new cover replaces the old one only once it has loaded, and a series cover is loaded again once per burst of changes instead of once per book. Versioned thumbnail URLs are now cached by the browser.

## 0.1.2 — 2026-11-15

Fixes for large and CMYK pages, decode memory and duplicate tasks.

### Fixed

- CMYK JPEG pages without an Adobe marker (common in print-resolution scans) now get covers and page thumbnails; they failed with "4-component JPEG doesn't have Adobe APP14 metadata".
- A page whose thumbnail cannot be made is not decoded again on every page-list open (until the book file changes or the server restarts), and the warning is logged once.
- Books with pages up to 200 megapixels (print masters reach about 115) are analysed; pages over 100 megapixels made the book fail with "image or entry over the size limit".
- An image is decoded only when the memory it needs, estimated from its header (format, colour type, bit depth, subsampling, progressive or interlaced), fits the decode limit: 5/8 of the container memory limit, or 1 GiB without one. A larger image gets no thumbnail instead of running the server out of memory; the book still opens. In a 1 GiB container this includes progressive JPEGs over about 100 megapixels.
- Two tasks of the same kind for one library no longer run at once. A scan or analysis requested while one is running now waits as one queued task and starts when the running one ends; before, two analyses of one library could run side by side and read every book twice.
- A new library is no longer scanned twice when its first scan takes longer than a minute: the scheduler skipped no library whose scan was still running, because its last scan time is set only when the scan ends.
- A request that the browser drops before it is answered (a page left while loading) is no longer logged as an error ("authenticate ... context canceled").

## 0.1.1 — 2026-11-14

Faster analysis and thumbnails.

### Changed

- Analysis of CBZ/ZIP books reads only the header of each page except the cover, instead of the whole archive. A damaged page body is now found when the page is opened, not during analysis.
- Books are analysed in parallel: up to 4 at a time across all libraries, never more than the task threads.
- Thumbnails are made about 40% faster: large pages are first shrunk by a box average, then resized.

## 0.1.0 — 2026-11-14

A preview release. It has every feature of the withdrawn 1.0.0 (listed
below) and these changes on top.

### Changed

- The server no longer scans every library when it starts. Libraries are scanned by hand, by their scan interval, when created, and by file watching.

### Fixed

- Books whose pages are AVIF images are read instead of reported as having no images.
- The server no longer runs out of memory under a container memory limit while analysing many large pages: it sets its Go memory target from the limit and bounds how many images are decoded at once.
- Deleting a large library no longer blocks other changes for minutes: the library disappears at once and its books are removed in the background in small batches.

## 1.0.0 — 2026-11-13 (withdrawn)

Withdrawn on 2026-11-14: testing on a large real library found the server
running out of memory and blocking writes for minutes while deleting a
library. The release, its tag and its images were removed; the fixes are
listed under Unreleased.

The first public release.

### Libraries

- Several libraries, each a folder on the server, picked by typing a path or
  from a list of the server's folders.
- Scans by hand, on a schedule and at start-up, plus a deep scan that
  re-reads every file. Optional file watching rescans a library about 30
  seconds after its files change.
- Formats: CBZ/ZIP, CBR/RAR (RAR5 and solid archives included), CB7/7z and
  PDF. PDF pages are shown in the reader; a PDF that cannot be read opens in
  the browser.
- Series and books come from the folder layout and file names.
  `ComicInfo.xml` and cover images in a series folder (cover, folder, poster)
  are imported.
- Exclude patterns per library.
- Files that disappear go to the trash, where they can be deleted one by one
  or all at once.
- Metadata editing for one item or many, with locks that keep a scan from
  overwriting edited fields; thumbnail upload and choice.
- Thumbnails in JPEG, WebP or AVIF.

### Reading

- Dashboard with in-progress books, up next, and recently added or updated
  series and books.
- Library and series grids with sorting and filters (read state, status,
  genre, tag, publisher, language, age rating, author).
- Search across series, books, collections and read lists.
- Collections of series and read lists of books, with CBL import.
- Web reader: left to right, right to left, vertical, webtoon scrolling,
  two-page spreads with a single cover page, fit modes, background colour,
  page list, keyboard, touch and click zones, preloading, and incognito mode.
- Reading progress synced between devices, and a card at the end of a book
  that leads to the next one.
- Mark books and series read or unread in bulk.
- Download a book file, or a whole series as a ZIP.

### Users and sharing

- Several users, administrators and regular users, with per-library access
  and permissions for downloads and page streaming.
- API keys, and "remember me" sign-in.
- OPDS 1.2 with page streaming (OPDS-PSE) for reader apps.
- A Komga-compatible API for the Mihon Komga extension and tracker.
- Live updates in the web app through server-sent events.

### Running it

- One static Go binary with SQLite and the web app inside. No external
  database.
- Docker images for `linux/amd64` and `linux/arm64` on distroless, with a
  built-in health check.
- Installable web app (PWA) on phones and tablets over HTTPS.
- Interface in Japanese and English.
- Task list with scan and analysis progress, media issue list, thumbnail
  regeneration and re-analysis.
