// Reader display preferences. The global set is the master. A series can keep
// its own set (shared by all its volumes), and a single volume can keep its
// own on top of that: volume > series > global. Synchronous on purpose: the
// reader needs them before its first frame. Today they live in localStorage;
// with the Go server they become per-user settings (GET/PUT
// /api/me/reader-prefs, /api/me/series/:id/reader-prefs and
// /api/me/books/:id/reader-prefs) cached here.
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

const GLOBAL_KEY = "cn-reader-global";
const SERIES_KEY = (id: string) => `cn-reader-series-${id}`;
const SERIES_PREFIX = "cn-reader-series-";
const BOOK_PREFIX = "cn-reader-book-";

export const DEFAULT_GLOBAL: GlobalReaderPrefs = { direction: "series", spread: "off", fit: "screen", bg: "black" };

function read<T>(key: string): T | null {
  try {
    const raw = localStorage.getItem(key);
    return raw ? (JSON.parse(raw) as T) : null;
  } catch {
    return null;
  }
}

export function getGlobalPrefs(): GlobalReaderPrefs {
  return { ...DEFAULT_GLOBAL, ...read<Partial<GlobalReaderPrefs>>(GLOBAL_KEY) };
}
export function setGlobalPrefs(p: GlobalReaderPrefs) {
  localStorage.setItem(GLOBAL_KEY, JSON.stringify(p));
}

export function getSeriesPrefs(seriesId: string): SeriesReaderPrefs {
  const v = read<SeriesReaderPrefs>(SERIES_KEY(seriesId));
  return v?.mode === "custom" && v.prefs ? v : { mode: "global" };
}
export function setSeriesPrefs(seriesId: string, p: SeriesReaderPrefs) {
  if (p.mode === "global") localStorage.removeItem(SERIES_KEY(seriesId));
  else localStorage.setItem(SERIES_KEY(seriesId), JSON.stringify(p));
}

export function getBookPrefs(bookId: string): SeriesReaderPrefs {
  const v = read<SeriesReaderPrefs>(BOOK_PREFIX + bookId);
  return v?.mode === "custom" && v.prefs ? v : { mode: "global" };
}
export function setBookPrefs(bookId: string, p: SeriesReaderPrefs) {
  if (p.mode === "global") localStorage.removeItem(BOOK_PREFIX + bookId);
  else localStorage.setItem(BOOK_PREFIX + bookId, JSON.stringify(p));
}

function idsWith(prefix: string) {
  const ids: string[] = [];
  for (let i = 0; i < localStorage.length; i++) {
    const k = localStorage.key(i);
    if (k?.startsWith(prefix) && read<SeriesReaderPrefs>(k)?.mode === "custom") ids.push(k.slice(prefix.length));
  }
  return ids;
}
/** Series ids that keep their own settings. */
export const customSeriesIds = () => idsWith(SERIES_PREFIX);
/** Volume (book) ids that keep their own settings. */
export const customBookIds = () => idsWith(BOOK_PREFIX);

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
