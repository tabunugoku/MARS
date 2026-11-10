import { useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from "react";
import { Link, useSearchParams } from "react-router";
import { Clock, Search, X } from "@keyline-icons/react";
import { api, useApi } from "@/api";
import { BookCard, SeriesCard } from "@/components/media/cards";
import { CoverGrid, EmptyState, Page, SectionTitle, Shelf, ShelfItem } from "@/components/media/layout";
import { GridSkeleton } from "@/components/media/skeletons";
import { Button } from "@/components/ui/button";
import Input from "@/components/ui/input";
import { Tabs, TabsItem, TabsPanel } from "@/components/ui/tabs";
import { Tooltip } from "@/components/ui/tooltip";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";

const RECENT_KEY = "cn-recent-searches";
const RECENT_MAX = 8;

function readRecent(): string[] {
  try {
    const v: unknown = JSON.parse(localStorage.getItem(RECENT_KEY) ?? "[]");
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string").slice(0, RECENT_MAX) : [];
  } catch {
    return [];
  }
}

function writeRecent(xs: string[]) {
  try {
    if (xs.length) localStorage.setItem(RECENT_KEY, JSON.stringify(xs));
    else localStorage.removeItem(RECENT_KEY);
  } catch {
    /* storage unavailable: keep the list for this visit only */
  }
}

type TabKey = "all" | "series" | "books" | "collections" | "readLists";

interface ListRow {
  id: string;
  name: string;
  count: string;
  to: string;
}

function ListRows({ rows }: { rows: ListRow[] }) {
  return (
    <ul className="flex flex-col border-t border-border">
      {rows.map((r) => (
        <li key={r.id} className="border-b border-border">
          <Link
            to={r.to}
            className="flex items-baseline justify-between gap-4 py-3 outline-none hover:bg-hover focus-visible:ring-2 ring-[var(--focus-ring)] focus-visible:ring-inset"
          >
            <span className={cn(typeClass("body"), "truncate text-foreground")}>{r.name}</span>
            <span className={cn(typeClass("caption"), "shrink-0 text-muted-foreground tabular-nums")}>{r.count}</span>
          </Link>
        </li>
      ))}
    </ul>
  );
}

export default function Screen() {
  const [params, setParams] = useSearchParams();
  const q = params.get("q") ?? "";
  const term = q.trim();

  const [text, setText] = useState(q);
  // The ?q= value this component last wrote, or accepted from the URL.
  const lastQ = useRef(q);
  const writeTimer = useRef<number | undefined>(undefined);
  const [recent, setRecent] = useState<string[]>(readRecent);
  const [tab, setTab] = useState<TabKey>("all");

  // Series list: genre chips and series titles for book cards.
  const library = useApi(() => api.series(), []);
  const results = useApi(() => api.search(term), [term]);

  const titles = useMemo(() => new Map((library.data ?? []).map((s) => [s.id, s.title])), [library.data]);
  const genres = useMemo(
    () => [...new Set((library.data ?? []).flatMap((s) => s.genres))].sort((a, b) => a.localeCompare(b, "ja")),
    [library.data],
  );

  // External ?q= change (sidebar search, back/forward): adopt it and drop any
  // pending write of the old text so it cannot overwrite the new query.
  useEffect(() => {
    if (q === lastQ.current) return;
    window.clearTimeout(writeTimer.current);
    lastQ.current = q;
    setText(q);
  }, [q]);

  // Debounced write of the input into ?q= (replace, so back still leaves the page).
  useEffect(() => {
    if (text.trim() === term) return;
    writeTimer.current = window.setTimeout(() => {
      const nextQ = text.trim() ? text : "";
      lastQ.current = nextQ;
      setParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          if (nextQ) next.set("q", nextQ);
          else next.delete("q");
          return next;
        },
        { replace: true },
      );
    }, 200);
    return () => window.clearTimeout(writeTimer.current);
    // Only the typed text drives the write; `term` is read at the moment it changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [text]);

  // Unmount: never write a stale query after the page is gone.
  useEffect(() => () => window.clearTimeout(writeTimer.current), []);

  // A query that stays on screen for a second counts as a recent search.
  useEffect(() => {
    if (!term) return;
    const t = window.setTimeout(() => remember(term), 1000);
    return () => window.clearTimeout(t);
  }, [term]);

  function remember(v: string) {
    setRecent((prev) => {
      const next = [v, ...prev.filter((x) => x !== v)].slice(0, RECENT_MAX);
      writeRecent(next);
      return next;
    });
  }

  function forget(v: string) {
    setRecent((prev) => {
      const next = prev.filter((x) => x !== v);
      writeRecent(next);
      return next;
    });
  }

  function clearAll() {
    setRecent([]);
    writeRecent([]);
  }

  const submit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (text.trim()) remember(text.trim());
  };

  // Body by state: no query (recent + genres), loading, empty, or results.
  let body: ReactNode;
  const r = results.data;
  if (!term) {
    body = library.data ? (
      <>
        {recent.length > 0 && (
          <section className="mb-10">
            <SectionTitle action={<Button variant="ghost" size="compact" onClick={clearAll}>すべて消去</Button>}>最近の検索</SectionTitle>
            <ul className="flex flex-col">
              {recent.map((v) => (
                <li key={v} className="flex items-center gap-1">
                  <button
                    type="button"
                    onClick={() => setText(v)}
                    className="flex h-9 min-w-0 flex-1 items-center gap-2 rounded-md px-2 text-left text-foreground hover:bg-hover outline-none focus-visible:ring-2 ring-[var(--focus-ring)]"
                  >
                    <Clock size={16} aria-hidden className="shrink-0 text-muted-foreground" />
                    <span className={cn(typeClass("body"), "truncate")}>{v}</span>
                  </button>
                  <Tooltip content="削除">
                    <Button variant="ghost" size="icon" aria-label={`${v} を削除`} onClick={() => forget(v)}>
                      <X />
                    </Button>
                  </Tooltip>
                </li>
              ))}
            </ul>
          </section>
        )}
        {genres.length > 0 && (
          <section className="mb-10">
            <SectionTitle>ジャンル</SectionTitle>
            <div className="flex flex-wrap gap-2">
              {genres.map((g) => (
                <Link
                  key={g}
                  to={`/libraries/all?genre=${encodeURIComponent(g)}`}
                  className={cn(
                    typeClass("caption"),
                    "inline-flex h-7 items-center rounded-full border border-border px-3 text-foreground hover:bg-hover outline-none focus-visible:ring-2 ring-[var(--focus-ring)]",
                  )}
                >
                  {g}
                </Link>
              ))}
            </div>
          </section>
        )}
      </>
    ) : (
      <GridSkeleton count={6} />
    );
  } else if (!r) {
    body = <GridSkeleton count={6} />;
  } else {
    const counts = {
      series: r.series.length,
      books: r.books.length,
      collections: r.collections.length,
      readLists: r.readLists.length,
    };
    const total = counts.series + counts.books + counts.collections + counts.readLists;
    if (total === 0) {
      body = <EmptyState title={`「${term}」に一致するものはありません`} body="タイトル・著者名・ISBN で探せます" />;
    } else {
      const tabs: { key: TabKey; label: string }[] = [
        { key: "all", label: "すべて" },
        ...(counts.series ? [{ key: "series" as const, label: `シリーズ（${counts.series}）` }] : []),
        ...(counts.books ? [{ key: "books" as const, label: `巻（${counts.books}）` }] : []),
        ...(counts.collections ? [{ key: "collections" as const, label: `コレクション（${counts.collections}）` }] : []),
        ...(counts.readLists ? [{ key: "readLists" as const, label: `リーディングリスト（${counts.readLists}）` }] : []),
      ];
      const active = tabs.some((t) => t.key === tab) ? tab : "all";
      const sel = Math.max(0, tabs.findIndex((t) => t.key === active));

      const collectionRows: ListRow[] = r.collections.map((c) => ({
        id: c.id,
        name: c.name,
        count: `${c.seriesIds.length}件`,
        to: `/collections/${c.id}`,
      }));
      const readListRows: ListRow[] = r.readLists.map((l) => ({
        id: l.id,
        name: l.name,
        count: `${l.bookIds.length}件`,
        to: `/readlists/${l.id}`,
      }));

      const seriesGrid = (
        <CoverGrid>
          {r.series.map((s) => (
            <SeriesCard key={s.id} series={s} />
          ))}
        </CoverGrid>
      );
      const booksGrid = (
        <CoverGrid>
          {r.books.map((b) => (
            <BookCard key={b.id} book={b} seriesTitle={titles.get(b.seriesId)} />
          ))}
        </CoverGrid>
      );

      let panel: ReactNode;
      if (active === "series") panel = seriesGrid;
      else if (active === "books") panel = booksGrid;
      else if (active === "collections") panel = <ListRows rows={collectionRows} />;
      else if (active === "readLists") panel = <ListRows rows={readListRows} />;
      else
        panel = (
          <>
            {r.series.length > 0 && (
              <Shelf title="シリーズ">
                {r.series.map((s) => (
                  <ShelfItem key={s.id}>
                    <SeriesCard series={s} />
                  </ShelfItem>
                ))}
              </Shelf>
            )}
            {r.books.length > 0 && (
              <Shelf title="巻">
                {r.books.map((b) => (
                  <ShelfItem key={b.id}>
                    <BookCard book={b} seriesTitle={titles.get(b.seriesId)} />
                  </ShelfItem>
                ))}
              </Shelf>
            )}
            {collectionRows.length > 0 && (
              <section className="mb-10">
                <SectionTitle>コレクション</SectionTitle>
                <ListRows rows={collectionRows} />
              </section>
            )}
            {readListRows.length > 0 && (
              <section className="mb-10">
                <SectionTitle>リーディングリスト</SectionTitle>
                <ListRows rows={readListRows} />
              </section>
            )}
          </>
        );

      body = (
        <>
          <Tabs
            selectedIndex={sel}
            onSelect={(i) => setTab(tabs[i].key)}
            idPrefix="search"
            className="mb-6"
          >
            {tabs.map((t, i) => (
              <TabsItem key={t.key} label={t.label} index={i} />
            ))}
          </Tabs>
          <TabsPanel index={sel} selectedIndex={sel} idPrefix="search">
            {panel}
          </TabsPanel>
        </>
      );
    }
  }

  return (
    <Page>
      <h1 className="sr-only">検索</h1>

      <form role="search" onSubmit={submit} className="relative mb-6">
        <Search size={16} aria-hidden className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
        <Input
          type="text"
          autoFocus
          aria-label="検索"
          placeholder="タイトル・著者名・ISBN"
          value={text}
          onChange={(e) => setText(e.target.value)}
          className={cn(typeClass("title"), "h-11 pl-10 pr-10")}
        />
        {text && (
          <Tooltip content="クリア">
            <Button
              variant="ghost"
              size="icon"
              aria-label="検索語を消す"
              className="absolute right-1 top-1/2 -translate-y-1/2"
              onClick={() => setText("")}
            >
              <X />
            </Button>
          </Tooltip>
        )}
      </form>

      {body}
    </Page>
  );
}
