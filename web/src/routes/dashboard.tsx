import { Link } from "react-router";
import { Play } from "@keyline-icons/react";
import { api, useApi, type Book, type Series } from "@/api";
import { useLiveReload } from "@/api/live";
import { BookCard, SeriesCard } from "@/components/media/cards";
import { Cover } from "@/components/media/cover";
import { BrandAction } from "@/components/media/brand-action";
import { CountUp } from "@/components/media/count-up";
import { DotTrack } from "@/components/media/dot-track";
import { EmptyState, Page, Shelf, ShelfItem } from "@/components/media/layout";
import { Block, CoverSkeleton } from "@/components/media/skeletons";
import { Button } from "@/components/ui/button";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";
import { defineMessages, useT } from "@/i18n";

const m = defineMessages({
  ja: {
    home: "ホーム",
    inProgress: "読みかけ",
    upNext: "次に読む",
    recentBooks: "最近追加された巻",
    newSeries: "新しいシリーズ",
    recentUpdated: "最近更新されたシリーズ",
    recentRead: "最近読んだ巻",
    noBooks: "まだ本がありません",
    noBooksBody: "管理 → ライブラリ設定でフォルダを追加すると、ここに表示されます",
    addLibrary: "ライブラリを追加",
    lastRead: "前回の続き",
    readingProgress: "読書の進捗",
    pagesLeft: ({ total, left }) => `${total} · 残り ${left} ページ`,
    resume: "続きから読む",
    bookDetails: "巻の詳細",
  },
  en: {
    home: "Home",
    inProgress: "In progress",
    upNext: "Up next",
    recentBooks: "Recently added books",
    newSeries: "New series",
    recentUpdated: "Recently updated series",
    recentRead: "Recently read books",
    noBooks: "No books yet",
    noBooksBody: "Add a folder under Admin → Library settings to see it here",
    addLibrary: "Add library",
    lastRead: "Last read",
    readingProgress: "Reading progress",
    pagesLeft: ({ total, left }) => `${total} · ${left} pages left`,
    resume: "Resume",
    bookDetails: "Book details",
  },
});

type Key = keyof typeof m.ja;

type Row = { title: Key; books?: Book[]; series?: Series[] };

// Chart-coordinate style date for the context line: 2026.11.09
const ymd = (iso: string) => {
  const d = new Date(iso);
  return `${d.getFullYear()}.${String(d.getMonth() + 1).padStart(2, "0")}.${String(d.getDate()).padStart(2, "0")}`;
};

