// Reader display preferences. The global set is the master. A series can keep
// its own set (shared by all its volumes), and a single volume can keep its
// own on top of that: volume > series > global. They are per-user settings on
// the server; loadReaderPrefs() fetches them once after sign-in, and the
// getters below then answer synchronously, because the reader needs them
// before its first frame. Setters update the cache at once and write through.
import { api } from "./impl";
import type { ReadingDirection } from "./types";

export interface ReaderPrefs {
  direction: ReadingDirection;
  spread: "off" | "on" | "cover";
  fit: "screen" | "width" | "height" | "original";
  bg: "black" | "gray" | "white";
}

/** The master set. Its direction may defer to the series' own metadata
 *  (manga right-to-left, webtoon vertical scroll), which is the default. */
export interface GlobalReaderPrefs extends Omit<ReaderPrefs, "direction"> {
  direction: ReadingDirection | "series";
}

export type SeriesReaderPrefs = { mode: "global" } | { mode: "custom"; prefs: ReaderPrefs };

/** Where a volume's settings come from. */
export type ReaderScope = "global" | "series" | "book";

export const DEFAULT_GLOBAL: GlobalReaderPrefs = { direction: "series", spread: "off", fit: "screen", bg: "black" };

const cache = {
  global: DEFAULT_GLOBAL,
  series: new Map<string, ReaderPrefs>(),
  books: new Map<string, ReaderPrefs>(),
};
let loading: Promise<void> | null = null;

/** Fetches the caller's settings once; later calls reuse the first answer. */
export function loadReaderPrefs(): Promise<void> {
  loading ??= api.readerPrefs().then(
    (all) => {
      cache.global = { ...DEFAULT_GLOBAL, ...all.global };
      cache.series = new Map(Object.entries(all.series));
      cache.books = new Map(Object.entries(all.books));
    },
    (err: unknown) => {
      loading = null; // try again next time
      throw err;
    },
  );
  return loading;
}

export function getGlobalPrefs(): GlobalReaderPrefs {
  return cache.global;
}
/** One cached value and how to read and replace it. */
interface Slot<V> {
  get: () => V;
  set: (v: V) => void;
}

/** Per setting: writes run one at a time in order; acked is the last value
 *  the server accepted; seq tells the newest write apart. */
const queues = new Map<string, { chain: Promise<void>; acked: unknown; seq: number }>();

/** Applies a change to the cache at once and writes it through. Writes to the
 *  same setting reach the server in order. When one fails and no newer one is
 *  waiting, the cache goes back to what the server last accepted (not to the
 *  value before this write, which a newer success may have replaced). */
function writeThrough<V>(key: string, slot: Slot<V>, next: V, send: () => Promise<void>): Promise<void> {
  let q = queues.get(key);
  if (!q) queues.set(key, (q = { chain: Promise.resolve(), acked: slot.get(), seq: 0 }));
  const queue = q;
  const n = ++queue.seq;
  slot.set(next);
  const run = queue.chain.then(send).then(
    () => void (queue.acked = next),
    (err: unknown) => {
      if (n === queue.seq) slot.set(queue.acked as V);
      throw err;
    },
  );
  queue.chain = run.catch(() => undefined);
  return run;
}

function setScoped(kind: string, map: Map<string, ReaderPrefs>, id: string, p: SeriesReaderPrefs, put: () => Promise<void>, del: () => Promise<void>) {
  const slot: Slot<ReaderPrefs | undefined> = { get: () => map.get(id), set: (v) => (v ? map.set(id, v) : map.delete(id)) };
  return p.mode === "global" ? writeThrough(`${kind}:${id}`, slot, undefined, del) : writeThrough(`${kind}:${id}`, slot, p.prefs, put);
}

export function setGlobalPrefs(p: GlobalReaderPrefs): Promise<void> {
  const slot: Slot<GlobalReaderPrefs> = { get: () => cache.global, set: (v) => (cache.global = v) };
  return writeThrough("global", slot, p, () => api.putGlobalPrefs(p));
}

const asScoped = (p: ReaderPrefs | undefined): SeriesReaderPrefs => (p ? { mode: "custom", prefs: p } : { mode: "global" });

export function getSeriesPrefs(seriesId: string): SeriesReaderPrefs {
  return asScoped(cache.series.get(seriesId));
}
export function setSeriesPrefs(seriesId: string, p: SeriesReaderPrefs): Promise<void> {
  return setScoped("series", cache.series, seriesId, p,
    () => api.putSeriesPrefs(seriesId, (p as { prefs: ReaderPrefs }).prefs), () => api.deleteSeriesPrefs(seriesId));
}

export function getBookPrefs(bookId: string): SeriesReaderPrefs {
  return asScoped(cache.books.get(bookId));
}
export function setBookPrefs(bookId: string, p: SeriesReaderPrefs): Promise<void> {
  return setScoped("book", cache.books, bookId, p,
    () => api.putBookPrefs(bookId, (p as { prefs: ReaderPrefs }).prefs), () => api.deleteBookPrefs(bookId));
}

/** Series ids that keep their own settings. */
export const customSeriesIds = () => [...cache.series.keys()];
/** Volume (book) ids that keep their own settings. */
export const customBookIds = () => [...cache.books.keys()];

export function scopeOf(series: SeriesReaderPrefs, book: SeriesReaderPrefs): ReaderScope {
  return book.mode === "custom" ? "book" : series.mode === "custom" ? "series" : "global";
}

/** The global set applied to one series. */
export function resolveGlobal(g: GlobalReaderPrefs, seriesDirection: ReadingDirection): ReaderPrefs {
  return { ...g, direction: g.direction === "series" ? seriesDirection : g.direction };
}

/** What the reader actually uses for a volume: its own, else its series', else global. */
export function effectivePrefs(series: SeriesReaderPrefs, book: SeriesReaderPrefs, g: GlobalReaderPrefs, seriesDirection: ReadingDirection): ReaderPrefs {
  if (book.mode === "custom") return book.prefs;
  if (series.mode === "custom") return series.prefs;
  return resolveGlobal(g, seriesDirection);
}

export const DIRECTION_LABEL: Record<ReadingDirection, string> = { rtl: "右→左", ltr: "左→右", vertical: "縦", webtoon: "縦スクロール" };
export const SPREAD_LABEL: Record<ReaderPrefs["spread"], string> = { off: "単ページ", on: "見開き", cover: "表紙だけ単ページ" };
export const FIT_LABEL: Record<ReaderPrefs["fit"], string> = { screen: "画面", width: "幅", height: "高さ", original: "原寸" };
export const BG_LABEL: Record<ReaderPrefs["bg"], string> = { black: "黒", gray: "グレー", white: "白" };
