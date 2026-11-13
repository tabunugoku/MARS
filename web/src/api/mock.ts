// The mock server for `VITE_API=mock` (design work without the Go server).
// It is loaded with import() only in that mode, so neither it nor the
// generated art reaches a production build. Writes change the in-memory data
// and are lost on reload, except settings and reader prefs (localStorage).
import * as db from "@/mock/data";
import { coverArt, pageArt } from "@/lib/art";
import { ApiError } from "./http";
import type { Api } from "./real";
import type {
  Book, CblEntry, CblImportResult, Collection, Library, LoginScreen, ReaderPrefsAll, ReadList, Series, SeriesQuery,
  ServerSettings, User,
} from "./types";
import type { GlobalReaderPrefs, ReaderPrefs, ReaderProfile } from "./reader-prefs";

const LATENCY = 120;
const delay = <T,>(v: T): Promise<T> => new Promise((res) => setTimeout(() => res(structuredClone(v)), LATENCY));
const fail = (status: number, message: string) => Promise.reject(new ApiError(status, message));
const now = () => new Date().toISOString();
let nextId = 1;
const newId = (p: string) => `${p}-new-${nextId++}`;

// Object URLs of uploaded thumbnails, by series or book id (revoked on replace).
const uploads = new Map<string, string>();
function setUpload(key: string, file: Blob) {
  const old = uploads.get(key);
  if (old) URL.revokeObjectURL(old);
  uploads.set(key, URL.createObjectURL(file));
}
const series = (s: db.MockSeries): Series => ({ ...s, thumbnailUrl: seriesCover(s) });
const findSeries = (id: string) => db.series.find((s) => s.id === id);
const book = (b: db.MockBook): Book => ({
  ...b,
  seriesTitle: findSeries(b.seriesId)?.title ?? b.seriesTitle,
  thumbnailUrl: uploads.get(b.id) ?? coverArt(b.coverSeed, b.title, String(b.number)),
});
// An uploaded cover wins, then the cover book's thumbnail, then the generated art.
function seriesCover(s: db.MockSeries): string {
  if (s.thumbnailSource === "upload") return uploads.get(s.id) ?? "";
  const cover = s.coverBookId ? findBook(s.coverBookId) : undefined;
  return cover ? book(cover).thumbnailUrl : coverArt(s.coverSeed, s.title);
}
const findBook = (id: string) => db.books.find((b) => b.id === id);
const seriesBooks = (id: string) => db.books.filter((b) => b.seriesId === id).sort((a, b) => a.numberSort - b.numberSort);

// The list after adding and removing names; a name already present is kept once.
function editNames(cur: string[], add: string[] = [], remove: string[] = []): string[] {
  const out = cur.filter((x) => !remove.includes(x));
  for (const x of add) if (!out.includes(x)) out.push(x);
  return out;
}
const sameList = (a: string[], b: string[]) => a.length === b.length && a.every((x, i) => x === b[i]);
// Known total reached, as the server counts it (complete filter).
const reached = (s: Series) => s.totalBookCount !== null && s.bookCount >= s.totalBookCount;
// Comparison key of a series title, folded as the server does (cbl.go).
const cblKey = (s: string) => s.normalize("NFKC").replace(/\s/g, "").toLowerCase();
// The book for one CBL entry: a title match with that number; a matching release year wins.
function matchCbl(e: CblEntry): db.MockBook | undefined {
  const number = Number(e.number);
  if (e.number === "" || Number.isNaN(number)) return undefined;
  const key = cblKey(e.series);
  let first: db.MockBook | undefined;
  let fit: db.MockBook | undefined;
  for (const s of db.series) {
    if (![s.title, ...s.alternateTitles].some((t) => cblKey(t) === key)) continue;
    const b = seriesBooks(s.id).find((x) => x.number === number);
    if (!b) continue;
    if (!first) first = b;
    if (!fit && e.year && b.releaseDate?.startsWith(e.year)) fit = b;
  }
  return fit ?? first;
}

function readState(s: Series) {
  if (s.booksRead === s.bookCount) return "read" as const;
  if (s.booksRead > 0 || s.booksInProgress > 0) return "inProgress" as const;
  return "unread" as const;
}

