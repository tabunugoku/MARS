import { memo } from "react";
import { cn } from "@/lib/utils";
import { coverArt } from "@/lib/art";

export interface CoverProps {
  seed: number;
  title: string;
  label?: string;
  /** 0..1 reading progress; renders the bottom bar when between 0 and 1. */
  progress?: number;
  /** Count of unread books (series) — shown as a corner chip. */
  unread?: number;
  read?: boolean;
  className?: string;
  /** Eager-load covers above the fold; everything else is lazy. */
  priority?: boolean;
}

/** The one cover primitive. Fixed 2:3 box so grids never shift while art loads. */
export const Cover = memo(function Cover({ seed, title, label, progress, unread, read, className, priority }: CoverProps) {
  return (
    <div
      className={cn("relative aspect-[2/3] w-full overflow-hidden rounded-[6px] bg-muted", className)}
      style={{ boxShadow: "var(--cover-shadow)" }}
    >
      <img
        src={coverArt(seed, title, label)}
        alt=""
        loading={priority ? "eager" : "lazy"}
        decoding="async"
        draggable={false}
        className="size-full object-cover"
      />
      {unread !== undefined && unread > 0 && (
        <span className="absolute right-1.5 top-1.5 min-w-5 rounded-full bg-brand px-1.5 text-center text-[11px] leading-5 text-brand-foreground tabular-nums">
          {unread}
        </span>
      )}
      {read && (
        <span className="absolute right-1.5 top-1.5 rounded-full bg-black/60 px-1.5 text-[11px] leading-5 text-white backdrop-blur-sm">
          既読
        </span>
      )}
      {progress !== undefined && progress > 0 && progress < 1 && (
        <span className="absolute inset-x-0 bottom-0 h-1 bg-black/40" aria-hidden>
          <span className="block h-full bg-brand" style={{ width: `${Math.round(progress * 100)}%` }} />
        </span>
      )}
    </div>
  );
});
