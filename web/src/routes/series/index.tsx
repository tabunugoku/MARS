import type { IconComponent } from "@/lib/icon-context";
import { Fragment, useState, type ReactNode } from "react";
import { Link, useParams } from "react-router";
import { ArrowDown, ArrowUp, ChevronLeft, Play } from "@keyline-icons/react";
import { toast } from "@yusufceng1z/ding-react";
import { api, errorMessage, formatDate, seriesFileUrl, startDownload, statusLabel, useApi, type Series } from "@/api";
import { useLiveReload } from "@/api/live";
import { LoadError } from "@/components/shell/load-error";
import { BookCard } from "@/components/media/cards";
import { Cover } from "@/components/media/cover";
import { CountUp } from "@/components/media/count-up";
import { DotTrack } from "@/components/media/dot-track";
import { BrandAction } from "@/components/media/brand-action";
import { CoverGrid, EmptyState, Page } from "@/components/media/layout";
import { Block } from "@/components/media/skeletons";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DropdownContent, DropdownMenu, DropdownTrigger } from "@/components/ui/dropdown";
import { MenuItem } from "@/components/ui/menu-item";
import { Tabs, TabsItem, TabsPanel } from "@/components/ui/tabs";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";
import { defineMessages, useT } from "@/i18n";
import { EditDialog, directionLabel, fieldLabel, roleLabel } from "./edit-dialog";
import { AddToCollectionDialog } from "./add-to-collection";

const m = defineMessages({
  ja: {
    notFound: "シリーズが見つかりません",
    notFoundBody: "ライブラリからもう一度探してください",
    markedRead: "既読にしました",
    markedUnread: "未読にしました",
    thumbnailsStarted: "表紙の作り直しを開始しました",
    readAgain: "もう一度読む",
    continueReading: "続きから読む",
    readFromFirst: "1巻から読む",
    markUnread: "未読にする",
    markRead: "既読にする",
    library: "ライブラリ",
    booksRead: "/ {total} 巻 既読",
    listSep: "、",
    author: "{name}（{role}）",
    titleSep: "・",
    tabBooks: "巻（{n}）",
    tabInfo: "情報",
    ascending: "巻数の昇順",
    descending: "巻数の降順",
    editMetadata: "メタデータを編集",
    addToCollection: "コレクションに追加",
    downloadAll: "まとめてダウンロード（ZIP）",
    regenerate: "表紙を作り直す",
    showLess: "閉じる",
    showMore: "続きを読む",
    noBooks: "巻がありません",
    noBooksBody: "スキャンで巻が見つかると、ここに表示されます",
    readingDirection: "読み方向",
    totalBooks: "総巻数",
    added: "追加日",
    updated: "更新日",
    publisher: "出版社",
    lockedFields: "ロックされたフィールド",
    none: "なし",
    notSet: "未設定",
    progress: "既読の巻数",
    more: "その他",
  },
  en: {
    notFound: "Series not found",
    notFoundBody: "Find it again from the library",
    markedRead: "Marked as read",
    markedUnread: "Marked as unread",
    thumbnailsStarted: "Thumbnail regeneration started",
    readAgain: "Read again",
    continueReading: "Continue reading",
    readFromFirst: "Read from book 1",
    markUnread: "Mark as unread",
    markRead: "Mark as read",
    library: "Library",
    booksRead: "/ {total} books read",
    listSep: ", ",
    author: "{name} ({role})",
    titleSep: ", ",
    tabBooks: "Books ({n})",
    tabInfo: "Info",
    ascending: "Ascending",
    descending: "Descending",
    editMetadata: "Edit metadata",
    addToCollection: "Add to collection",
    downloadAll: "Download all (ZIP)",
    regenerate: "Regenerate thumbnail",
    showLess: "Show less",
    showMore: "Show more",
    noBooks: "No books",
    noBooksBody: "Books show up here once a scan finds them",
    readingDirection: "Reading direction",
    totalBooks: "Total books",
    added: "Added",
    updated: "Updated",
    publisher: "Publisher",
    lockedFields: "Locked fields",
    none: "None",
    notSet: "Not set",
    progress: "Books read",
    more: "More",
  },
});

export default function Screen() {
  const { seriesId = "" } = useParams();
  // Keyed by id so tab, sort and dialog state reset when the route changes.
  return <SeriesPage key={seriesId} id={seriesId} />;
}