function recount(seriesId: string) {
  const s = findSeries(seriesId);
  if (!s) return;
  const bs = seriesBooks(seriesId);
  s.booksRead = bs.filter((b) => b.progress?.completed).length;
  s.booksInProgress = bs.filter((b) => b.progress && !b.progress.completed).length;
}

function markRead(bs: db.MockBook[], completed: boolean) {
  for (const b of bs) b.progress = completed ? { page: Math.max(b.pageCount, 1), completed: true, readAt: now() } : null;
  new Set(bs.map((b) => b.seriesId)).forEach(recount);
}

function filterSeries(q: SeriesQuery): db.MockSeries[] {
  let xs = db.series.slice();
  if (q.libraryId) xs = xs.filter((s) => s.libraryId === q.libraryId);
  if (q.search) {
    const t = q.search.toLowerCase();
    xs = xs.filter((s) => s.title.toLowerCase().includes(t) || s.authors.some((a) => a.name.toLowerCase().includes(t)));
  }
  if (q.status?.length) xs = xs.filter((s) => q.status!.includes(s.status));
  if (q.genres?.length) xs = xs.filter((s) => s.genres.some((g) => q.genres!.includes(g)));
  if (q.tags?.length) xs = xs.filter((s) => s.tags.some((t) => q.tags!.includes(t)));
  if (q.publishers?.length) xs = xs.filter((s) => q.publishers!.includes(s.publisher));
  if (q.languages?.length) xs = xs.filter((s) => q.languages!.includes(s.language));
  if (q.authors?.length) xs = xs.filter((s) => s.authors.some((a) => q.authors!.includes(a.name)));
  if (q.ageRatingMax !== undefined) xs = xs.filter((s) => s.ageRating === null || s.ageRating <= q.ageRatingMax!);
  if (q.complete !== undefined) xs = xs.filter((s) => reached(s) === q.complete);
  if (q.readState?.length) xs = xs.filter((s) => q.readState!.includes(readState(s)));
  const sort = q.sort ?? "title";
  const key: Record<string, (s: Series) => string | number> = {
    title: (s) => s.titleSort,
    updated: (s) => s.updatedAt,
    created: (s) => s.createdAt,
    releaseDate: (s) => seriesBooks(s.id)[0]?.releaseDate ?? "9999",
    bookCount: (s) => s.bookCount,
  };
  xs.sort((a, b) => {
    const ka = key[sort](a), kb = key[sort](b);
    const c = typeof ka === "number" ? ka - (kb as number) : String(ka).localeCompare(String(kb), "ja");
    return q.desc ? -c : c;
  });
  const from = q.offset ?? 0;
  return xs.slice(from, q.limit ? from + q.limit : undefined);
}

// Settings and reader prefs survive a reload, as they would on a server.
function stored<T extends object>(key: string, defaults: T): T {
  try {
    const v: unknown = JSON.parse(localStorage.getItem(key) ?? "{}");
    return v && typeof v === "object" ? { ...defaults, ...(v as Partial<T>) } : defaults;
  } catch {
    return defaults;
  }
}
const SETTINGS_KEY = "cn-mock-server-settings";
const PREFS_KEY = "cn-mock-reader-prefs";
const loadSettings = () => stored<ServerSettings>(SETTINGS_KEY, db.settings);
const loadPrefs = () =>
  stored<ReaderPrefsAll>(PREFS_KEY, {
    global: { direction: "series", spread: "off", fit: "screen", bg: "black" },
    series: {},
    books: {},
  });
function savePrefs(change: (p: ReaderPrefsAll) => void) {
  const p = loadPrefs();
  change(p);
  localStorage.setItem(PREFS_KEY, JSON.stringify(p));
  return delay(undefined);
}

// True when the two id lists hold the same members (no duplicates) in any order.
const sameMembers = (next: string[], current: string[]) =>
  next.length === current.length && new Set(next).size === next.length && next.every((x) => current.includes(x));

const me = (): User => db.currentUser;
const shelf = (bs: db.MockBook[], n = 20) => bs.slice(0, n).map(book);

