import { memo, useEffect, useState } from "react";
import { onCoverRetry } from "@/lib/cover-retry";
import { cn } from "@/lib/utils";

export interface CoverProps {
  /** The thumbnail URL from the API (thumbnailUrl). */
  src: string;
  title: string;
  /** Shown with the title when there is no cover image (volume number). */
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
export const Cover = memo(function Cover({ src, title, label, progress, unread, read, className, priority }: CoverProps) {
  // A book without a thumbnail yet (not analysed, or no image) answers 404.
  const [failed, setFailed] = useState<string | null>(null);
  const missing = !src || failed === src;
  // Try again once books have been analysed (see live events).
  useEffect(() => (missing && src ? onCoverRetry(() => setFailed(null)) : undefined), [missing, src]);
  return (
    <div
      className={cn("relative aspect-[2/3] w-full overflow-hidden rounded-[6px] bg-muted", className)}
      style={{ boxShadow: "var(--cover-shadow)" }}
    >
      {missing ? (
        <span className="flex size-full flex-col justify-end gap-0.5 p-2 text-[11px] leading-4 text-muted-foreground">
          <span className="line-clamp-3 break-words">{title}</span>
          {label && <span className="tabular-nums">{label}</span>}
        </span>
      ) : (
        <img
          src={src}
          alt=""
          loading={priority ? "eager" : "lazy"}
          decoding="async"
          draggable={false}
          onError={() => setFailed(src)}
          className="size-full object-cover"
        />
      )}
      {unread !== undefined && unread > 0 && (
        <span className="absolute bottom-2 right-1.5 min-w-5 rounded-full bg-brand px-1.5 text-center text-[11px] leading-5 text-brand-foreground tabular-nums">
          {unread}
        </span>
      )}
      {read && (
        <span className="absolute bottom-2 right-1.5 rounded-full bg-black/70 px-1.5 text-[11px] leading-5 text-white">
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
