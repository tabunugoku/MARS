import { Link } from "react-router";
import { Play } from "@keyline-icons/react";
import { api, useApi, type Book, type Series } from "@/api";
import { BookCard, SeriesCard } from "@/components/media/cards";
import { Cover } from "@/components/media/cover";
import { BrandAction } from "@/components/media/brand-action";
import { EmptyState, Page, Shelf, ShelfItem } from "@/components/media/layout";
import { Block, CoverSkeleton } from "@/components/media/skeletons";
import { Button } from "@/components/ui/button";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";

type Row = { title: string; books?: Book[]; series?: Series[] };

export default function Screen() {
  const { data } = useApi(
    async () => {
      const [keep, deck, addedBooks, addedSeries, updatedSeries, read, all] = await Promise.all([
        api.keepReading(),
        api.onDeck(),
        api.recentlyAddedBooks(),
        api.recentlyAddedSeries(),
        api.recentlyUpdatedSeries(),
        api.recentlyRead(),
        api.series(),
      ]);
      return {
        keep,
        deck,
        addedBooks,
        addedSeries,
        updatedSeries,
        read,
        titles: new Map(all.map((s) => [s.id, s.title])),
      };
    },
    [],
  );

  if (!data) {
    return (
      <Page>
        <h1 className="sr-only">ホーム</h1>
        <section className="mb-10 flex flex-col gap-4 rounded-[12px] bg-card p-4 sm:flex-row md:p-5" aria-hidden>
          <Block className="aspect-[2/3] w-[120px] shrink-0 rounded-[6px]" />
          <div className="flex min-w-0 flex-1 flex-col gap-2 pt-1">
            <Block className="h-5 w-2/3" />
            <Block className="h-4 w-1/2" />
            <Block className="mt-auto h-1 w-full rounded-full" />
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
  const titles = data.titles;
  const rows: Row[] = [
    { title: "読みかけ", books: data.keep.slice(1) },
    { title: "次に読む", books: data.deck },
    { title: "最近追加された巻", books: data.addedBooks },
    { title: "新しいシリーズ", series: data.addedSeries },
    { title: "最近更新されたシリーズ", series: data.updatedSeries },
    { title: "最近読んだ巻", books: data.read },
  ].filter((r) => (r.books ?? r.series ?? []).length > 0);

  if (!hero && rows.length === 0) {
    return (
      <Page>
        <h1 className="sr-only">ホーム</h1>
        <EmptyState
          title="まだ本がありません"
          body="管理 → ライブラリ設定でフォルダを追加すると、ここに表示されます"
          action={<BrandAction to="/admin/libraries">ライブラリを追加</BrandAction>}
        />
      </Page>
    );
  }

  const heroSeriesTitle = hero ? (titles.get(hero.seriesId) ?? hero.title) : "";
  const heroPage = hero?.progress?.page ?? 0;
  const heroPct = hero ? Math.min(100, Math.round((heroPage / hero.pageCount) * 100)) : 0;

  return (
    <Page>
      <h1 className="sr-only">ホーム</h1>

      {hero && (
        <section className="mb-10 flex flex-col gap-4 rounded-[12px] bg-card p-4 sm:flex-row md:p-5">
          <div className="w-[120px] shrink-0">
            <Cover seed={hero.coverSeed} title={heroSeriesTitle} label={String(hero.number)} />
          </div>
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <h2 className={cn(typeClass("title"), "line-clamp-2")} style={{ fontVariationSettings: fontWeights.semibold }}>
              {heroSeriesTitle}
            </h2>
            <p className={cn(typeClass("body"), "line-clamp-2 text-muted-foreground")}>{hero.title}</p>
            <div className="mt-auto space-y-1.5 pt-3">
              <div
                role="progressbar"
                aria-label="読書の進捗"
                aria-valuemin={0}
                aria-valuemax={100}
                aria-valuenow={heroPct}
                className="h-1 w-full overflow-hidden rounded-full bg-muted"
              >
                <div className="h-full rounded-full bg-brand" style={{ width: `${heroPct}%` }} />
              </div>
              <p className={cn(typeClass("caption"), "text-muted-foreground tabular-nums")}>
                残り {hero.pageCount - heroPage} ページ
              </p>
            </div>
            <div className="mt-3 flex flex-wrap items-center gap-2">
              <BrandAction to={`/read/${hero.id}`} icon={Play}>
                続きから読む
              </BrandAction>
              <Button variant="ghost" asChild>
                <Link to={`/books/${hero.id}`}>巻の詳細</Link>
              </Button>
            </div>
          </div>
        </section>
      )}

      {rows.map((row, ri) => (
        <Shelf key={row.title} title={row.title}>
          {row.books
            ? row.books.map((b, i) => (
                <ShelfItem key={b.id}>
                  <BookCard book={b} seriesTitle={titles.get(b.seriesId)} priority={ri === 0 && i < 4} />
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
