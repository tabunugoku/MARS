import { Link } from "react-router";
import type { Book, Series } from "@/api";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { Cover } from "./cover";

const titleCls = cn(typeClass("body"), "line-clamp-2 text-foreground");
const metaCls = cn(typeClass("caption"), "truncate text-muted-foreground");

/** Grid/shelf tile for a series. Whole tile is one link; hover lifts the cover only. */
export function SeriesCard({ series, priority, className }: { series: Series; priority?: boolean; className?: string }) {
  const unread = series.bookCount - series.booksRead;
  return (
    <Link
      to={`/series/${series.id}`}
      className={cn("group block rounded-[8px] outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)] focus-visible:ring-offset-2 focus-visible:ring-offset-background", className)}
    >
      <div className="transition-transform duration-160 ease-[cubic-bezier(0.16,1,0.3,1)] group-hover:-translate-y-0.5 motion-reduce:transform-none">
        <Cover
          seed={series.coverSeed}
          title={series.title}
          priority={priority}
          unread={series.booksRead === 0 ? undefined : unread}
          read={unread === 0}
        />
      </div>
      <div className="mt-2 space-y-0.5 px-0.5">
        <p className={titleCls}>{series.title}</p>
        <p className={metaCls}>{series.bookCount}冊</p>
      </div>
    </Link>
  );
}

/** Grid/shelf tile for a book. `showSeries` prints the series title as meta (dashboard shelves). */
export function BookCard({ book, seriesTitle, priority, className, to }: { book: Book; seriesTitle?: string; priority?: boolean; className?: string; to?: string }) {
  const p = book.progress;
  const progress = p ? (p.completed ? 1 : p.page / book.pageCount) : undefined;
  return (
    <Link
      to={to ?? `/books/${book.id}`}
      className={cn("group block rounded-[8px] outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)] focus-visible:ring-offset-2 focus-visible:ring-offset-background", className)}
    >
      <div className="transition-transform duration-160 ease-[cubic-bezier(0.16,1,0.3,1)] group-hover:-translate-y-0.5 motion-reduce:transform-none">
        <Cover seed={book.coverSeed} title={seriesTitle ?? book.title} label={String(book.number)} progress={progress} read={p?.completed} priority={priority} />
      </div>
      <div className="mt-2 space-y-0.5 px-0.5">
        <p className={titleCls}>{seriesTitle ?? book.title}</p>
        <p className={metaCls}>
          {seriesTitle ? book.title : `${book.pageCount}ページ`}
          {p && !p.completed && ` · ${p.page}/${book.pageCount}`}
        </p>
      </div>
    </Link>
  );
}
