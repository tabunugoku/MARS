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
export function setGlobalPrefs(p: GlobalReaderPrefs): Promise<void> {
  cache.global = p;
  return api.putGlobalPrefs(p);
}

const asScoped = (p: ReaderPrefs | undefined): SeriesReaderPrefs => (p ? { mode: "custom", prefs: p } : { mode: "global" });

export function getSeriesPrefs(seriesId: string): SeriesReaderPrefs {
  return asScoped(cache.series.get(seriesId));
}
export function setSeriesPrefs(seriesId: string, p: SeriesReaderPrefs): Promise<void> {
  if (p.mode === "global") {
    cache.series.delete(seriesId);
    return api.deleteSeriesPrefs(seriesId);
  }
  cache.series.set(seriesId, p.prefs);
  return api.putSeriesPrefs(seriesId, p.prefs);
}

export function getBookPrefs(bookId: string): SeriesReaderPrefs {
  return asScoped(cache.books.get(bookId));
}
export function setBookPrefs(bookId: string, p: SeriesReaderPrefs): Promise<void> {
  if (p.mode === "global") {
    cache.books.delete(bookId);
    return api.deleteBookPrefs(bookId);
  }
  cache.books.set(bookId, p.prefs);
  return api.putBookPrefs(bookId, p.prefs);
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
