import type { IconComponent } from "@/lib/icon-context";
import { useEffect, useRef, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router";
import { toast } from "@yusufceng1z/ding-react";
import { AnimatePresence } from "motion/react";
import { ArrowUpDown, Circle, CircleCheck, FolderPlus, ListChecks, MoreHorizontal, PenLine, Scan, Settings, SlidersHorizontal, X } from "@keyline-icons/react";
import { api, errorMessage, formatBytes, statusLabel, useApi, usePagedApi, type SeriesSort, type SeriesStatus } from "@/api";
import { useLiveReload } from "@/api/live";
import { LoadError } from "@/components/shell/load-error";
import { SeriesCard } from "@/components/media/cards";
import { CountUp } from "@/components/media/count-up";
import { CoverGrid, EmptyState, Page, PageHeader } from "@/components/media/layout";
import { KindIcon } from "@/components/media/kind-icon";
import { SelectionBar, SelectTile, type BarAction } from "@/components/media/selection";
import { inChunks, useBulkRun, useSelection } from "@/components/media/selection-state";
import { CoverSkeleton, GridSkeleton } from "@/components/media/skeletons";
import { Button } from "@/components/ui/button";
import { DropdownContent, DropdownMenu, DropdownTrigger } from "@/components/ui/dropdown";
import { MenuItem } from "@/components/ui/menu-item";
import { Select, SelectContent, SelectItem, SelectTrigger } from "@/components/ui/select";
import { Tooltip } from "@/components/ui/tooltip";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { defineMessages, intlLocale, useT } from "@/i18n";
import { CheckList, FilterGroup, CheckRow } from "./library-filters";
import { AddToCollectionDialog } from "./series/add-to-collection";
import { SeriesBulkDialog } from "./series-bulk-dialog";

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
    tags: "タグ",
    publishers: "出版社",
    languages: "言語",
    authors: "作者",
    ageRating: "年齢制限",
    ageUpTo: ({ n }) => `${n}+ 以下`,
    completion: "完結",
    completeYes: "完結",
    completeNo: "未完結",
    any: "すべて",
    searchOptions: "候補を検索",
    noOptionMatch: "一致する候補がありません",
    clear: "クリア",
    removeChip: "{label}を外す",
    noMatch: "条件に合うシリーズがありません",
    clearFilters: "絞り込みをクリア",
    noSeries: "シリーズがありません",
    noSeriesBody: "ライブラリをスキャンすると、ここに表示されます",
    select: "選択",
    markRead: "既読にする",
    markUnread: "未読にする",
    markedReadN: ({ n }) => `${n} 件を既読にしました`,
    markedUnreadN: ({ n }) => `${n} 件を未読にしました`,
    addToCollection: "コレクションに追加",
    bulkEdit: "一括編集",
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
    tags: "Tags",
    publishers: "Publishers",
    languages: "Languages",
    authors: "Authors",
    ageRating: "Age rating",
    ageUpTo: ({ n }) => `Up to ${n}+`,
    completion: "Completion",
    completeYes: "Complete",
    completeNo: "Not complete",
    any: "Any",
    searchOptions: "Search options",
    noOptionMatch: "No matches",
    clear: "Clear",
    removeChip: "Remove {label}",
    noMatch: "No series match these filters",
    clearFilters: "Clear filters",
    noSeries: "No series yet",
    noSeriesBody: "Scan the library and series will show up here",
    select: "Select",
    markRead: "Mark as read",
    markUnread: "Mark as unread",
    markedReadN: ({ n }) => `Marked ${n} as read`,
    markedUnreadN: ({ n }) => `Marked ${n} as unread`,
    addToCollection: "Add to collection",
    bulkEdit: "Edit series",
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
  const [collectionOpen, setCollectionOpen] = useState(false);
  const [bulkOpen, setBulkOpen] = useState(false);
  const [bulkRun, setBulkRun] = useState(0);

  const isAll = libraryId === "all";
  const scope = isAll ? undefined : libraryId;

  // URL is the source of truth for sort and filters. A list filter repeats
  // its parameter once per value (?tag=a&tag=b), so a value may contain a
  // comma. Unknown values from the URL are dropped (sort falls back to "title").
  const sort = parseSort(params.get("sort"));
  const desc = params.get("desc") === "1";
  const read = params.getAll("read").filter(isReadState);
  const status = params.getAll("status").filter(isStatus);
  const genre = params.getAll("genre").filter(Boolean);
  const tag = params.getAll("tag").filter(Boolean);
  const publisher = params.getAll("publisher").filter(Boolean);
  const language = params.getAll("language").filter(Boolean);
  const author = params.getAll("author").filter(Boolean);
  // Age rating "up to N" (a whole number) and completion (1 complete, 0 not).
  const age = parseAge(params.get("age"));
  const complete = params.get("complete") === "1" ? true : params.get("complete") === "0" ? false : undefined;
  const filterCount =
    read.length +
    status.length +
    genre.length +
    tag.length +
    publisher.length +
    language.length +
    author.length +
    (age === undefined ? 0 : 1) +
    (complete === undefined ? 0 : 1);

  // A string sets one value, an array repeats the parameter once per value;
  // null, "" or [] removes the parameter. Other parameters are kept.
  const update = (patch: Record<string, string | string[] | null>) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const [k, v] of Object.entries(patch)) {
          next.delete(k);
          for (const x of Array.isArray(v) ? v : v ? [v] : []) next.append(k, x);
        }
        return next;
      },
      { replace: true },
    );

  const toggle = (key: string, current: string[], value: string) => {
    const next = current.includes(value) ? current.filter((x) => x !== value) : [...current, value];
    update({ [key]: next });
  };
  const clearAll = () =>
    update({
      read: null,
      status: null,
      genre: null,
      tag: null,
      publisher: null,
      language: null,
      author: null,
      age: null,
      complete: null,
    });

  const me = useApi(() => api.me(), []);
  const isAdmin = me.data?.role === "admin";
  const libs = useApi(() => (isAll ? api.libraries() : Promise.resolve([])), [isAll]);
  // One library by id: null when it does not exist or is hidden from the caller.
  const one = useApi(() => (isAll ? Promise.resolve(undefined) : api.library(libraryId)), [libraryId]);
  // Every filter's options of this scope, from the server (not from a full list).
  const facets = useApi(() => api.seriesFacets(scope), [scope]);
  const genres = sortedOptions(facets.data?.genres);
  const tags = sortedOptions(facets.data?.tags);
  const publishers = sortedOptions(facets.data?.publishers);
  const languages = sortedOptions(facets.data?.languages);
  const authors = sortedOptions(facets.data?.authors);
  const ageRatings = [...(facets.data?.ageRatings ?? [])].sort((a, b) => a - b);
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
        tags: tag.length ? tag : undefined,
        publishers: publisher.length ? publisher : undefined,
        languages: language.length ? language : undefined,
        authors: author.length ? author : undefined,
        ageRatingMax: age,
        complete,
        limit,
        offset,
      }),
    [
      scope,
      sort,
      desc,
      JSON.stringify([read, status, genre, tag, publisher, language, author]),
      age,
      complete,
    ],
  );
  const series = seriesList.items;

  // Selection ends when the list changes (library, sort or filter): the key says which list it is.
  const selKey = JSON.stringify([scope ?? null, sort, desc, read, status, genre, tag, publisher, language, author, age ?? null, complete ?? null]);
  const sel = useSelection(series?.map((s) => s.id) ?? [], selKey);
  // After a bulk action the visible data reloads; the selection clears only when
  // the action worked, so a failed one can be retried with the same series.
  const afterBulk = (ok = true) => {
    if (ok) sel.clear();
    seriesList.reload();
    facets.reload();
    if (isAll) libs.reload();
    else one.reload();
  };
  const bulk = useBulkRun(afterBulk);

  // Scans, analysis and reading elsewhere change what this list shows.
  useLiveReload(
    () => {
      seriesList.reload();
      facets.reload();
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
    ...tag.map((v) => ({ key: `tag-${v}`, label: v, remove: () => toggle("tag", tag, v) })),
    ...publisher.map((v) => ({ key: `publisher-${v}`, label: v, remove: () => toggle("publisher", publisher, v) })),
    ...language.map((v) => ({ key: `language-${v}`, label: languageName(v), remove: () => toggle("language", language, v) })),
    ...author.map((v) => ({ key: `author-${v}`, label: v, remove: () => toggle("author", author, v) })),
    ...(age === undefined ? [] : [{ key: "age", label: t("ageUpTo", { n: age }), remove: () => update({ age: null }) }]),
    ...(complete === undefined
      ? []
      : [{ key: "complete", label: complete ? t("completeYes") : t("completeNo"), remove: () => update({ complete: null }) }]),
  ];

  const readSeries = (read: boolean) =>
    bulk.run(
      () => inChunks(sel.ids, (chunk) => api.setSeriesReadState(chunk, read).then(() => chunk.length)),
      (n) => t(read ? "markedReadN" : "markedUnreadN", { n }),
    );
  const selectionActions: BarAction[] = [
    { key: "read", label: t("markRead"), icon: CircleCheck as IconComponent, onSelect: () => readSeries(true) },
    { key: "unread", label: t("markUnread"), icon: Circle as IconComponent, onSelect: () => readSeries(false) },
    ...(isAdmin
      ? [
          { key: "collection", label: t("addToCollection"), icon: FolderPlus as IconComponent, onSelect: () => setCollectionOpen(true) },
          {
            key: "edit",
            label: t("bulkEdit"),
            icon: PenLine as IconComponent,
            onSelect: () => {
              setBulkRun((r) => r + 1);
              setBulkOpen(true);
            },
          },
        ]
      : []),
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
                <DropdownTrigger render={<Button variant="ghost" size="icon" aria-label={t("moreActions")}><MoreHorizontal size={16} /></Button>} />
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

  // One list filter: a checkbox list of the options, with a search box when long.
  const listGroup = (
    title: string,
    options: string[],
    selected: string[],
    key: string,
    label?: (value: string) => string,
  ) =>
    options.length > 0 && (
      <FilterGroup title={title}>
        <CheckList
          options={options}
          selected={selected}
          label={label}
          onToggle={(v) => toggle(key, selected, v)}
          searchLabel={t("searchOptions")}
          noMatch={t("noOptionMatch")}
        />
      </FilterGroup>
    );

  return (
    <Page>
      {header}

      <div className="sticky top-0 z-10 mb-4 flex flex-wrap items-center justify-between gap-2 bg-surface-2 py-2">
        <p className={cn(typeClass("caption"), "font-mono text-muted-foreground tabular-nums")}>{t("seriesTotal", { n: seriesList.total })}</p>
        <div className="flex items-center gap-2">
          {!sel.active && series.length > 0 && (
            <Button variant="secondary" onClick={sel.enter} leadingIcon={ListChecks as IconComponent}>
              {t("select")}
            </Button>
          )}
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
          {listGroup(t("genres"), genres, genre, "genre")}
          {listGroup(t("tags"), tags, tag, "tag")}
          {listGroup(t("publishers"), publishers, publisher, "publisher")}
          {listGroup(t("languages"), languages, language, "language", languageName)}
          {listGroup(t("authors"), authors, author, "author")}
          {ageRatings.length > 0 && (
            <FilterGroup title={t("ageRating")}>
              <Select value={age === undefined ? "any" : String(age)} onValueChange={(v) => update({ age: v === "any" ? null : v })}>
                <SelectTrigger aria-label={t("ageRating")} />
                <SelectContent>
                  <SelectItem index={0} value="any">
                    {t("any")}
                  </SelectItem>
                  {ageRatings.map((n, i) => (
                    <SelectItem key={n} index={i + 1} value={String(n)}>
                      {t("ageUpTo", { n })}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FilterGroup>
          )}
          <FilterGroup title={t("completion")}>
            <Select
              value={complete === undefined ? "any" : complete ? "yes" : "no"}
              onValueChange={(v) => update({ complete: v === "yes" ? "1" : v === "no" ? "0" : null })}
            >
              <SelectTrigger aria-label={t("completion")} />
              <SelectContent>
                <SelectItem index={0} value="any">
                  {t("any")}
                </SelectItem>
                <SelectItem index={1} value="yes">
                  {t("completeYes")}
                </SelectItem>
                <SelectItem index={2} value="no">
                  {t("completeNo")}
                </SelectItem>
              </SelectContent>
            </Select>
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
              <SelectTile
                key={s.id}
                selecting={sel.active}
                selected={sel.has(s.id)}
                label={s.title}
                onToggle={(shift) => sel.toggle(s.id, shift)}
              >
                <SeriesCard series={s} priority={i < 8} />
              </SelectTile>
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

      <AnimatePresence>
        {sel.active && (
          <SelectionBar
            key="selection"
            count={sel.count}
            busy={bulk.busy}
            onSelectAll={sel.selectAll}
            onCancel={sel.exit}
            actions={selectionActions}
          />
        )}
      </AnimatePresence>
      {isAdmin && (
        <AddToCollectionDialog seriesIds={sel.ids} open={collectionOpen} onOpenChange={setCollectionOpen} onAdded={() => afterBulk()} />
      )}
      {isAdmin && (
        <SeriesBulkDialog key={bulkRun} seriesIds={sel.ids} open={bulkOpen} onOpenChange={setBulkOpen} onDone={afterBulk} />
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

/** Sorted copy of a facet list, in the UI's collation. */
function sortedOptions(list: readonly string[] | undefined): string[] {
  return [...(list ?? [])].sort((a, b) => a.localeCompare(b, intlLocale()));
}

/** A language tag as its name ("ja" to "日本語"); the tag itself when unknown. */
function languageName(code: string): string {
  try {
    return new Intl.DisplayNames([intlLocale()], { type: "language" }).of(code) ?? code;
  } catch {
    return code;
  }
}

function parseAge(raw: string | null): number | undefined {
  return raw && /^\d{1,3}$/.test(raw) ? Number(raw) : undefined;
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
