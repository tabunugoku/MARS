import type { IconComponent } from "@/lib/icon-context";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router";
import { toast } from "@yusufceng1z/ding-react";
import { ArrowUpDown, Circle, CircleCheck, MoreHorizontal, Scan, Settings, SlidersHorizontal, X } from "@keyline-icons/react";
import { api, errorMessage, formatBytes, statusLabel, useApi, usePagedApi, type SeriesSort, type SeriesStatus } from "@/api";
import { useLiveReload } from "@/api/live";
import { LoadError } from "@/components/shell/load-error";
import { SeriesCard } from "@/components/media/cards";
import { CountUp } from "@/components/media/count-up";
import { CoverGrid, EmptyState, Page, PageHeader } from "@/components/media/layout";
import { KindIcon } from "@/components/media/kind-icon";
import { CoverSkeleton, GridSkeleton } from "@/components/media/skeletons";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { DropdownContent, DropdownMenu, DropdownTrigger } from "@/components/ui/dropdown";
import { MenuItem } from "@/components/ui/menu-item";
import { Select, SelectContent, SelectItem, SelectTrigger } from "@/components/ui/select";
import { Tooltip } from "@/components/ui/tooltip";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { defineMessages, intlLocale, useT } from "@/i18n";

const m = defineMessages({
  ja: {
    allLibraries: "すべてのライブラリ",
    library: "ライブラリ",
    sortTitle: "タイトル",
    sortUpdated: "更新日",
    sortAdded: "追加日",
    sortBookCount: "巻数",
    readUnread: "未読",
    readInProgress: "読みかけ",
    readRead: "既読",
    moreActions: "その他の操作",
    scan: "スキャン",
    scanStarted: "スキャンを開始しました",
    markLibraryRead: "ライブラリを既読にする",
    markLibraryUnread: "ライブラリを未読にする",
    markedRead: "既読にしました",
    markedUnread: "未読にしました",
    libraryConfig: "ライブラリ設定",
    libraryNotFound: "ライブラリが見つかりません",
    libraryNotFoundBody: "ライブラリの一覧からもう一度選んでください",
    seriesLabel: "シリーズ",
    booksLabel: "本",
    seriesTotal: ({ n }) => `${n} シリーズ`,
    sortLabel: "並び順",
    sortReverse: "並び順を逆にする",
    sortAsc: "昇順",
    sortDesc: "降順",
    filters: "絞り込み",
    filtersCount: ({ n }) => `絞り込み ${n}`,
    readState: "既読状態",
    status: "ステータス",
    genres: "ジャンル",
    clear: "クリア",
    removeChip: "{label}を外す",
    noMatch: "条件に合うシリーズがありません",
    clearFilters: "絞り込みをクリア",
    noSeries: "シリーズがありません",
    noSeriesBody: "ライブラリをスキャンすると、ここに表示されます",
  },
  en: {
    allLibraries: "All libraries",
    library: "Library",
    sortTitle: "Title",
    sortUpdated: "Updated",
    sortAdded: "Added",
    sortBookCount: "Book count",
    readUnread: "Unread",
    readInProgress: "In progress",
    readRead: "Read",
    moreActions: "More actions",
    scan: "Scan",
    scanStarted: "Scan started",
    markLibraryRead: "Mark library as read",
    markLibraryUnread: "Mark library as unread",
    markedRead: "Marked as read",
    markedUnread: "Marked as unread",
    libraryConfig: "Library settings",
    libraryNotFound: "Library not found",
    libraryNotFoundBody: "Pick it again from the library list",
    seriesLabel: "Series",
    booksLabel: "Books",
    seriesTotal: ({ n }) => `${n} series`,
    sortLabel: "Sort order",
    sortReverse: "Reverse sort order",
    sortAsc: "Ascending",
    sortDesc: "Descending",
    filters: "Filters",
    filtersCount: ({ n }) => `Filters ${n}`,
    readState: "Read state",
    status: "Status",
    genres: "Genres",
    clear: "Clear",
    removeChip: "Remove {label}",
    noMatch: "No series match these filters",
    clearFilters: "Clear filters",
    noSeries: "No series yet",
    noSeriesBody: "Scan the library and series will show up here",
  },
});

type Key = keyof typeof m.ja;

