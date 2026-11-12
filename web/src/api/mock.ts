// The mock server for `VITE_API=mock` (design work without the Go server).
// It is loaded with import() only in that mode, so neither it nor the
// generated art reaches a production build. Writes change the in-memory data
// and are lost on reload, except settings and reader prefs (localStorage).
import * as db from "@/mock/data";
import { coverArt, pageArt } from "@/lib/art";
import { ApiError } from "./http";
import type { Api } from "./real";
import type {
  Book, Collection, Library, LoginScreen, ReaderPrefsAll, ReadList, Series, SeriesQuery, ServerSettings, User,
} from "./types";
import type { GlobalReaderPrefs, ReaderPrefs } from "./reader-prefs";

const LATENCY = 120;
const delay = <T,>(v: T): Promise<T> => new Promise((res) => setTimeout(() => res(structuredClone(v)), LATENCY));
const fail = (status: number, message: string) => Promise.reject(new ApiError(status, message));
const now = () => new Date().toISOString();
let nextId = 1;
const newId = (p: string) => `${p}-new-${nextId++}`;

const series = (s: db.MockSeries): Series => ({ ...s, thumbnailUrl: coverArt(s.coverSeed, s.title) });
const findSeries = (id: string) => db.series.find((s) => s.id === id);
const book = (b: db.MockBook): Book => ({
  ...b,
  seriesTitle: findSeries(b.seriesId)?.title ?? b.seriesTitle,
  thumbnailUrl: coverArt(b.coverSeed, b.title, String(b.number)),
});
const findBook = (id: string) => db.books.find((b) => b.id === id);
const seriesBooks = (id: string) => db.books.filter((b) => b.seriesId === id).sort((a, b) => a.numberSort - b.numberSort);

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
        : [];
    return delay({ backdrop, covers, needsSetup: false });
  },
  login: () => delay(me()),
  setup: () => delay(me()),
  logout: () => delay(undefined),

  me: () => delay(me()),
  updateMe: (name) => {
    db.currentUser.name = name.trim();
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
  seriesById: (id) => {
    const s = findSeries(id);
    return delay(s ? series(s) : null);
  },
  seriesByIds: (ids) => delay(ids.map(findSeries).filter((s) => s !== undefined).map(series)),
  updateSeries: (id, changes) => {
    const s = findSeries(id);
    if (!s) return fail(404, "シリーズが見つかりません");
    const { lockedFields, ...fields } = changes;
    Object.assign(s, fields);
    s.lockedFields = lockedFields ?? [...new Set([...s.lockedFields, ...Object.keys(fields)])];
    s.updatedAt = now();
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
      Array.from({ length: b.pageCount }, (_, i) => ({
        number: i + 1, width: w, height: h, mediaType: "image/svg+xml", url: pageArt(b.coverSeed, i + 1, { webtoon }),
      })),
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

  readerPrefs: () => delay(loadPrefs()),
  putGlobalPrefs: (p: GlobalReaderPrefs) => savePrefs((all) => void (all.global = p)),
  putSeriesPrefs: (id: string, p: ReaderPrefs) => savePrefs((all) => void (all.series[id] = p)),
  deleteSeriesPrefs: (id: string) => savePrefs((all) => void delete all.series[id]),
  putBookPrefs: (id: string, p: ReaderPrefs) => savePrefs((all) => void (all.books[id] = p)),
  deleteBookPrefs: (id: string) => savePrefs((all) => void delete all.books[id]),

  users: () => delay(db.users),
  createUser: (body) => {
    const u: User = {
      id: newId("u"), name: body.name, email: body.email, role: body.role, canDownload: body.canDownload ?? true,
      canStream: body.canStream ?? true, allLibraries: body.allLibraries ?? true, libraryIds: body.libraryIds ?? [], lastActive: null,
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
