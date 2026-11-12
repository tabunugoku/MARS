// The only module screens talk to for data: the API (Go server, or the mock
// with VITE_API=mock), its types, reader settings and small formatters.
// Keep screens free of direct "@/mock" imports.
import { useEffect, useRef, useState, type DependencyList } from "react";
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