const SORTS: { value: SeriesSort; label: Key }[] = [
  { value: "title", label: "sortTitle" },
  { value: "updated", label: "sortUpdated" },
  { value: "created", label: "sortAdded" },
  { value: "bookCount", label: "sortBookCount" },
];
type ReadState = "unread" | "inProgress" | "read";
const READ_STATES: { value: ReadState; label: Key }[] = [
  { value: "unread", label: "readUnread" },
  { value: "inProgress", label: "readInProgress" },
  { value: "read", label: "readRead" },
];
const STATUSES: SeriesStatus[] = ["ongoing", "ended", "hiatus", "abandoned"];

export default function Screen() {
  const t = useT(m);
  const { libraryId = "all" } = useParams();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const [panelOpen, setPanelOpen] = useState(false);
  const [busy, setBusy] = useState(false);

  const isAll = libraryId === "all";
  const scope = isAll ? undefined : libraryId;

  // URL is the source of truth for sort and filters (comma-separated lists).
  // Unknown values from the URL are dropped (sort falls back to "title").
  const sort = parseSort(params.get("sort"));
  const desc = params.get("desc") === "1";
  const read = list(params.get("read")).filter(isReadState);
  const status = list(params.get("status")).filter(isStatus);
  const genre = list(params.get("genre"));
  const filterCount = read.length + status.length + genre.length;

  const update = (patch: Record<string, string | null>) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const [k, v] of Object.entries(patch)) {
          if (v) next.set(k, v);
          else next.delete(k);
        }
        return next;
      },
      { replace: true },
    );

  const toggle = (key: string, current: string[], value: string) => {
    const next = current.includes(value) ? current.filter((x) => x !== value) : [...current, value];
    update({ [key]: next.join(",") || null });
  };
  const clearAll = () => update({ read: null, status: null, genre: null });

  const me = useApi(() => api.me(), []);
  const isAdmin = me.data?.role === "admin";
  const libs = useApi(() => (isAll ? api.libraries() : Promise.resolve([])), [isAll]);
  // One library by id: null when it does not exist or is hidden from the caller.
  const one = useApi(() => (isAll ? Promise.resolve(undefined) : api.library(libraryId)), [libraryId]);
  // Genre options of this scope, from the server (not from a full list).
  const genreList = useApi(() => api.genres(scope), [scope]);
  const genres = [...(genreList.data ?? [])].sort((a, b) => a.localeCompare(b, intlLocale()));
  // A page at a time; a new scope, sort or filter starts again from the top.
  const seriesList = usePagedApi(
    (offset, limit) =>
      api.seriesPage({
        libraryId: scope,
        sort,
        desc,
        readState: read.length ? read : undefined,
        status: status.length ? status : undefined,
        genres: genre.length ? genre : undefined,
        limit,
        offset,
      }),
    [scope, sort, desc, read.join(), status.join(), genre.join()],
  );
  const series = seriesList.items;

  // Scans, analysis and reading elsewhere change what this list shows.
  useLiveReload(
    () => {
      seriesList.reload();
      genreList.reload();
      if (isAll) libs.reload();
      else one.reload();
    },
    (e) => e.type !== "task" && (isAll || e.data.libraryId === libraryId),
    { quiet: 1500, maxWait: 8000 },
  );

  const library = isAll ? undefined : (one.data ?? undefined);
  const libraryList = libs.data ?? [];
  const totals = isAll
    ? libraryList.reduce(
        (t, l) => ({ series: t.series + l.seriesCount, books: t.books + l.bookCount, size: t.size + l.sizeBytes }),
        { series: 0, books: 0, size: 0 },
      )
    : library && { series: library.seriesCount, books: library.bookCount, size: library.sizeBytes };
  const title = isAll ? t("allLibraries") : (library?.name ?? t("library"));
  const meta = totals ? (
    <span className="font-mono">
      {t("seriesLabel")} <CountUp value={totals.series} /> · {t("booksLabel")} <CountUp value={totals.books} /> · {formatBytes(totals.size)}
    </span>
  ) : undefined;

  // Runs a library-wide action; the list reloads so read state shows at once.
  const runAction = async (call: () => Promise<unknown>, done: string, reloadList: boolean) => {
    setBusy(true);
    try {
      await call();
      toast.success(done);
      if (reloadList) {
        seriesList.reload();
        one.reload();
      }
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };
  const menuItems = isAll
    ? []
    : [
        ...(isAdmin
          ? [
              {
                label: t("scan"),
                icon: Scan,
                disabled: busy,
                run: () => runAction(() => api.scanLibrary(libraryId), t("scanStarted"), false),
              },
            ]
          : []),
        {
          label: t("markLibraryRead"),
          icon: CircleCheck,
          disabled: busy,
          run: () => runAction(() => api.markLibraryRead(libraryId, true), t("markedRead"), true),
        },
        {
          label: t("markLibraryUnread"),
          icon: Circle,
          disabled: busy,
          run: () => runAction(() => api.markLibraryRead(libraryId, false), t("markedUnread"), true),
        },
        ...(isAdmin ? [{ label: t("libraryConfig"), icon: Settings, disabled: false, run: () => navigate("/admin/libraries") }] : []),
      ];

  const readLabel = (v: string) => {
    const r = READ_STATES.find((x) => x.value === v);
    return r ? t(r.label) : v;
  };
  const chips = [
    ...read.map((v) => ({ key: `read-${v}`, label: readLabel(v), remove: () => toggle("read", read, v) })),
    ...status.map((v) => ({ key: `status-${v}`, label: statusLabel()[v], remove: () => toggle("status", status, v) })),
    ...genre.map((v) => ({ key: `genre-${v}`, label: v, remove: () => toggle("genre", genre, v) })),
  ];

  const header = (
    <PageHeader
      icon={library ? <KindIcon kind={library.kind} size={52} /> : undefined}
      title={title}
      meta={meta}
      actions={
        isAll ? undefined : (
          <DropdownMenu>
            <Tooltip content={t("moreActions")}>
              <span className="inline-flex">
                <DropdownTrigger render={<Button variant="ghost" size="icon" aria-label={t("moreActions")} />}>
                  <MoreHorizontal size={16} />
                </DropdownTrigger>
              </span>
            </Tooltip>
            <DropdownContent align="end">
              {menuItems.map((item, i) => (
                <MenuItem key={item.label} index={i} label={item.label} icon={item.icon} disabled={item.disabled} onSelect={item.run} />
              ))}
            </DropdownContent>
          </DropdownMenu>
        )
      }
    />
  );

  if (!isAll && one.data === null) {
    return (
      <Page>
        <EmptyState title={t("libraryNotFound")} body={t("libraryNotFoundBody")} />
      </Page>
    );
  }

  // A failed first page replaces the grid; a failed later page shows under it.
  const failed = one.error ?? (series ? null : seriesList.error);
  if (failed) {
    return (
      <Page>
        {header}
        <LoadError message={failed} onRetry={() => { one.reload(); seriesList.retry(); }} />
      </Page>
    );
  }

  if (!series) {
    return (
      <Page>
        {header}
        <GridSkeleton />
      </Page>
    );
  }

  return (
    <Page>
      {header}

      <div className="sticky top-0 z-10 mb-4 flex flex-wrap items-center justify-between gap-2 bg-surface-2 py-2">
        <p className={cn(typeClass("caption"), "font-mono text-muted-foreground tabular-nums")}>{t("seriesTotal", { n: seriesList.total })}</p>
        <div className="flex items-center gap-2">
          <Select value={sort} onValueChange={(v) => update({ sort: v === "title" ? null : v })}>
            <SelectTrigger aria-label={t("sortLabel")} />
            <SelectContent>
              {SORTS.map((s, i) => (
                <SelectItem key={s.value} index={i} value={s.value}>
                  {t(s.label)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Tooltip content={desc ? t("sortDesc") : t("sortAsc")}>
            <Button
              variant="ghost"
              size="icon"
              aria-label={t("sortReverse")}
              onClick={() => update({ desc: desc ? null : "1" })}
            >
              <ArrowUpDown size={16} />
            </Button>
          </Tooltip>
          <Button
            variant="secondary"
            aria-expanded={panelOpen}
            onClick={() => setPanelOpen((o) => !o)}
            leadingIcon={SlidersHorizontal as IconComponent}
          >
            {filterCount > 0 ? t("filtersCount", { n: filterCount }) : t("filters")}
          </Button>
        </div>
      </div>

      {panelOpen && (
        <div className="mb-4 grid gap-4 rounded-[12px] bg-card p-4 md:grid-cols-3">
          <FilterGroup title={t("readState")}>
            {READ_STATES.map((o) => (
              <CheckRow key={o.value} label={t(o.label)} checked={read.includes(o.value)} onChange={() => toggle("read", read, o.value)} />
            ))}
          </FilterGroup>
          <FilterGroup title={t("status")}>
            {STATUSES.map((v) => (
              <CheckRow key={v} label={statusLabel()[v]} checked={status.includes(v)} onChange={() => toggle("status", status, v)} />
            ))}
          </FilterGroup>
          <FilterGroup title={t("genres")}>
            {genres.map((g) => (
              <CheckRow key={g} label={g} checked={genre.includes(g)} onChange={() => toggle("genre", genre, g)} />
            ))}
          </FilterGroup>
          <div className="flex justify-end md:col-span-3">
            <Button variant="ghost" onClick={clearAll}>
              {t("clear")}
            </Button>
          </div>
        </div>
      )}

      {chips.length > 0 && (
        <div className="mb-4 flex flex-wrap gap-2">
          {chips.map((c) => (
            <button
              key={c.key}
              type="button"
              onClick={c.remove}
              aria-label={t("removeChip", { label: c.label })}
              className={cn(
                typeClass("body"),
                "inline-flex h-9 items-center gap-1 rounded-full bg-muted px-3 text-foreground outline-none focus-visible:ring-2 ring-[var(--focus-ring)]",
              )}
            >
              <span className="[text-box:trim-both_cap_alphabetic]">{c.label}</span>
              <X size={14} />
            </button>
          ))}
        </div>
      )}

      {series.length === 0 ? (
        filterCount > 0 ? (
          <EmptyState
            title={t("noMatch")}
            action={
              <Button variant="secondary" onClick={clearAll}>
                {t("clearFilters")}
              </Button>
            }
          />
        ) : (
          <EmptyState title={t("noSeries")} body={t("noSeriesBody")} />
        )
      ) : (
        <>
          <CoverGrid>
            {series.map((s, i) => (
              <SeriesCard key={s.id} series={s} priority={i < 8} />
            ))}
            {seriesList.hasMore && !seriesList.error && Array.from({ length: 6 }, (_, i) => <CoverSkeleton key={`more-${i}`} />)}
          </CoverGrid>
          {seriesList.error ? (
            <LoadError message={seriesList.error} onRetry={seriesList.retry} />
          ) : (
            seriesList.hasMore && <LoadMore onVisible={seriesList.loadMore} shown={series.length} />
          )}
        </>
      )}
    </Page>
  );
}

/** Asks for the next page when the end of the grid comes within a screen of
 *  view. It observes again whenever the list grows, so a page too short to
 *  fill the screen is followed by the next one at once. */
function LoadMore({ onVisible, shown }: { onVisible: () => void; shown: number }) {
  const ref = useRef<HTMLDivElement>(null);
  const cb = useRef(onVisible);
  useEffect(() => {
    cb.current = onVisible;
  });
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const io = new IntersectionObserver((entries) => entries.some((e) => e.isIntersecting) && cb.current(), {
      root: document.getElementById("cn-scroll"),
      rootMargin: "800px 0px",
    });
    io.observe(el);
    return () => io.disconnect();
  }, [shown]);
  return <div ref={ref} aria-hidden className="h-px" />;
}

function list(raw: string | null): string[] {
  return raw ? raw.split(",").filter(Boolean) : [];
}

function isSort(v: string | null): v is SeriesSort {
  return SORTS.some((s) => s.value === v);
}

function parseSort(raw: string | null): SeriesSort {
  return isSort(raw) ? raw : "title";
}

function isReadState(v: string): v is ReadState {
  return READ_STATES.some((r) => r.value === v);
}

function isStatus(v: string): v is SeriesStatus {
  return STATUSES.includes(v as SeriesStatus);
}

function FilterGroup({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="space-y-2">
      <p className={cn(typeClass("caption"), "text-muted-foreground")}>{title}</p>
      <div className="flex flex-col gap-2">{children}</div>
    </div>
  );
}

function CheckRow({ label, checked, onChange }: { label: string; checked: boolean; onChange: () => void }) {
  return (
    <label className={cn(typeClass("body"), "flex min-h-9 cursor-pointer items-center gap-2 text-foreground")}>
      <Checkbox checked={checked} onCheckedChange={onChange} />
      {label}
    </label>
  );
}
