import type { IconComponent } from "@/lib/icon-context";
import { Fragment, useState, type ReactNode } from "react";
import { Link, useParams } from "react-router";
import { ArrowDown, ArrowUp, ChevronLeft, Play } from "@keyline-icons/react";
import { toast } from "@yusufceng1z/ding-react";
import { api, formatDate, statusLabel, useApi } from "@/api";
import { BookCard } from "@/components/media/cards";
import { Cover } from "@/components/media/cover";
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
import { EditDialog, directionLabel, fieldLabel, roleLabel } from "./edit-dialog";

export default function Screen() {
  const { seriesId = "" } = useParams();
  // Keyed by id so tab, sort and dialog state reset when the route changes.
  return <SeriesPage key={seriesId} id={seriesId} />;
}

function SeriesPage({ id }: { id: string }) {
  const { data } = useApi(async () => {
    const series = await api.seriesById(id);
    if (!series) return null;
    const [library, books] = await Promise.all([api.library(series.libraryId), api.booksOfSeries(id)]);
    return { series, library, books };
  }, [id]);

  const [tab, setTab] = useState(0);
  const [order, setOrder] = useState<"asc" | "desc">("asc");
  const [more, setMore] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [editRun, setEditRun] = useState(0);

  if (data === undefined) {
    return (
      <Page>
        <div className="flex flex-col gap-6 md:flex-row md:gap-8" aria-hidden>
          <Block className="mx-auto aspect-[2/3] w-[160px] shrink-0 rounded-[6px] md:mx-0 md:w-[220px]" />
          <div className="flex min-w-0 flex-1 flex-col gap-3 pt-1">
            <Block className="h-4 w-24" />
            <Block className="h-8 w-2/3" />
            <Block className="h-4 w-1/2" />
            <Block className="h-1 w-full rounded-full" />
            <Block className="mt-2 h-9 w-40" />
          </div>
        </div>
      </Page>
    );
  }

  if (!data) {
    return (
      <Page>
        <EmptyState title="シリーズが見つかりません" body="ライブラリからもう一度探してください" />
      </Page>
    );
  }

  const { series, library, books } = data;
  const total = series.totalBookCount ?? series.bookCount;
  const pct = total ? Math.min(100, Math.round((series.booksRead / total) * 100)) : 0;

  // Primary action: resume the book in progress, else the first unread one;
  // when everything is read, start over from book 1.
  const allRead = books.length > 0 && books.every((b) => b.progress?.completed);
  const started = books.some((b) => b.progress);
  const next = books.find((b) => b.progress && !b.progress.completed) ?? books.find((b) => !b.progress?.completed);
  const primary = allRead
    ? { to: `/read/${books[0].id}`, label: "もう一度読む" }
    : next
      ? { to: `/read/${next.id}`, label: started ? "続きから読む" : "1巻から読む" }
      : null;

  const shown = order === "asc" ? books : [...books].reverse();
  const authors = series.authors.map((a) => `${a.name}（${roleLabel[a.role]}）`).join("、");
  const meta: ReactNode[] = [
    authors || null,
    series.publisher || null,
    <Badge key="status" variant="dot">
      {statusLabel[series.status]}
    </Badge>,
    series.language || null,
    series.ageRating ? `${series.ageRating}+` : null,
  ];

  return (
    <Page>
      <Link
        to={`/libraries/${series.libraryId}`}
        className={cn(typeClass("caption"), "inline-flex w-fit items-center gap-1 text-muted-foreground hover:text-foreground pointer-coarse:-my-3 pointer-coarse:py-3", "mb-4 pointer-coarse:mb-1")}
      >
        <ChevronLeft size={14} />
        {library?.name ?? "ライブラリ"}
      </Link>
      <div className="flex flex-col gap-6 md:flex-row md:gap-8">
        <div className="mx-auto w-[160px] shrink-0 md:mx-0 md:w-[220px]">
          <Cover seed={series.coverSeed} title={series.title} priority />
        </div>

        <div className="flex min-w-0 flex-1 flex-col gap-3">

          <h1 className={cn(typeClass("display"), "break-words")} style={{ fontVariationSettings: fontWeights.bold }}>
            {series.title}
          </h1>
          {series.alternateTitles.length > 0 && (
            <p className={cn(typeClass("caption"), "text-muted-foreground")}>{series.alternateTitles.join("・")}</p>
          )}

          <div className={cn(typeClass("body"), "flex flex-wrap items-center gap-x-2 gap-y-1 text-muted-foreground")}>
            {meta
              .filter(Boolean)
              .map((m, i) => (
                <Fragment key={i}>
                  {i > 0 && <span aria-hidden>·</span>}
                  {m}
                </Fragment>
              ))}
          </div>

          <div className="mt-1 space-y-1.5">
            <p className={cn(typeClass("caption"), "text-muted-foreground tabular-nums")}>
              {series.booksRead} / {total} 巻 既読
            </p>
            <div
              role="progressbar"
              aria-label="既読の進捗"
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={pct}
              className="h-1 w-full overflow-hidden rounded-full bg-muted"
            >
              <div className="h-full rounded-full bg-brand" style={{ width: `${pct}%` }} />
            </div>
          </div>

          <div className="mt-2 flex flex-wrap items-center gap-2">
            {primary && (
              <BrandAction to={primary.to} icon={Play}>
                {primary.label}
              </BrandAction>
            )}
            <Button variant="secondary" onClick={() => toast.success("既読にしました")}>
              既読にする
            </Button>
            <DropdownMenu>
              <DropdownTrigger render={<Button variant="ghost">その他</Button>} />
              <DropdownContent align="start" sideOffset={4}>
                <MenuItem index={0} label="メタデータを編集" onSelect={() => { setEditRun((r) => r + 1); setEditOpen(true); }} />
                <MenuItem index={1} label="ZIP でダウンロード" />
                <MenuItem index={2} label="コレクションに追加" />
                <MenuItem index={3} label="サムネイルを変更" />
              </DropdownContent>
            </DropdownMenu>
          </div>

          {series.summary && (
            <div className="space-y-1">
              <p className={cn(typeClass("body"), "text-muted-foreground", !more && "line-clamp-3")}>{series.summary}</p>
              <Button variant="ghost" size="compact" className="pointer-coarse:h-11" onClick={() => setMore((m) => !m)}>
                {more ? "閉じる" : "続きを読む"}
              </Button>
            </div>
          )}

          {(series.genres.length > 0 || series.tags.length > 0) && (
            <div className="flex flex-wrap gap-1.5">
              {series.genres.map((g) => (
                <Link
                  key={g}
                  to={`/libraries/${series.libraryId}?genre=${encodeURIComponent(g)}`}
                  className="rounded-full outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)] pointer-coarse:py-2"
                >
                  <Badge variant="dot">{g}</Badge>
                </Link>
              ))}
              {series.tags.map((t) => (
                <Badge key={t} variant="dot">
                  {t}
                </Badge>
              ))}
            </div>
          )}
        </div>
      </div>

      <div className="mt-10 mb-4 flex flex-wrap items-center justify-between gap-2">
        <Tabs selectedIndex={tab} onSelect={setTab} idPrefix="series">
          <TabsItem index={0} label={`巻（${books.length}）`} />
          <TabsItem index={1} label="情報" />
        </Tabs>
        {tab === 0 && books.length > 1 && (
          <Button
            variant="ghost"
            size="compact"
            className="pointer-coarse:h-11"
            leadingIcon={(order === "asc" ? ArrowUp : ArrowDown) as IconComponent}
            onClick={() => setOrder((o) => (o === "asc" ? "desc" : "asc"))}
          >
            {order === "asc" ? "巻数の昇順" : "巻数の降順"}
          </Button>
        )}
      </div>

      <TabsPanel index={0} selectedIndex={tab} idPrefix="series">
        {books.length === 0 ? (
          <EmptyState title="巻がありません" body="スキャンで巻が見つかると、ここに表示されます" />
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
          <Fact label="読み方向">{directionLabel[series.readingDirection]}</Fact>
          <Fact label="総巻数">{series.totalBookCount ?? "未設定"}</Fact>
          <Fact label="追加日">{formatDate(series.createdAt)}</Fact>
          <Fact label="更新日">{formatDate(series.updatedAt)}</Fact>
          <Fact label="出版社">{series.publisher || "未設定"}</Fact>
          <Fact label="ロックされたフィールド">
            {series.lockedFields.length ? series.lockedFields.map((f) => fieldLabel[f] ?? f).join("、") : "なし"}
          </Fact>
        </dl>
      </TabsPanel>

      <EditDialog
        key={editRun}
        series={series}
        open={editOpen}
        onOpenChange={setEditOpen}
      />
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
