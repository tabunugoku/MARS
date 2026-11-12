import { useRef, type ReactNode } from "react";
import { Link } from "react-router";
import { ChevronLeft, ChevronRight } from "@keyline-icons/react";
import { Button } from "@/components/ui/button";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";

/** Page frame: one max width and one gutter for every screen. */
export function Page({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("mx-auto w-full max-w-[1400px] px-4 pb-12 pt-2 md:px-8", className)}>{children}</div>;
}

export function PageHeader({ title, meta, actions, back, icon }: { title: ReactNode; meta?: ReactNode; actions?: ReactNode; back?: { to: string; label: string }; icon?: ReactNode }) {
  return (
    <header className="mb-6 flex flex-wrap items-end justify-between gap-3">
      <div className="flex min-w-0 items-center gap-3">
        {icon}
      <div className="min-w-0 space-y-1">
        {back && (
          <Link to={back.to} className={cn(typeClass("caption"), "inline-flex items-center gap-1 text-muted-foreground hover:text-foreground pointer-coarse:-my-3 pointer-coarse:py-3")}>
            <ChevronLeft size={14} />
            {back.label}
          </Link>
        )}
        <h1 className={cn(typeClass("display"), "truncate")} style={{ fontVariationSettings: fontWeights.bold }}>
          {title}
        </h1>
        {meta && <p className={cn(typeClass("body"), "text-muted-foreground")}>{meta}</p>}
      </div>
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </header>
  );
}

export function SectionTitle({ children, to, action }: { children: ReactNode; to?: string; action?: ReactNode }) {
  return (
    <div className="mb-3 flex items-baseline justify-between gap-2">
      <h2 className={typeClass("title")} style={{ fontVariationSettings: fontWeights.semibold }}>
        {children}
      </h2>
      {to ? (
        <Link to={to} className={cn(typeClass("caption"), "text-muted-foreground hover:text-foreground")}>
          すべて表示
        </Link>
      ) : (
        action
      )}
    </div>
  );
}

/** Responsive cover grid. Column width, not count, is fixed so density scales with the screen. */
export function CoverGrid({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className={cn("grid grid-cols-[repeat(auto-fill,minmax(128px,1fr))] gap-x-4 gap-y-6 md:grid-cols-[repeat(auto-fill,minmax(150px,1fr))]", className)}>
      {children}
    </div>
  );
}

/** Horizontal shelf with snap and paging buttons (desktop only; touch swipes). */
export function Shelf({ title, to, children }: { title: ReactNode; to?: string; children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  const page = (dir: 1 | -1) => ref.current?.scrollBy({ left: dir * ref.current.clientWidth * 0.85, behavior: "smooth" });
  return (
    <section className="mb-10">
      <SectionTitle
        to={to}
        action={
          // Paging arrows are for a mouse; on touch the shelf scrolls under the finger.
          <div className="hidden gap-1 md:pointer-fine:flex">
            <Button variant="ghost" size="icon-compact" aria-label="前へ" onClick={() => page(-1)}>
              <ChevronLeft />
            </Button>
            <Button variant="ghost" size="icon-compact" aria-label="次へ" onClick={() => page(1)}>
              <ChevronRight />
            </Button>
          </div>
        }
      >
        {title}
      </SectionTitle>
      <div ref={ref} className="shelf-scroll -mx-4 -my-3 flex scroll-px-4 gap-4 overflow-x-auto px-4 py-3 md:-mx-8 md:scroll-px-8 md:px-8">
        {children}
      </div>
    </section>
  );
}

/** Fixed-width slot for shelf items. */
export function ShelfItem({ children }: { children: ReactNode }) {
  return <div className="w-[132px] shrink-0 md:w-[150px]">{children}</div>;
}

export function EmptyState({ title, body, action }: { title: string; body?: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-border px-6 py-16 text-center">
      <p className={typeClass("title")} style={{ fontVariationSettings: fontWeights.semibold }}>
        {title}
      </p>
      {body && <p className={cn(typeClass("body"), "max-w-sm text-muted-foreground")}>{body}</p>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}
