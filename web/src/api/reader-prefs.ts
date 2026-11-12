// Reader display preferences. The global set is the master. A series can keep
// its own set (shared by all its volumes), and a single volume can keep its
// own on top of that: volume > series > global. They are per-user settings on
// the server; loadReaderPrefs() fetches them once after sign-in, and the
// getters below then answer synchronously, because the reader needs them
// before its first frame. Setters update the cache at once and write through.
//
// The global set comes in two profiles, desktop and mobile; each browser
// reads with one of them (chosen on the settings page, detected by default).
// Series and volume settings are shared by both.
import { api } from "./impl";
import type { ReadingDirection } from "./types";

export interface ReaderPrefs {
  direction: ReadingDirection;
  spread: "off" | "on" | "cover";
  fit: "screen" | "width" | "height" | "original";
  bg: "black" | "gray" | "white";
}

/** The master set. Its direction may defer to the series' own metadata
 *  (manga right-to-left, webtoon vertical scroll), which is the default.
 *  splitWide: on a portrait screen, a landscape page (a scanned spread) is
 *  shown as its two halves, one after the other. */
export interface GlobalReaderPrefs extends Omit<ReaderPrefs, "direction"> {
  direction: ReadingDirection | "series";
  splitWide?: boolean;
}

export type ReaderProfile = "desktop" | "mobile";
/** What this browser uses: a profile, or "auto" (detected). */
export type ReaderProfileChoice = ReaderProfile | "auto";

export type SeriesReaderPrefs = { mode: "global" } | { mode: "custom"; prefs: ReaderPrefs };

/** Where a volume's settings come from. */
export type ReaderScope = "global" | "series" | "book";

export const DEFAULT_GLOBAL: GlobalReaderPrefs = { direction: "series", spread: "off", fit: "screen", bg: "black" };

const cache = {
  global: DEFAULT_GLOBAL,
  globalMobile: undefined as GlobalReaderPrefs | undefined,
  series: new Map<string, ReaderPrefs>(),
  books: new Map<string, ReaderPrefs>(),
};
let loading: Promise<void> | null = null;

const listeners = new Set<() => void>();
/** Calls fn after every change to the cached settings: a load, a change, or
 *  a rollback after the server refused one. Returns the unsubscribe. */
export function subscribeReaderPrefs(fn: () => void): () => void {
  listeners.add(fn);
  return () => listeners.delete(fn);
}
const notify = () => listeners.forEach((fn) => fn());

/** Fetches the caller's settings once; later calls reuse the first answer. */
export function loadReaderPrefs(): Promise<void> {
  loading ??= api.readerPrefs().then(
    (all) => {
      cache.global = { ...DEFAULT_GLOBAL, ...all.global };
      cache.globalMobile = all.globalMobile ? { ...DEFAULT_GLOBAL, ...all.globalMobile } : undefined;
      cache.series = new Map(Object.entries(all.series));
      cache.books = new Map(Object.entries(all.books));
      notify();
    },
    (err: unknown) => {
      loading = null; // try again next time
      throw err;
    },
  );
  return loading;
}

const PROFILE_KEY = "cominavi.readerProfile";

/** A touch screen without hover: a phone or a tablet. */
export function detectedProfile(): ReaderProfile {
  return typeof matchMedia !== "undefined" && matchMedia("(pointer: coarse) and (hover: none)").matches ? "mobile" : "desktop";
}

export function getProfileChoice(): ReaderProfileChoice {
  try {
    const v = localStorage.getItem(PROFILE_KEY);
    if (v === "desktop" || v === "mobile") return v;
  } catch {
    // storage blocked: detect
  }
  return "auto";
}

export function setProfileChoice(c: ReaderProfileChoice): void {
  try {
    if (c === "auto") localStorage.removeItem(PROFILE_KEY);
    else localStorage.setItem(PROFILE_KEY, c);
  } catch {
    // storage blocked: the choice lasts for this page only
  }
  notify();
}

/** The profile this browser reads with. */
export function activeProfile(): ReaderProfile {
  const c = getProfileChoice();
  return c === "auto" ? detectedProfile() : c;
}

/** The global set of a profile (this browser's when omitted). A mobile
 *  profile that was never saved starts from the desktop one, single page. */
export function getGlobalPrefs(profile: ReaderProfile = activeProfile()): GlobalReaderPrefs {
  if (profile === "desktop") return cache.global;
  return cache.globalMobile ?? { ...cache.global, spread: "off", splitWide: false };
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
  notify();
  const run = queue.chain.then(send).then(
    () => void (queue.acked = next),
    (err: unknown) => {
      if (n === queue.seq) {
        slot.set(queue.acked as V);
        notify();
      }
      throw err;
    },
  );
  queue.chain = run.catch(() => undefined);
  return run;
}

function setScoped(kind: "series" | "books", id: string, p: SeriesReaderPrefs, put: () => Promise<void>, del: () => Promise<void>) {
  // cache[kind], not a Map captured now: loadReaderPrefs() replaces the Maps.
  const slot: Slot<ReaderPrefs | undefined> = {
    get: () => cache[kind].get(id),
    set: (v) => (v ? cache[kind].set(id, v) : cache[kind].delete(id)),
  };
  return p.mode === "global" ? writeThrough(`${kind}:${id}`, slot, undefined, del) : writeThrough(`${kind}:${id}`, slot, p.prefs, put);
}

export function setGlobalPrefs(p: GlobalReaderPrefs, profile: ReaderProfile = activeProfile()): Promise<void> {
  if (profile === "mobile") {
    const slot: Slot<GlobalReaderPrefs | undefined> = { get: () => cache.globalMobile, set: (v) => (cache.globalMobile = v) };
    return writeThrough("global:mobile", slot, p, () => api.putGlobalPrefs(p, "mobile"));
  }
  const slot: Slot<GlobalReaderPrefs> = { get: () => cache.global, set: (v) => (cache.global = v) };
  return writeThrough("global", slot, p, () => api.putGlobalPrefs(p, "desktop"));
}

const asScoped = (p: ReaderPrefs | undefined): SeriesReaderPrefs => (p ? { mode: "custom", prefs: p } : { mode: "global" });

export function getSeriesPrefs(seriesId: string): SeriesReaderPrefs {
  return asScoped(cache.series.get(seriesId));
}
export function setSeriesPrefs(seriesId: string, p: SeriesReaderPrefs): Promise<void> {
  return setScoped("series", seriesId, p,
    () => api.putSeriesPrefs(seriesId, (p as { prefs: ReaderPrefs }).prefs), () => api.deleteSeriesPrefs(seriesId));
}

export function getBookPrefs(bookId: string): SeriesReaderPrefs {
  return asScoped(cache.books.get(bookId));
}
export function setBookPrefs(bookId: string, p: SeriesReaderPrefs): Promise<void> {
  return setScoped("books", bookId, p,
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
export const PROFILE_LABEL: Record<ReaderProfile, string> = { desktop: "PC", mobile: "モバイル" };
/** "PC 用" / "モバイル用": a space only after Latin letters. */
export const PROFILE_FOR_LABEL: Record<ReaderProfile, string> = { desktop: "PC 用", mobile: "モバイル用" };
export const BG_LABEL: Record<ReaderPrefs["bg"], string> = { black: "黒", gray: "グレー", white: "白" };
