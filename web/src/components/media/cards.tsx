import { Link } from "react-router";
import type { Book, Series } from "@/api";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { defineMessages, useT } from "@/i18n";

const m = defineMessages({
  ja: {
    volumes: ({ n }) => `${n}冊`,
    pages: ({ n }) => `${n}ページ`,
    progress: ({ page, total }) => ` · ${page}/${total}`,
  },
  en: {
    volumes: ({ n }) => (n === 1 ? "1 volume" : `${n} volumes`),
    pages: ({ n }) => (n === 1 ? "1 page" : `${n} pages`),
    progress: ({ page, total }) => ` · ${page}/${total}`,
  },
});
import { Cover } from "./cover";

/** Spotlight (Aimran "spotlight card"): violet screentone pressed onto the
 *  cover around the pointer, like a tone sheet. Pure CSS from two
 *  custom properties the card writes on pointer move — no re-render. */
function trackPointer(e: React.PointerEvent<HTMLElement>) {
  if (e.pointerType !== "mouse") return;
  const r = e.currentTarget.getBoundingClientRect();
  e.currentTarget.style.setProperty("--sx", `${e.clientX - r.left}px`);
  e.currentTarget.style.setProperty("--sy", `${e.clientY - r.top}px`);
}
function Spotlight() {
  return (
    <span
      aria-hidden
      className="pointer-events-none absolute inset-0 rounded-[6px] opacity-0 transition-opacity duration-160 group-hover:opacity-90 motion-reduce:hidden [background-image:radial-gradient(var(--brand)_1.7px,transparent_2.1px)] [background-size:7px_7px] [mask-image:radial-gradient(90px_circle_at_var(--sx,50%)_var(--sy,50%),#000_20%,transparent_80%)]"
    />
  );
}

const titleCls = cn(typeClass("body"), "line-clamp-2 text-foreground");
const metaCls = cn(typeClass("caption"), "truncate text-muted-foreground");

/** Grid/shelf tile for a series. Whole tile is one link; hover lifts the cover only. */
export function SeriesCard({ series, priority, className }: { series: Series; priority?: boolean; className?: string }) {
  const t = useT(m);
  const unread = series.bookCount - series.booksRead;
  return (
    <Link
      to={`/series/${series.id}`}
      onPointerMove={trackPointer}
      className={cn("group block rounded-[8px] outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)] focus-visible:ring-offset-2 focus-visible:ring-offset-background", className)}
    >
      <div className="relative">
        <div data-cover-lift className="relative transition-transform duration-300 ease-[cubic-bezier(0.16,1,0.3,1)] group-hover:-translate-y-1 motion-reduce:transform-none">
        <Cover
          src={series.thumbnailUrl}
          title={series.title}
          priority={priority}
          unread={series.booksRead === 0 ? undefined : unread}
          read={unread === 0}
        />
          <Spotlight />
        </div>
      </div>
      <div className="mt-2 space-y-0.5 px-0.5">
        <p className={titleCls}>{series.title}</p>
        <p className={metaCls}>{t("volumes", { n: series.bookCount })}</p>
      </div>
    </Link>
  );
}

/** Grid/shelf tile for a book. `showSeries` prints the series title as meta (dashboard shelves). */
export function BookCard({ book, seriesTitle, priority, className, to }: { book: Book; seriesTitle?: string; priority?: boolean; className?: string; to?: string }) {
  const t = useT(m);
  const p = book.progress;
  const progress = p ? (p.completed ? 1 : p.page / book.pageCount) : undefined;
  return (
    <Link
      to={to ?? `/books/${book.id}`}
      onPointerMove={trackPointer}
      className={cn("group block rounded-[8px] outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)] focus-visible:ring-offset-2 focus-visible:ring-offset-background", className)}
    >
      <div className="relative">
        <div data-cover-lift className="relative transition-transform duration-300 ease-[cubic-bezier(0.16,1,0.3,1)] group-hover:-translate-y-1 motion-reduce:transform-none">
        <Cover src={book.thumbnailUrl} title={seriesTitle ?? book.title} label={String(book.number)} progress={progress} read={p?.completed} priority={priority} />
          <Spotlight />
        </div>
      </div>
      <div className="mt-2 space-y-0.5 px-0.5">
        <p className={titleCls}>{seriesTitle ?? book.title}</p>
        <p className={metaCls}>
          {seriesTitle ? book.title : t("pages", { n: book.pageCount })}
          {p && !p.completed && t("progress", { page: p.page, total: book.pageCount })}
        </p>
      </div>
    </Link>
  );
}