export const mockApi: Api = {
  health: () => Promise.resolve({ status: "ok", version: "mock" }),
  loginScreen: (): Promise<LoginScreen> => {
    const backdrop = loadSettings().loginBackdrop;
    const covers =
      backdrop === "covers"
        ? [...db.series]
            .sort((a, b) => b.createdAt.localeCompare(a.createdAt))
            .slice(0, 36)
            .map((s) => ({ thumbnailUrl: coverArt(s.coverSeed, s.title), title: s.title }))
        : backdrop === "bookCovers"
          ? [...db.books]
              .sort((a, b) => b.createdAt.localeCompare(a.createdAt))
              .slice(0, 36)
              .map((b) => {
                const bk = book(b);
                return { thumbnailUrl: bk.thumbnailUrl, title: `${bk.seriesTitle} ${bk.title}` };
              })
          : [];
    return delay({ backdrop, covers, needsSetup: false });
  },
  login: () => delay(me()),
  setup: () => delay(me()),
  logout: () => delay(undefined),

  me: () => delay(me()),
  sessionUser: () => delay(me()),
  updateMe: (name, locale) => {
    db.currentUser.name = name.trim();
    if (locale !== undefined) db.currentUser.locale = locale;
    return delay(me());
  },
  changePassword: (current) => (current ? delay(undefined) : fail(400, "現在のパスワードが違います")),
  apiKeys: () => delay([]),
  createApiKey: (name) => delay({ id: newId("key"), name, key: "cn_mock_" + Math.random().toString(36).slice(2) }),
  deleteApiKey: () => delay(undefined),

  libraries: () => delay(db.libraries),
  library: (id) => delay(db.libraries.find((l) => l.id === id) ?? null),
  createLibrary: (body) => {
    const l: Library = {
      id: newId("lib"), seriesCount: 0, bookCount: 0, sizeBytes: 0, lastScan: null, scanInterval: "daily", watch: false,
      importComicInfo: true, importLocalArtwork: true, exclude: [], ...body,
    };
    db.libraries.push(l);
    return delay(l);
  },
  updateLibrary: (id, body) => {
    const l = db.libraries.find((x) => x.id === id);
    if (!l) return fail(404, "ライブラリが見つかりません");
    Object.assign(l, body);
    return delay(l);
  },
  deleteLibrary: (id) => {
    const i = db.libraries.findIndex((x) => x.id === id);
    if (i >= 0) db.libraries.splice(i, 1);
    return delay(undefined);
  },
  scanLibrary: (id, deep = false) =>
    delay({ id: newId("t"), kind: "scan" as const, label: deep ? "詳細スキャン" : "スキャン", libraryId: id, state: "queued" as const, progress: 0, startedAt: null, error: "" }),
  regenerateLibraryThumbnails: (id) =>
    delay({ id: newId("t"), kind: "thumbnail" as const, label: "表紙を作り直す", libraryId: id, state: "queued" as const, progress: 0, startedAt: null, error: "" }),
  markLibraryRead: (id, completed) => {
    markRead(db.books.filter((b) => b.libraryId === id), completed);
    return delay(undefined);
  },

  seriesPage: (q) =>
    delay({
      items: filterSeries(q).map(series),
      total: filterSeries({ ...q, limit: undefined, offset: undefined }).length,
    }),
  genres: (libraryId) =>
    delay([...new Set(db.series.filter((s) => !libraryId || s.libraryId === libraryId).flatMap((s) => s.genres))].sort()),
  seriesFacets: (libraryId) => {
    const ss = db.series.filter((s) => !libraryId || s.libraryId === libraryId);
    const distinct = (xs: string[]) => [...new Set(xs.filter((x) => x !== ""))].sort((a, b) => a.localeCompare(b, "ja"));
    return delay({
      genres: distinct(ss.flatMap((s) => s.genres)),
      tags: distinct(ss.flatMap((s) => s.tags)),
      publishers: distinct(ss.map((s) => s.publisher)),
      languages: distinct(ss.map((s) => s.language)),
      authors: distinct(ss.flatMap((s) => s.authors.map((a) => a.name))),
      ageRatings: [...new Set(ss.flatMap((s) => (s.ageRating === null ? [] : [s.ageRating])))].sort((a, b) => a - b),
    });
  },
  seriesById: (id) => {
    const s = findSeries(id);
    return delay(s ? series(s) : null);
  },
  seriesByIds: (ids) => delay(ids.map(findSeries).filter((s) => s !== undefined).map(series)),
  updateSeries: (id, changes) => {
    const s = findSeries(id);
    if (!s) return fail(404, "シリーズが見つかりません");
    const { lockedFields, coverBookId, ...fields } = changes;
    Object.assign(s, fields);
    if (coverBookId !== undefined) s.coverBookId = coverBookId;
    s.lockedFields = lockedFields ?? [...new Set([...s.lockedFields, ...Object.keys(fields)])];
    s.updatedAt = now();
    return delay(series(s));
  },
  bulkUpdateSeries: (body) => {
    const targets = body.seriesIds.map(findSeries).filter((s): s is db.MockSeries => s !== undefined);
    if (targets.length !== body.seriesIds.length) return fail(404, "シリーズが見つかりません");
    let updated = 0;
    for (const s of targets) {
      const locks: string[] = [];
      if (body.status !== undefined && s.status !== body.status) { s.status = body.status; locks.push("status"); }
      if (body.publisher !== undefined && s.publisher !== body.publisher) { s.publisher = body.publisher; locks.push("publisher"); }
      if (body.readingDirection !== undefined && s.readingDirection !== body.readingDirection) {
        s.readingDirection = body.readingDirection;
        locks.push("readingDirection");
      }
      if (body.ageRating !== undefined && s.ageRating !== body.ageRating) { s.ageRating = body.ageRating; locks.push("ageRating"); }
      if (body.language !== undefined && s.language !== body.language) { s.language = body.language; locks.push("language"); }
      const genres = editNames(s.genres, body.addGenres, body.removeGenres);
      if (!sameList(genres, s.genres)) { s.genres = genres; locks.push("genres"); }
      const tags = editNames(s.tags, body.addTags, body.removeTags);
      if (!sameList(tags, s.tags)) { s.tags = tags; locks.push("tags"); }
      if (locks.length === 0) continue;
      s.lockedFields = [...new Set([...s.lockedFields, ...locks])];
      s.updatedAt = now();
      updated++;
    }
    return delay({ updated });
  },
  setSeriesReadState: (seriesIds, read) => {
    markRead(seriesIds.flatMap((id) => seriesBooks(id)), read);
    return delay(undefined);
  },
  uploadSeriesThumbnail: (id, file) => {
    const s = findSeries(id);
    if (!s) return fail(404, "シリーズが見つかりません");
    setUpload(id, file);
    s.thumbnailSource = "upload";
    return delay(series(s));
  },
  deleteSeriesThumbnail: (id) => {
    const s = findSeries(id);
    if (!s) return fail(404, "シリーズが見つかりません");
    const old = uploads.get(id);
    if (old) URL.revokeObjectURL(old);
    uploads.delete(id);
    s.thumbnailSource = "book";
    return delay(series(s));
  },
  markSeriesRead: (id, completed) => {
    markRead(seriesBooks(id), completed);
    return delay(undefined);
  },
  booksOfSeries: (id) => delay(seriesBooks(id).map(book)),
  book: (id) => {
    const b = findBook(id);
    return delay(b ? book(b) : null);
  },
  booksByIds: (ids) => delay(ids.map(findBook).filter((b) => b !== undefined).map(book)),
  updateBook: (id, changes) => {
    const b = findBook(id);
    if (!b) return fail(404, "本が見つかりません");
    const { lockedFields, ...fields } = changes;
    Object.assign(b, fields);
    b.lockedFields = lockedFields ?? [...new Set([...b.lockedFields, ...Object.keys(fields)])];
    return delay(book(b));
  },
  setBooksReadState: (bookIds, read) => {
    markRead(bookIds.map(findBook).filter((b): b is db.MockBook => b !== undefined), read);
    return delay(undefined);
  },
  uploadBookThumbnail: (id, file) => {
    const b = findBook(id);
    if (!b) return fail(404, "本が見つかりません");
    setUpload(id, file);
    b.thumbnailUploaded = true;
    return delay(book(b));
  },
  bulkUpdateBooks: (body) => {
    const targets = body.bookIds.map(findBook).filter((b): b is db.MockBook => b !== undefined);
    if (targets.length !== body.bookIds.length) return fail(404, "本が見つかりません");
    let updated = 0;
    for (const b of targets) {
      const tags = editNames(b.tags, body.addTags, body.removeTags);
      if (sameList(tags, b.tags)) continue;
      b.tags = tags;
      b.lockedFields = [...new Set([...b.lockedFields, "tags"])];
      updated++;
    }
    return delay({ updated });
  },
  adjacentBooks: (id) => {
    const b = findBook(id);
    const sib = b ? seriesBooks(b.seriesId) : [];
    const i = sib.findIndex((x) => x.id === id);
    const at = (j: number) => (sib[j] ? book(sib[j]) : null);
    return delay({ prev: at(i - 1), next: at(i + 1) });
  },
  pages: (bookId) => {
    const b = findBook(bookId);
    if (!b) return fail(404, "本が見つかりません");
    const webtoon = findSeries(b.seriesId)?.readingDirection === "webtoon";
    const [w, h] = webtoon ? [800, 1280] : [1000, 1500];
    return delay(
      Array.from({ length: b.pageCount }, (_, i) => {
        const url = pageArt(b.coverSeed, i + 1, { webtoon });
        return { number: i + 1, width: w, height: h, mediaType: "image/svg+xml", url, thumbnailUrl: url };
      }),
    );
  },
  setProgress: (bookId, page, completed) => {
    const b = findBook(bookId);
    if (!b) return fail(404, "本が見つかりません");
    if (page < 1 || page > b.pageCount) return fail(400, "ページが範囲外です");
    b.progress = { page, completed: completed ?? page === b.pageCount, readAt: now() };
    recount(b.seriesId);
    return delay(b.progress);
  },
  clearProgress: (bookId) => {
    const b = findBook(bookId);
    if (b) {
      b.progress = null;
      recount(b.seriesId);
    }
    return delay(undefined);
  },
  analyzeBook: (bookId) =>
    delay({ id: newId("t"), kind: "analyze" as const, label: "解析", libraryId: findBook(bookId)?.libraryId ?? null, state: "queued" as const, progress: 0, startedAt: null, error: "" }),
  regenerateBookThumbnail: (bookId) => (findBook(bookId) ? delay(undefined) : fail(404, "本が見つかりません")),
  setBookCover: (bookId, page) => {
    const b = findBook(bookId);
    if (!b) return fail(404, "本が見つかりません");
    if (page < 1 || page > b.pageCount) return fail(400, "そのページはありません");
    b.coverPage = page;
    return delay(book(b));
  },
  regenerateSeriesThumbnails: (seriesId) =>
    delay({ id: newId("t"), kind: "thumbnail" as const, label: "表紙を作り直す", libraryId: findSeries(seriesId)?.libraryId ?? null, state: "queued" as const, progress: 0, startedAt: null, error: "" }),

  dashboard: (libraryId) => {
    const bs = db.books.filter((b) => !libraryId || b.libraryId === libraryId);
    const ss = db.series.filter((s) => !libraryId || s.libraryId === libraryId);
    const byRead = (a: db.MockBook, b: db.MockBook) => b.progress!.readAt.localeCompare(a.progress!.readAt);
    const onDeck: db.MockBook[] = [];
    for (const s of ss) {
      const sb = seriesBooks(s.id);
      const i = sb.findIndex((b) => !b.progress);
      if (i > 0 && sb[i - 1].progress?.completed && !sb.some((b) => b.progress && !b.progress.completed)) onDeck.push(sb[i]);
    }
    return delay({
      keepReading: shelf(bs.filter((b) => b.progress && !b.progress.completed).sort(byRead)),
      onDeck: shelf(onDeck),
      recentlyAddedBooks: shelf(bs.slice().sort((a, b) => b.createdAt.localeCompare(a.createdAt))),
      recentlyAddedSeries: ss.slice().sort((a, b) => b.createdAt.localeCompare(a.createdAt)).slice(0, 20).map(series),
      recentlyUpdatedSeries: ss.slice().sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)).slice(0, 20).map(series),
      recentlyRead: shelf(bs.filter((b) => b.progress?.completed).sort(byRead)),
    });
  },
  search: (q) => {
    const t = q.trim().toLowerCase();
    if (!t) return delay({ series: [], books: [], collections: [], readLists: [] });
    return delay({
      series: db.series.filter((s) => s.title.toLowerCase().includes(t) || s.authors.some((a) => a.name.toLowerCase().includes(t))).slice(0, 20).map(series),
      books: db.books.filter((b) => b.title.toLowerCase().includes(t) || b.isbn.includes(t)).slice(0, 20).map(book),
      collections: db.collections.filter((c) => c.name.toLowerCase().includes(t)),
      readLists: db.readLists.filter((c) => c.name.toLowerCase().includes(t)),
    });
  },

  collections: () => delay(db.collections),
  collection: (id) => delay(db.collections.find((c) => c.id === id) ?? null),
  collectionSeries: (id) => {
    const c = db.collections.find((x) => x.id === id);
    if (!c) return fail(404, "コレクションが見つかりません");
    const xs = c.seriesIds.map(findSeries).filter((s) => s !== undefined);
    if (!c.ordered) xs.sort((a, b) => a.titleSort.localeCompare(b.titleSort, "ja"));
    return delay(xs.map(series));
  },
  createCollection: (body) => {
    const c: Collection = { id: newId("c"), name: body.name.trim(), ordered: body.ordered ?? false, seriesIds: body.seriesIds ?? [] };
    db.collections.push(c);
    return delay(c);
  },
  updateCollection: (id, body) => {
    const c = db.collections.find((x) => x.id === id);
    if (!c) return fail(404, "コレクションが見つかりません");
    Object.assign(c, body);
    return delay(c);
  },
  deleteCollection: (id) => {
    const i = db.collections.findIndex((x) => x.id === id);
    if (i >= 0) db.collections.splice(i, 1);
    return delay(undefined);
  },
  addCollectionSeries: (id, seriesId) => {
    const c = db.collections.find((x) => x.id === id);
    if (!c) return fail(404, "コレクションが見つかりません");
    if (!findSeries(seriesId)) return fail(404, "シリーズが見つかりません");
    if (!c.seriesIds.includes(seriesId)) c.seriesIds.push(seriesId);
    return delay(c);
  },
  addCollectionSeriesMany: (id, seriesIds) => {
    const c = db.collections.find((x) => x.id === id);
    if (!c) return fail(404, "コレクションが見つかりません");
    if (seriesIds.some((x) => !findSeries(x))) return fail(404, "シリーズが見つかりません");
    for (const x of seriesIds) if (!c.seriesIds.includes(x)) c.seriesIds.push(x);
    return delay(c);
  },
  removeCollectionSeries: (id, seriesId) => {
    const c = db.collections.find((x) => x.id === id);
    if (!c) return fail(404, "コレクションが見つかりません");
    c.seriesIds = c.seriesIds.filter((x) => x !== seriesId);
    return delay(c);
  },
  reorderCollection: (id, seriesIds) => {
    const c = db.collections.find((x) => x.id === id);
    if (!c) return fail(404, "コレクションが見つかりません");
    if (!sameMembers(seriesIds, c.seriesIds)) return fail(409, "コレクションの内容が変わりました");
    c.seriesIds = [...seriesIds];
    return delay(c);
  },
  importReadList: async (file, dryRun) => {
    const doc = new DOMParser().parseFromString(await file.text(), "text/xml");
    if (doc.getElementsByTagName("parsererror").length > 0) throw new ApiError(400, "CBL ファイルを読み込めませんでした");
    const name = doc.getElementsByTagName("Name")[0]?.textContent?.trim() || "読書リスト";
    const entries: CblEntry[] = Array.from(doc.getElementsByTagName("Book"), (el) => ({
      series: el.getAttribute("Series")?.trim() ?? "",
      number: el.getAttribute("Number")?.trim() ?? "",
      volume: el.getAttribute("Volume")?.trim() ?? "",
      year: el.getAttribute("Year")?.trim() ?? "",
    }));
    const matched: CblImportResult["matched"] = [];
    const unmatched: CblEntry[] = [];
    for (const e of entries) {
      const hit = matchCbl(e);
      if (hit) matched.push({ entry: e, book: book(hit) });
      else unmatched.push(e);
    }
    if (dryRun) return delay<CblImportResult>({ name, readList: null, matched, unmatched });
    if (matched.length === 0) throw new ApiError(400, "一致する本がありませんでした");
    const readList: ReadList = { id: newId("rl"), name, summary: "", bookIds: [...new Set(matched.map((m) => m.book.id))] };
    db.readLists.push(readList);
    return delay<CblImportResult>({ name, readList, matched, unmatched });
  },
  readLists: () => delay(db.readLists),
  readList: (id) => delay(db.readLists.find((r) => r.id === id) ?? null),
  readListBooks: (id) => {
    const r = db.readLists.find((x) => x.id === id);
    if (!r) return fail(404, "リーディングリストが見つかりません");
    return delay(r.bookIds.map(findBook).filter((b) => b !== undefined).map(book));
  },
  createReadList: (body) => {
    const r: ReadList = { id: newId("rl"), name: body.name.trim(), summary: body.summary ?? "", bookIds: body.bookIds ?? [] };
    db.readLists.push(r);
    return delay(r);
  },
  updateReadList: (id, body) => {
    const r = db.readLists.find((x) => x.id === id);
    if (!r) return fail(404, "リーディングリストが見つかりません");
    Object.assign(r, body);
    return delay(r);
  },
  deleteReadList: (id) => {
    const i = db.readLists.findIndex((x) => x.id === id);
    if (i >= 0) db.readLists.splice(i, 1);
    return delay(undefined);
  },

  addReadListBooks: (id, bookIds) => {
    const r = db.readLists.find((x) => x.id === id);
    if (!r) return fail(404, "リーディングリストが見つかりません");
    if (bookIds.some((x) => !findBook(x))) return fail(404, "本が見つかりません");
    for (const x of bookIds) if (!r.bookIds.includes(x)) r.bookIds.push(x);
    return delay(r);
  },
  addReadListBook: (id, bookId) => {
    const r = db.readLists.find((x) => x.id === id);
    if (!r) return fail(404, "リーディングリストが見つかりません");
    if (!findBook(bookId)) return fail(404, "本が見つかりません");
    if (!r.bookIds.includes(bookId)) r.bookIds.push(bookId);
    return delay(r);
  },
  removeReadListBook: (id, bookId) => {
    const r = db.readLists.find((x) => x.id === id);
    if (!r) return fail(404, "リーディングリストが見つかりません");
    r.bookIds = r.bookIds.filter((x) => x !== bookId);
    return delay(r);
  },
  reorderReadList: (id, bookIds) => {
    const r = db.readLists.find((x) => x.id === id);
    if (!r) return fail(404, "リーディングリストが見つかりません");
    if (!sameMembers(bookIds, r.bookIds)) return fail(409, "リーディングリストの内容が変わりました");
    r.bookIds = [...bookIds];
    return delay(r);
  },

  readerPrefs: () => delay(loadPrefs()),
  putGlobalPrefs: (p: GlobalReaderPrefs, profile: ReaderProfile = "desktop") =>
    savePrefs((all) => void (profile === "mobile" ? (all.globalMobile = p) : (all.global = p))),
  putSeriesPrefs: (id: string, p: ReaderPrefs) => savePrefs((all) => void (all.series[id] = p)),
  deleteSeriesPrefs: (id: string) => savePrefs((all) => void delete all.series[id]),
  putBookPrefs: (id: string, p: ReaderPrefs) => savePrefs((all) => void (all.books[id] = p)),
  deleteBookPrefs: (id: string) => savePrefs((all) => void delete all.books[id]),

  users: () => delay(db.users),
  createUser: (body) => {
    const u: User = {
      id: newId("u"), name: body.name, email: body.email, role: body.role, canDownload: body.canDownload ?? true,
      canStream: body.canStream ?? true, allLibraries: body.allLibraries ?? true, libraryIds: body.libraryIds ?? [], lastActive: null, locale: "",
    };
    db.users.push(u);
    return delay(u);
  },
  updateUser: (id, body) => {
    const u = db.users.find((x) => x.id === id);
    if (!u) return fail(404, "ユーザーが見つかりません");
    const { password: _password, ...rest } = body;
    Object.assign(u, rest);
    return delay(u);
  },
  deleteUser: (id) => {
    const i = db.users.findIndex((x) => x.id === id);
    if (i >= 0) db.users.splice(i, 1);
    return delay(undefined);
  },
  tasks: () => delay(db.tasks),
  cancelTask: (id) => {
    const t = db.tasks.find((x) => x.id === id);
    if (t && (t.state === "queued" || t.state === "running")) t.state = "cancelled";
    return delay(undefined);
  },
  mediaIssues: () => delay(db.mediaIssues),
  emptyTrash: () => delay({ series: 0, books: 0 }),
  settings: () => delay(loadSettings()),
  saveSettings: (changes) => {
    const next = { ...loadSettings(), ...changes };
    localStorage.setItem(SETTINGS_KEY, JSON.stringify(next));
    return delay(next);
  },
};
