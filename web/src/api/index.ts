// The only module screens talk to for data. Today it reads the mock store;
// later each function becomes a fetch() against the Go server with the same
// signature. Keep screens free of direct "@/mock" imports.
import { useEffect, useState, type DependencyList } from "react";
import * as db from "@/mock/data";
import type { Book, Series, SeriesQuery } from "./types";

export * from "./types";
export * from "./reader-prefs";

const LATENCY = 120;
const delay = <T,>(v: T): Promise<T> =>
  new Promise((res) => setTimeout(() => res(structuredClone(v)), LATENCY));

function readState(s: Series) {
  if (s.booksRead === s.bookCount) return "read" as const;
  if (s.booksRead > 0 || s.booksInProgress > 0) return "inProgress" as const;
  return "unread" as const;
}

export const api = {
  me: () => delay(db.currentUser),
  libraries: () => delay(db.libraries),
  library: (id: string) => delay(db.libraries.find((l) => l.id === id) ?? null),

  series: (q: SeriesQuery = {}) => {
    let xs = db.series.slice();
    if (q.libraryId) xs = xs.filter((s) => s.libraryId === q.libraryId);
    if (q.search) {
      const t = q.search.toLowerCase();
      xs = xs.filter((s) => s.title.toLowerCase().includes(t) || s.authors.some((a) => a.name.includes(q.search!)));
    }
    if (q.status?.length) xs = xs.filter((s) => q.status!.includes(s.status));
    if (q.genres?.length) xs = xs.filter((s) => s.genres.some((g) => q.genres!.includes(g)));
    if (q.readState?.length) xs = xs.filter((s) => q.readState!.includes(readState(s)));
    const sort = q.sort ?? "title";
    const key: Record<string, (s: Series) => string | number> = {
      title: (s) => s.titleSort,
      updated: (s) => s.updatedAt,
      created: (s) => s.createdAt,
      releaseDate: (s) => s.updatedAt,
      bookCount: (s) => s.bookCount,
    };
    xs.sort((a, b) => {
      const ka = key[sort](a), kb = key[sort](b);
      const c = typeof ka === "number" ? ka - (kb as number) : String(ka).localeCompare(String(kb), "ja");
      return q.desc ? -c : c;
    });
    return delay(xs);
  },
  seriesById: (id: string) => delay(db.series.find((s) => s.id === id) ?? null),
  booksOfSeries: (seriesId: string) =>
    delay(db.books.filter((b) => b.seriesId === seriesId).sort((a, b) => a.numberSort - b.numberSort)),
  book: (id: string) => delay(db.books.find((b) => b.id === id) ?? null),
  /** Neighbours inside the series, for "next book" in the reader. */
  adjacentBooks: (id: string) => {
    const b = db.books.find((x) => x.id === id);
    const sib = db.books.filter((x) => x.seriesId === b?.seriesId).sort((a, c) => a.numberSort - c.numberSort);
    const i = sib.findIndex((x) => x.id === id);
    return delay({ prev: sib[i - 1] ?? null, next: sib[i + 1] ?? null });
  },

  // Dashboard shelves
  keepReading: () =>
    delay(
      db.books
        .filter((b) => b.progress && !b.progress.completed)
        .sort((a, b) => b.progress!.readAt.localeCompare(a.progress!.readAt))
        .slice(0, 12),
    ),
  onDeck: () => {
    // First unread book of series where the previous one is finished.
    const out: Book[] = [];
    for (const s of db.series) {
      const bs = db.books.filter((b) => b.seriesId === s.id).sort((a, b) => a.numberSort - b.numberSort);
      const i = bs.findIndex((b) => !b.progress);
      if (i > 0 && bs[i - 1].progress?.completed) out.push(bs[i]);
    }
    return delay(out.slice(0, 12));
  },
  recentlyAddedBooks: () => delay(db.books.slice().sort((a, b) => b.createdAt.localeCompare(a.createdAt)).slice(0, 16)),
  recentlyAddedSeries: () => delay(db.series.slice().sort((a, b) => b.createdAt.localeCompare(a.createdAt)).slice(0, 12)),
  recentlyUpdatedSeries: () => delay(db.series.slice().sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)).slice(0, 12)),
  recentlyRead: () =>
    delay(
      db.books
        .filter((b) => b.progress?.completed)
        .sort((a, b) => b.progress!.readAt.localeCompare(a.progress!.readAt))
        .slice(0, 12),
    ),

  collections: () => delay(db.collections),
  collection: (id: string) => delay(db.collections.find((c) => c.id === id) ?? null),
  readLists: () => delay(db.readLists),
  readList: (id: string) => delay(db.readLists.find((c) => c.id === id) ?? null),
  booksByIds: (ids: string[]) => delay(ids.map((id) => db.books.find((b) => b.id === id)).filter(Boolean) as Book[]),
  seriesByIds: (ids: string[]) => delay(ids.map((id) => db.series.find((s) => s.id === id)).filter(Boolean) as Series[]),

  search: async (q: string) => {
    const t = q.trim().toLowerCase();
    if (!t) return delay({ series: [], books: [], collections: [], readLists: [] });
    return delay({
      series: db.series.filter((s) => s.title.toLowerCase().includes(t) || s.authors.some((a) => a.name.toLowerCase().includes(t))),
      books: db.books.filter((b) => b.title.toLowerCase().includes(t) || b.isbn.includes(t)).slice(0, 24),
      collections: db.collections.filter((c) => c.name.toLowerCase().includes(t)),
      readLists: db.readLists.filter((c) => c.name.toLowerCase().includes(t)),
    });
  },

  users: () => delay(db.users),
  tasks: () => delay(db.tasks),
  mediaIssues: () => delay(db.mediaIssues),
  settings: () => delay(db.settings),
};

/** Minimal async hook for the mock era. Swap for TanStack Query with the real API. */
export function useApi<T>(fn: () => Promise<T>, deps: DependencyList): { data: T | undefined; loading: boolean } {
  const [state, setState] = useState<{ data: T | undefined; loading: boolean }>({ data: undefined, loading: true });
  useEffect(() => {
    let live = true;
    // New parameters mean new data: never show the previous request's result
    // under the new heading (search query, library id, …).
    setState({ data: undefined, loading: true });
    fn().then((data) => live && setState({ data, loading: false }));
    return () => {
      live = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
  return state;
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
