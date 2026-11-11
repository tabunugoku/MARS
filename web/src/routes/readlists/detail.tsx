import { useRef } from "react";
import { DotTrack } from "@/components/media/dot-track";
import { Link, useParams } from "react-router";
import { Play } from "@keyline-icons/react";
import { api, useApi, type Book } from "@/api";
import { BrandAction } from "@/components/media/brand-action";
import { Cover } from "@/components/media/cover";
import { EmptyState, Page, PageHeader } from "@/components/media/layout";
import { Block } from "@/components/media/skeletons";
import { FluidHoverHighlight } from "@/components/fluid-hover-highlight";
import { useFluidHover, useRegisterFluidHoverItem } from "@/hooks/use-fluid-hover";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";

export default function Screen() {
  const { readListId = "" } = useParams();

  const { data } = useApi(async () => {
    const rl = await api.readList(readListId);
    if (!rl) return { rl: null, books: [] as Book[], titles: new Map<string, string>() };
    const [books, series] = await Promise.all([api.booksByIds(rl.bookIds), api.series()]);
    return { rl, books, titles: new Map(series.map((s) => [s.id, s.title])) };
  }, [readListId]);

  if (!data) {
    return (
      <Page>
        <Block className="mb-6 mt-2 h-8 w-56" />
        <div className="flex flex-col gap-2">
          {Array.from({ length: 6 }, (_, i) => (
            <Block key={i} className="h-14 w-full rounded-[8px]" />
          ))}
        </div>
      </Page>
    );
  }

  const { rl, books, titles } = data;
  if (!rl) {
    return (
      <Page>
        <EmptyState title="リーディングリストが見つかりません" />
      </Page>
    );
  }

  const next = books.find((b) => !b.progress?.completed) ?? books[0];

  return (
    <Page>
      <PageHeader
        back={{ to: "/readlists", label: "リーディングリスト" }}
        title={rl.name}
        meta={rl.summary || undefined}
        actions={next && <BrandAction to={`/read/${next.id}`} icon={Play}>最初から読む</BrandAction>}
      />
      {books.length === 0 ? (
        <EmptyState title="巻がありません" body="巻の詳細からこのリーディングリストに追加できます" />
      ) : (
        <BookRows books={books} titles={titles} />
      )}
    </Page>
  );
}

function BookRows({ books, titles }: { books: Book[]; titles: Map<string, string> }) {
  const ref = useRef<HTMLDivElement>(null);
  const hover = useFluidHover(ref);
  return (
    <div ref={ref} className="relative" {...hover.handlers}>
      <FluidHoverHighlight hover={hover} className="rounded-[8px]" />
      <ol>
        {books.map((b, i) => (
          <BookRow key={b.id} book={b} index={i} seriesTitle={titles.get(b.seriesId)} register={hover.registerItem} />
        ))}
      </ol>
    </div>
  );
}

function BookRow({
  book,
  index,
  seriesTitle,
  register,
}: {
  book: Book;
  index: number;
  seriesTitle?: string;
  register: (index: number, element: HTMLElement | null) => void;
}) {
  const ref = useRef<HTMLAnchorElement>(null);
  useRegisterFluidHoverItem(register, index, ref);
  const p = book.progress;
  const pct = p ? Math.min(100, Math.round((p.page / book.pageCount) * 100)) : 0;

  return (
    <li>
      <Link
        ref={ref}
        to={`/books/${book.id}`}
        className="relative z-10 flex items-center gap-3 rounded-[8px] px-2 py-2 outline-none focus-visible:ring-2 ring-[var(--focus-ring)]"
      >
        <span className={cn(typeClass("body"), "w-6 shrink-0 text-right text-muted-foreground tabular-nums")}>{index + 1}</span>
        <div className="w-10 shrink-0">
          <Cover src={book.thumbnailUrl} title={seriesTitle ?? book.title} label={String(book.number)} />
        </div>
        <div className="min-w-0 flex-1 space-y-0.5">
          <p className={cn(typeClass("caption"), "truncate text-muted-foreground")}>{seriesTitle}</p>
          <p className={cn(typeClass("body"), "truncate text-foreground")}>{book.title}</p>
        </div>
        <div className="w-24 shrink-0 text-right">
          {p?.completed ? (
            <span className={cn(typeClass("caption"), "text-muted-foreground")}>既読</span>
          ) : p ? (
            <div className="space-y-1">
              <DotTrack total={10} value={Math.round(pct / 10)} label="読書の進捗" size="sm" className="justify-end" />
              <p className={cn(typeClass("caption"), "font-mono text-muted-foreground tabular-nums")}>
                {p.page}/{book.pageCount}ページ
              </p>
            </div>
          ) : (
            <span className={cn(typeClass("caption"), "text-muted-foreground")}>未読</span>
          )}
        </div>
      </Link>
    </li>
  );
}