function SeriesPage({ id }: { id: string }) {
  const t = useT(m);
  const { data, error, reload } = useApi(async () => {
    const series = await api.seriesById(id);
    if (!series) return null;
    const [library, books] = await Promise.all([api.library(series.libraryId), api.booksOfSeries(id)]);
    return { series, library, books };
  }, [id]);
  // New or analysed books, and reading on another tab or device.
  const libraryId = data?.series.libraryId;
  useLiveReload(reload, (e) =>
    e.type === "library"
      ? e.data.libraryId === libraryId
      : e.type === "book" || e.type === "progress"
        ? e.data.seriesId === id || (e.type === "progress" && !e.data.seriesId && e.data.libraryId === libraryId)
        : false,
  );

  const [tab, setTab] = useState(0);
  const [order, setOrder] = useState<"asc" | "desc">("asc");
  const [more, setMore] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [editRun, setEditRun] = useState(0);
  const [addOpen, setAddOpen] = useState(false);
  // The series as last saved from the edit dialog; cleared when the page reloads.
  const [saved, setSaved] = useState<Series | null>(null);
  const [busy, setBusy] = useState(false);
  const { data: me } = useApi(() => api.me(), []);
  const isAdmin = me?.role === "admin";
  const canDownload = isAdmin || !!me?.canDownload;

  if (error) {
    return (
      <Page>
        <LoadError message={error} onRetry={reload} />
      </Page>
    );
  }

  if (data === undefined) {
    return (
      <Page>
        <div className="flex flex-col gap-6 md:flex-row md:gap-8" aria-hidden>
          <Block className="mx-auto aspect-[2/3] w-[160px] shrink-0 rounded-[6px] md:mx-0 md:w-[220px]" />
          <div className="flex min-w-0 flex-1 flex-col gap-3 pt-1">
            <Block className="h-4 w-24" />
            <Block className="h-8 w-2/3" />
            <Block className="h-4 w-1/2" />
            <Block className="h-2 w-2/3 rounded-full" />
            <Block className="mt-2 h-9 w-40" />
          </div>
        </div>
      </Page>
    );
  }

  if (!data) {
    return (
      <Page>
        <EmptyState title={t("notFound")} body={t("notFoundBody")} />
      </Page>
    );
  }

  const { library, books } = data;
  const series = saved ?? data.series;
  const total = series.totalBookCount ?? series.bookCount;
  const roles = roleLabel();
  const directionNames = directionLabel();
  const fields = fieldLabel();

  // Primary action: resume the book in progress, else the first unread one;
  // when everything is read, start over from book 1.
  const allRead = books.length > 0 && books.every((b) => b.progress?.completed);

  const markRead = async (completed: boolean) => {
    setBusy(true);
    try {
      await api.markSeriesRead(id, completed);
      toast.success(completed ? t("markedRead") : t("markedUnread"));
      setSaved(null);
      reload();
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };
  const regenerate = async () => {
    try {
      await api.regenerateSeriesThumbnails(id);
      toast.success(t("thumbnailsStarted"));
    } catch (err) {
      toast.error(errorMessage(err));
    }
  };
  const started = books.some((b) => b.progress);
  const next = books.find((b) => b.progress && !b.progress.completed) ?? books.find((b) => !b.progress?.completed);
  const primary = allRead
    ? { to: `/read/${books[0].id}`, label: t("readAgain") }
    : next
      ? { to: `/read/${next.id}`, label: started ? t("continueReading") : t("readFromFirst") }
      : null;

  const shown = order === "asc" ? books : [...books].reverse();
  const authors = series.authors.map((a) => t("author", { name: a.name, role: roles[a.role] })).join(t("listSep"));
  const meta: ReactNode[] = [
    authors || null,
    series.publisher || null,
    <Badge key="status" variant="dot">
      {statusLabel()[series.status]}
    </Badge>,
    series.language ? <span className="font-mono">{series.language}</span> : null,
    series.ageRating ? <span className="font-mono">{`${series.ageRating}+`}</span> : null,
  ];

  return (
    <Page>
      <Link
        to={`/libraries/${series.libraryId}`}
        className={cn(typeClass("caption"), "inline-flex w-fit items-center gap-1 text-muted-foreground hover:text-foreground pointer-coarse:-my-3.5 pointer-coarse:py-3.5", "mb-4 pointer-coarse:mb-1")}
      >
        <ChevronLeft size={14} />
        {library?.name ?? t("library")}
      </Link>
      <div className="flex flex-col gap-6 md:flex-row md:gap-8">
        <div className="mx-auto w-[160px] shrink-0 md:mx-0 md:w-[220px]">
          <Cover src={series.thumbnailUrl} title={series.title} priority />
        </div>

        <div className="flex min-w-0 flex-1 flex-col gap-3">

          <h1 className={cn(typeClass("display"), "break-words")} style={{ fontVariationSettings: fontWeights.bold }}>
            {series.title}
          </h1>
          {series.alternateTitles.length > 0 && (
            <p className={cn(typeClass("caption"), "text-muted-foreground")}>{series.alternateTitles.join(t("titleSep"))}</p>
          )}

          <div className={cn(typeClass("body"), "flex flex-wrap items-center gap-x-2 gap-y-1 text-muted-foreground")}>
            {meta
              .filter(Boolean)
              .map((item, i) => (
                <Fragment key={i}>
                  {i > 0 && <span aria-hidden>·</span>}
                  {item}
                </Fragment>
              ))}
          </div>

          <div className="mt-1 space-y-1.5">
            <p className={cn(typeClass("caption"), "font-mono text-muted-foreground tabular-nums")}>
              <CountUp value={series.booksRead} /> {t("booksRead", { total })}
            </p>
            <DotTrack total={series.bookCount} value={series.booksRead} label={t("progress")} />
          </div>

          <div className="mt-2 flex flex-wrap items-center gap-2">
            {primary && (
              <BrandAction to={primary.to} icon={Play}>
                {primary.label}
              </BrandAction>
            )}
            <Button variant="secondary" disabled={busy || books.length === 0} onClick={() => markRead(!allRead)}>
              {allRead ? t("markUnread") : t("markRead")}
            </Button>
            <DropdownMenu>
              <DropdownTrigger render={<Button variant="secondary">{t("more")}</Button>} />
              <DropdownContent align="start" sideOffset={4}>
                <MenuItem index={0} label={t("editMetadata")} onSelect={() => { setEditRun((r) => r + 1); setEditOpen(true); }} />
                {isAdmin && <MenuItem index={1} label={t("addToCollection")} onSelect={() => setAddOpen(true)} />}
                {canDownload && books.length > 0 && (
                  <MenuItem index={isAdmin ? 2 : 1} label={t("downloadAll")} onSelect={() => startDownload(seriesFileUrl(id))} />
                )}
                {isAdmin && <MenuItem index={(isAdmin ? 2 : 1) + (canDownload && books.length > 0 ? 1 : 0)} label={t("regenerate")} onSelect={regenerate} />}
              </DropdownContent>
            </DropdownMenu>
          </div>

          {series.summary && (
            <div className="space-y-1">
              <p className={cn(typeClass("body"), "text-muted-foreground", !more && "line-clamp-3")}>{series.summary}</p>
              <Button variant="ghost" size="compact" className="pointer-coarse:h-11" onClick={() => setMore((m) => !m)}>
                {more ? t("showLess") : t("showMore")}
              </Button>
            </div>
          )}

          {(series.genres.length > 0 || series.tags.length > 0) && (
            <div className="flex flex-wrap items-center gap-1.5 pointer-coarse:gap-y-5">
              {series.genres.map((g) => (
                <Link
                  key={g}
                  to={`/libraries/${series.libraryId}?genre=${encodeURIComponent(g)}`}
                  className="touch-hit relative rounded-full outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)]"
                >
                  <Badge variant="dot">{g}</Badge>
                </Link>
              ))}
              {series.tags.map((tag) => (
                <Badge key={tag} variant="dot">
                  {tag}
                </Badge>
              ))}
            </div>
          )}
        </div>
      </div>

      <div className="mt-10 mb-4 flex flex-wrap items-center justify-between gap-2">
        <Tabs selectedIndex={tab} onSelect={setTab} idPrefix="series">
          <TabsItem index={0} label={t("tabBooks", { n: books.length })} className="pointer-coarse:h-11" />
          <TabsItem index={1} label={t("tabInfo")} className="pointer-coarse:h-11" />
        </Tabs>
        {tab === 0 && books.length > 1 && (
          <Button
            variant="ghost"
            size="compact"
            className="pointer-coarse:h-11"
            leadingIcon={(order === "asc" ? ArrowUp : ArrowDown) as IconComponent}
            onClick={() => setOrder((o) => (o === "asc" ? "desc" : "asc"))}
          >
            {order === "asc" ? t("ascending") : t("descending")}
          </Button>
        )}
      </div>

      <TabsPanel index={0} selectedIndex={tab} idPrefix="series">
        {books.length === 0 ? (
          <EmptyState title={t("noBooks")} body={t("noBooksBody")} />
        ) : (
          <CoverGrid>
            {shown.map((b) => (
              <BookCard key={b.id} book={b} />
            ))}
          </CoverGrid>
        )}
      </TabsPanel>

      <TabsPanel index={1} selectedIndex={tab} idPrefix="series">
        <dl className="max-w-2xl divide-y divide-border">
          <Fact label={t("readingDirection")}>{directionNames[series.readingDirection]}</Fact>
          <Fact label={t("totalBooks")}>{series.totalBookCount ?? t("notSet")}</Fact>
          <Fact label={t("added")}>{formatDate(series.createdAt)}</Fact>
          <Fact label={t("updated")}>{formatDate(series.updatedAt)}</Fact>
          <Fact label={t("publisher")}>{series.publisher || t("notSet")}</Fact>
          <Fact label={t("lockedFields")}>
            {series.lockedFields.length ? series.lockedFields.map((f) => fields[f] ?? f).join(t("listSep")) : t("none")}
          </Fact>
        </dl>
      </TabsPanel>

      <EditDialog
        key={editRun}
        series={series}
        open={editOpen}
        onOpenChange={setEditOpen}
        onSaved={(s) => setSaved(s)}
      />
      {isAdmin && <AddToCollectionDialog seriesId={id} open={addOpen} onOpenChange={setAddOpen} />}
    </Page>
  );
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[7rem_1fr] gap-4 py-2.5">
      <dt className={cn(typeClass("caption"), "text-muted-foreground")}>{label}</dt>
      <dd className={typeClass("body")}>{children}</dd>
    </div>
  );
}
