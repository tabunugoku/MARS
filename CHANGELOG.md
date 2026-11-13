# Changelog

All notable changes to MARS are listed here. Versions follow
[Semantic Versioning](https://semver.org/). Docker images are published as
`ghcr.io/tabunugoku/mars` with the tags `X.Y.Z`, `X.Y`, `X` and `latest`.

## 1.0.0 — 2026-11-13

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