export default function Screen() {
  const t = useT(m);
  const { data, reload } = useApi(
    async () => {
      const d = await api.dashboard();
      return {
        keep: d.keepReading,
        deck: d.onDeck,
        addedBooks: d.recentlyAddedBooks,
        addedSeries: d.recentlyAddedSeries,
        updatedSeries: d.recentlyUpdatedSeries,
        read: d.recentlyRead,
      };
    },
    [],
  );
  // New books and reading elsewhere; quieter than the library screens.
  useLiveReload(reload, (e) => e.type === "library" || e.type === "progress", { quiet: 2000, maxWait: 15000 });

  if (!data) {
    return (
      <Page>
        <h1 className="sr-only">{t("home")}</h1>
        <section className="mb-10 flex gap-4 rounded-[12px] bg-card p-4 md:p-5 tone" aria-hidden>
          <Block className="aspect-[2/3] w-[96px] shrink-0 rounded-[6px] sm:w-[120px]" />
          <div className="flex min-w-0 flex-1 flex-col gap-2 pt-1">
            <Block className="h-5 w-2/3" />
            <Block className="h-4 w-1/2" />
            <div className="mt-auto flex flex-wrap gap-1.5 pt-3" aria-hidden>
              {Array.from({ length: 20 }, (_, i) => (
                <Block key={i} className="size-2 rounded-full" />
              ))}
            </div>
          </div>
        </section>
        {[0, 1].map((r) => (
          <section key={r} className="mb-10" aria-hidden>
            <Block className="mb-3 h-5 w-32" />
            <div className="flex gap-4 overflow-hidden">
              {Array.from({ length: 6 }, (_, i) => (
                <ShelfItem key={i}>
                  <CoverSkeleton />
                </ShelfItem>
              ))}
            </div>
          </section>
        ))}
      </Page>
    );
  }

  const hero = data.keep[0];
  const sections: Row[] = [
    { title: "inProgress", books: data.keep.slice(1) },
    { title: "upNext", books: data.deck },
    { title: "recentBooks", books: data.addedBooks },
    { title: "newSeries", series: data.addedSeries },
    { title: "recentUpdated", series: data.updatedSeries },
    { title: "recentRead", books: data.read },
  ];
  const rows = sections.filter((r) => (r.books ?? r.series ?? []).length > 0);

  if (!hero && rows.length === 0) {
    return (
      <Page>
        <h1 className="sr-only">{t("home")}</h1>
        <EmptyState
          title={t("noBooks")}
          body={t("noBooksBody")}
          action={<BrandAction to="/admin/libraries">{t("addLibrary")}</BrandAction>}
        />
      </Page>
    );
  }

  const heroSeriesTitle = hero ? hero.seriesTitle || hero.title : "";
  const heroPage = hero?.progress?.page ?? 0;
  const heroPct = hero ? Math.min(100, Math.round((heroPage / hero.pageCount) * 100)) : 0;

  return (
    <Page>
      <h1 className="sr-only">{t("home")}</h1>

      {hero && (
        // On phones the text column is too narrow for both buttons, so their
        // row spans the card under the cover; wider, it stays in the column.
        <section className="mb-10 grid grid-cols-[96px_minmax(0,1fr)] gap-x-4 gap-y-3 rounded-[12px] bg-card p-4 sm:grid-cols-[120px_minmax(0,1fr)] sm:grid-rows-[1fr_auto] md:p-5 tone">
          <div className="sm:row-span-2">
            <Cover src={hero.thumbnailUrl} title={heroSeriesTitle} label={String(hero.number)} />
          </div>
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            {hero.progress && (
              <p className={cn(typeClass("caption"), "text-muted-foreground")}>
                {t("lastRead")} · <span className="font-mono tabular-nums">{ymd(hero.progress.readAt)}</span>
              </p>
            )}
            <h2 className={cn(typeClass("title"), "line-clamp-2")} style={{ fontVariationSettings: fontWeights.semibold }}>
              {heroSeriesTitle}
            </h2>
            <p className={cn(typeClass("body"), "line-clamp-2 text-muted-foreground")}>{hero.title}</p>
            <div className="mt-auto space-y-1.5 pt-3">
              <DotTrack total={28} value={Math.round((heroPct / 100) * 28)} label={t("readingProgress")} />
              <p className={cn(typeClass("caption"), "font-mono text-muted-foreground")}>
                P.<CountUp value={heroPage} /> / {t("pagesLeft", { total: hero.pageCount, left: hero.pageCount - heroPage })}
              </p>
            </div>
          </div>
          <div className="col-span-2 flex flex-wrap items-center gap-2 sm:col-span-1 sm:col-start-2">
            <BrandAction to={`/read/${hero.id}`} icon={Play}>
              {t("resume")}
            </BrandAction>
            <Button variant="secondary" asChild>
              <Link to={`/books/${hero.id}`}>{t("bookDetails")}</Link>
            </Button>
          </div>
        </section>
      )}

      {rows.map((row, ri) => (
        <Shelf key={row.title} title={t(row.title)}>
          {row.books
            ? row.books.map((b, i) => (
                <ShelfItem key={b.id}>
                  <BookCard book={b} seriesTitle={b.seriesTitle} priority={ri === 0 && i < 4} />
                </ShelfItem>
              ))
            : row.series?.map((s, i) => (
                <ShelfItem key={s.id}>
                  <SeriesCard series={s} priority={ri === 0 && i < 4} />
                </ShelfItem>
              ))}
        </Shelf>
      ))}
    </Page>
  );
}
