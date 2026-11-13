# Changelog

All notable changes to MARS are listed here. Versions follow
[Semantic Versioning](https://semver.org/). Docker images are published as
`ghcr.io/tabunugoku/mars` with the tags `X.Y.Z`, `X.Y`, `X` and `latest`
once a release is out.

## Unreleased

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
