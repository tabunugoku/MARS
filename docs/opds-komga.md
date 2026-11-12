# OPDS and the Komga-compatible API

ComiNavi speaks two protocols that other apps already know:

- **OPDS 1.2** with OPDS-PSE page streaming, for reader apps such as Panels,
  Chunky and KyBook. Catalog address: `http://HOST:25600/opds/v1.2/catalog`.
- **A Komga-shaped API** for the Mihon/Tachiyomi Komga extension and Mihon's
  Komga tracker. Address to enter in the extension: `http://HOST:25600/komga`.

Both are off until an admin turns them on in 設定 > サーバー
(`opdsEnabled`, `komgaApiEnabled`). While off, every path under them
answers 404.

## Shared rules (`server/internal/httpapi/compat.go`)

- `NewCompat(deps)` serves `/opds/` and `/komga/`; `cmd/cominavi` mounts it at
  those two prefixes without stripping them. Routes are registered with full
  paths (`GET /opds/v1.2/catalog`, `GET /komga/api/v1/series`).
- Authentication: HTTP Basic (user name or email + password, or any name with
  an API key as the password), `X-API-Key`, `Authorization: Bearer <key>`, or
  the `cn_compat` cookie. No credentials → 401 with
  `WWW-Authenticate: Basic`. The first Basic sign-in starts a 30-day session
  and sets `cn_compat`; Mihon's tracker sends no credentials and relies on it.
  A sign-in to `/komga/` with an API key (any of the three ways) sets
  `cn_compat_key` (Path `/komga/`) to that key instead, checked as a key on
  every use, so deleting the key ends it; `/opds/` never echoes a key.
  Credentials the request sends win over either cookie. Neither cookie works
  on `/api/v1`.
- Handlers read the caller with `currentUser(ctx)` and must read series and
  books only through `seriesQuery`/`bookQuery`/`listSeries`/`listBooks`
  (`query.go`) or `libScope`/`visible` (`access.go`). Anything the caller
  may not see is 404, exactly as in the main API.
- Pages, thumbnails and downloads reuse the main API's serving code
  (`servePage`, `serveThumbnail`, `serveBookFile`): call the same helpers,
  don't copy them.
- Read-only, except the progress writes listed below. Progress writes go
  through the same code as the main API (`markRead`, `setReadState`,
  `SetBookProgress`'s logic) and publish the same events.
- Text meant for people (feed titles such as 続きを読む) comes from the
  message catalog in the request's language (`msg(ctx, code)`), in a
  `messages_<task>.go` file. Data (titles, names) is as stored.

## OPDS 1.2 (`opds.go`)

Atom XML (`application/atom+xml;profile=opds-catalog;kind=navigation` or
`kind=acquisition`), UTF-8, built with `encoding/xml` (no string pasting).
Every feed has `id`, `title`, `updated`, a `self` link, a `start` link to
the catalog, and a `search` link to the OpenSearch description. Links are
absolute paths (`/opds/v1.2/...`). Lists page by 50 with `?page=` (0-based)
and `next`/`previous` links.

| Path | Feed |
|---|---|
| `/opds/v1.2/catalog` | Navigation: 続きを読む, 最近追加されたシリーズ, 最近追加された巻, ライブラリ, コレクション, リーディングリスト |
| `/opds/v1.2/keep-reading` | Acquisition: books in progress for the caller, latest read first |
| `/opds/v1.2/series/latest` | Navigation: series, newest first |
| `/opds/v1.2/books/latest` | Acquisition: books, newest first |
| `/opds/v1.2/libraries` | Navigation: the caller's libraries, then each library's series |
| `/opds/v1.2/libraries/{id}` | Navigation: the library's series by title |
| `/opds/v1.2/collections`, `/opds/v1.2/collections/{id}` | Collections; a collection's series in its order |
| `/opds/v1.2/readlists`, `/opds/v1.2/readlists/{id}` | Read lists; a list's books (acquisition) in order |
| `/opds/v1.2/series/{id}` | Acquisition: the series' books by number |
| `/opds/v1.2/search.xml` | OpenSearch description (`application/opensearchdescription+xml`), template `/opds/v1.2/search?q={searchTerms}` |
| `/opds/v1.2/search?q=` | Navigation: matching series (same matching as the main search) |
| `/opds/v1.2/books/{id}/pages/{n}` | One page image, `n` 0-based (OPDS-PSE) |

A series entry links to its acquisition feed and carries its thumbnail
(`http://opds-spec.org/image` and `/image/thumbnail` →
`/api/v1/series/{id}/thumbnail` equivalent served under `/opds/`). A book
entry carries:

- `http://opds-spec.org/acquisition` → the book file (download), with its
  media type (`application/zip`, `application/x-rar-compressed`,
  `application/x-7z-compressed`, `application/pdf`, …) and length;
- the thumbnail links;
- the PSE stream link:
  `<link rel="http://vaemendis.net/opds-pse/stream" type="image/jpeg"
  href="/opds/v1.2/books/{id}/pages/{pageNumber}?maxWidth={maxWidth}"
  pse:count="N" pse:lastRead="P" pse:lastReadDate="…"/>`, with
  `xmlns:pse="http://vaemendis.net/opds-pse/ns"`. `lastRead` is the
  caller's 0-based page; omit `lastRead`/`lastReadDate` when unread. Books
  that are not ready (no pages) get no stream link.
- `dcterms:issued`, `author` entries, `summary` from the book or series.

Download and image links under `/opds/` go through the compat auth, so a
client that authenticated the feed can fetch them.

## Komga-shaped API (`komga.go`, `komga_progress.go`)

The inventory of what the extension and the tracker call, every field they
parse, and their quirks is in the research note this was built from (copied
into the task brief). The rules:

- Paths under `/komga/api/v1/...` (and the tracker's `/komga/api/v2/...`).
- JSON with Komga's field names. Every field the clients declare without a
  default is present, even when we have no value (empty string, empty list,
  0, false, or a fixed date), so their strict decoders do not fail.
- Paged lists use Komga's page wrapper (`content`, `pageable`, `totalElements`,
  `totalPages`, `last`, `first`, `number`, `size`, `numberOfElements`,
  `empty`, `sort`), `page` 0-based, default size 20, `unpaged=true` for all.
- Pages are 1-based. `?convert=png` re-encodes the page as PNG. The header is
  checked first: a page over 32 MP answers 422 (`too_large.page_convert`), a
  page no decoder reads answers 415 (`unsupported.page_convert`). At most two
  conversions run at once, and the PNG is streamed, not buffered.
- Enum values map: series status `ONGOING`/`ENDED`/`HIATUS`/`ABANDONED`,
  read status `UNREAD`/`IN_PROGRESS`/`READ`, media status `READY`/… .
- Unknown filters and sorts are ignored rather than rejected.
