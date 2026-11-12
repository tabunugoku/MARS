// The only module screens talk to for data: the API (Go server, or the mock
// with VITE_API=mock), its types, reader settings and small formatters.
// Keep screens free of direct "@/mock" imports.
import { useCallback, useEffect, useRef, useState, type DependencyList } from "react";
import { ApiError } from "./http";
import type { Series } from "./types";

export * from "./types";
export * from "./reader-prefs";
export { api } from "./impl";
export type { Api } from "./real";
export { ApiError, safeNext } from "./http";

/** Download links (ファイルのダウンロード). Plain URLs, not fetch calls: the
 *  browser streams the file straight to disk, which a series ZIP of many GB
 *  needs. The session cookie goes along with a same-origin link. */
export const bookFileUrl = (bookId: string) => `/api/v1/books/${encodeURIComponent(bookId)}/file`;
export const seriesFileUrl = (seriesId: string) => `/api/v1/series/${encodeURIComponent(seriesId)}/file`;

/** Starts a download without leaving the page. If the server refuses, the
 *  browser shows a failed download rather than an error page. */
export function startDownload(url: string) {
  const a = document.createElement("a");
  a.href = url;
  a.download = "";
  a.rel = "noopener";
  document.body.append(a);
  a.click();
  a.remove();
}

/** The message to show for a failed call. */
export function errorMessage(err: unknown): string {
  return err instanceof ApiError ? err.message : "サーバーに接続できませんでした";
}

/** Loads data for a screen. A new deps list starts over (never shows the
 *  previous request's result under a new heading). error is set when the
 *  call failed; reload() runs it again. */
export function useApi<T>(
  fn: () => Promise<T>,
  deps: DependencyList,
): { data: T | undefined; loading: boolean; error: string | null; reload: () => void } {
  const [state, setState] = useState<{ data: T | undefined; loading: boolean; error: string | null }>({
    data: undefined,
    loading: true,
    error: null,
  });
  const [tick, setTick] = useState(0);
  // A reload keeps the current data on screen; new deps (another id) never do.
  const last = useRef<{ tick: number; deps: DependencyList }>({ tick, deps });
  useEffect(() => {
    let live = true;
    const prev = last.current;
    const sameDeps = prev.deps.length === deps.length && prev.deps.every((d, i) => Object.is(d, deps[i]));
    const isReload = tick !== prev.tick && sameDeps;
    last.current = { tick, deps };
    setState((s) => (isReload ? { ...s, loading: true } : { data: undefined, loading: true, error: null }));
    fn().then(
      (data) => live && setState({ data, loading: false, error: null }),
      (err: unknown) => live && setState({ data: undefined, loading: false, error: errorMessage(err) }),
    );
    return () => {
      live = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, tick]);
  return { ...state, reload: () => setTick((t) => t + 1) };
}

/** One page of a list and the length of the whole list (from the server). */
export interface Paged<T> {
  items: T[];
  total: number;
}

/** Loads a long list a page at a time. A new deps list starts over from the
 *  first page; loadMore() appends the next one (ignored while a page is
 *  loading, after an error, or at the end). total is the server's count, not
 *  items.length. reload() fetches the pages already shown again in one call,
 *  keeping them on screen meanwhile; retry() repeats the call that failed. */
export function usePagedApi<T extends { id: string }>(
  fetchPage: (offset: number, limit: number) => Promise<Paged<T>>,
  deps: DependencyList,
  pageSize = 60,
): {
  items: T[] | undefined;
  total: number;
  loading: boolean;
  error: string | null;
  hasMore: boolean;
  loadMore: () => void;
  reload: () => void;
  retry: () => void;
} {
  const [state, setState] = useState<{
    items: T[] | undefined;
    total: number;
    loading: boolean;
    error: string | null;
    hasMore: boolean;
  }>({ items: undefined, total: 0, loading: true, error: null, hasMore: false });
  // The latest fetchPage (it closes over the current filters); the list as
  // it stands, kept in refs so loadMore never asks for the same page twice;
  // a generation that drops answers meant for an older list.
  const fetchRef = useRef(fetchPage);
  useEffect(() => {
    fetchRef.current = fetchPage;
  });
  type Req = { offset: number; limit: number; replace: boolean };
  // next is the server offset to ask for next. It counts rows the server
  // sent, not unique items shown: when series come and go between pages a
  // page can repeat rows already shown, and counting shown items would ask
  // for the same rows again forever. failed keeps the request to repeat.
  const empty = () => ({
    items: undefined as T[] | undefined,
    total: 0,
    next: 0,
    end: false,
    busy: false,
    failed: null as Req | null,
  });
  const list = useRef(empty());
  const gen = useRef(0);

  const load = useCallback((req: Req): void => {
    const g = gen.current;
    Object.assign(list.current, { busy: true, failed: null });
    setState((s) => ({ ...s, loading: true, error: null }));
    fetchRef.current(req.offset, req.limit).then(
      (page) => {
        if (g !== gen.current) return;
        let items = page.items;
        const prev = list.current.items;
        let added = items.length;
        if (!req.replace && prev) {
          // Offsets shift when series come and go between pages: keep the first copy.
          const seen = new Set(prev.map((x) => x.id));
          const fresh = page.items.filter((x) => !seen.has(x.id));
          added = fresh.length;
          items = [...prev, ...fresh];
        }
        const next = req.offset + page.items.length;
        const end = page.items.length === 0 || next >= page.total;
        list.current = { items, total: page.total, next, end, busy: false, failed: null };
        setState({ items, total: page.total, loading: false, error: null, hasMore: !end });
        // A page of nothing but repeats changes nothing on screen, so the
        // scroll sentinel would not fire again: fetch the next one now.
        if (!req.replace && added === 0 && !end) load({ offset: next, limit: req.limit, replace: false });
      },
      (err: unknown) => {
        if (g !== gen.current) return;
        Object.assign(list.current, { busy: false, failed: req });
        setState((s) => ({ ...s, loading: false, error: errorMessage(err) }));
      },
    );
  }, []);

  useEffect(() => {
    gen.current++;
    list.current = empty();
    setState({ items: undefined, total: 0, loading: true, error: null, hasMore: false });
    load({ offset: 0, limit: pageSize, replace: true });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, pageSize]);

  const loadMore = () => {
    const l = list.current;
    if (l.busy || l.failed || !l.items || l.end) return;
    load({ offset: l.next, limit: pageSize, replace: false });
  };
  const reload = () => {
    gen.current++;
    const n = list.current.items?.length ?? 0;
    load({ offset: 0, limit: Math.min(Math.max(n, pageSize), 1000), replace: true });
  };
  // Repeat exactly what failed: a failed reload stays a reload, so its
  // answer replaces the stale list instead of being appended to it.
  const retry = () => {
    const l = list.current;
    if (l.busy) return;
    if (l.failed) load(l.failed);
    else if (!l.items) reload();
  };
  return { ...state, loadMore, reload, retry };
}

export const formatBytes = (n: number) => {
  const u = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) (n /= 1024), i++;
  return `${n.toFixed(i > 1 ? 1 : 0)} ${u[i]}`;
};
export const formatDate = (iso: string) =>
  new Intl.DateTimeFormat("ja-JP", { year: "numeric", month: "short", day: "numeric" }).format(new Date(iso));
export const statusLabel: Record<Series["status"], string> = {
  ongoing: "連載中",
  ended: "完結",
  hiatus: "休載中",
  abandoned: "打ち切り",
};
