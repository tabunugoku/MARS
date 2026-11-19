import { memo, useEffect, useRef, useState } from "react";
import { coverKey, coverRevision, onCoverRetry, onCoverRevision } from "@/lib/cover-retry";
import { cn } from "@/lib/utils";
import { defineMessages, useT } from "@/i18n";

const m = defineMessages({
  ja: {
    read: "既読",
  },
  en: {
    read: "Read",
  },
});

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

/** The URL of a thumbnail at a revision: a bump adds ?r=<revision> (see cover-retry.ts). */
function withRevision(src: string, rev: number): string {
  return rev && src.startsWith("/") ? `${src}${src.includes("?") ? "&" : "?"}r=${rev}` : src;
}

/** Whether src may have a picture. A book or series thumbnail URL without
 *  ?v= means the server has none yet: it is shown as text and not requested
 *  (a bump or a reconnect asks again, see cover-retry.ts). */
function mayHavePicture(src: string): boolean {
  return !!src && !(src.startsWith("/") && /\/thumbnail$/.test(coverKey(src)) && !/[?&]v=/.test(src));
}

/** One shared observer for every cover: a callback per element, told whether
 *  the element is near the viewport. Created on first use. */
const nearCallbacks = new Map<Element, (near: boolean) => void>();
let nearObserver: IntersectionObserver | null = null;

/** Calls cb with whether el is near the viewport (300px margin). Without
 *  IntersectionObserver the element counts as near. Returns the cleanup. */
function observeNear(el: Element, cb: (near: boolean) => void): () => void {
  if (typeof IntersectionObserver === "undefined") {
    cb(true);
    return () => {};
  }
  if (!nearObserver) {
    nearObserver = new IntersectionObserver(
      (entries) => {
        for (const e of entries) nearCallbacks.get(e.target)?.(e.isIntersecting);
      },
      { rootMargin: "300px" },
    );
  }
  nearCallbacks.set(el, cb);
  nearObserver.observe(el);
  return () => {
    nearObserver?.unobserve(el);
    nearCallbacks.delete(el);
  };
}

/** The one cover primitive. Fixed 2:3 box so grids never shift while art loads. */
export const Cover = memo(function Cover({ src, title, label, progress, unread, read, className, priority }: CoverProps) {
  // A book without a thumbnail yet (not analysed, or no image) answers 404.
  // The text stays until a picture is there: a new or changed thumbnail is
  // loaded in the background and the <img> switches only once it has
  // decoded. So a cover does not blink to text while thumbnails are made,
  // the old picture is not replaced before the new one is ready, and no
  // <img> is rendered that is expected to 404.
  const t = useT(m);
  // Revisions are keyed by the URL's path (see cover-retry.ts), so the
  // ?v=hash on src does not hide a bump.
  const path = coverKey(src);
  const [rev, setRev] = useState(() => coverRevision(src));
  useEffect(() => {
    if (!src) return;
    setRev(coverRevision(src));
    return onCoverRevision(src, () => setRev(coverRevision(src)));
  }, [src]);
  const url = withRevision(src, rev);
  // The picture on screen, or null for the text. The first render shows the
  // picture at once: the browser loads it natively.
  const [shown, setShown] = useState<string | null>(() => (mayHavePicture(src) ? withRevision(src, coverRevision(src)) : null));
  // A URL whose picture failed (or that has none, see mayHavePicture) is not
  // requested again until a retry.
  const [failed, setFailed] = useState<string | null>(() => (src && !mayHavePicture(src) ? withRevision(src, coverRevision(src)) : null));
  // Counts the retries of a text cover (see below).
  const [attempt, setAttempt] = useState(0);
  // The list may hand this component another cover's path (a rotating hero,
  // an index-keyed list). That cover starts over like a first render: its own
  // picture, or the text. The previous cover's picture must never stay up.
  const [owner, setOwner] = useState(path);
  if (owner !== path) {
    setOwner(path);
    setRev(coverRevision(src));
    setShown(mayHavePicture(src) ? withRevision(src, coverRevision(src)) : null);
    setFailed(src && !mayHavePicture(src) ? withRevision(src, coverRevision(src)) : null);
  }
  // A text cover tries again when the live stream reconnects (see cover-retry.ts).
  // Subscribed again after each retry, which is the only way it is called.
  useEffect(
    () =>
      !shown && src
        ? onCoverRetry(() => {
            setFailed(null);
            setAttempt((n) => n + 1);
          })
        : undefined,
    [shown, src, attempt],
  );
  // Whether the cover is near the viewport. Off-screen covers do not preload,
  // so a library-wide thumbnail bump fetches only what the reader can see.
  const boxRef = useRef<HTMLDivElement>(null);
  const [near, setNear] = useState(!!priority);
  useEffect(() => {
    const el = boxRef.current;
    if (!el) return;
    return observeNear(el, setNear);
  }, []);
  // Loads the current URL in the background. A failed load keeps what is
  // shown; a stale one (a newer URL, or unmount) is dropped.
  useEffect(() => {
    if (!near || !url || url === shown || url === failed) return;
    let cancelled = false;
    const img = new Image();
    img.src = url;
    img.decode().then(
      () => {
        if (!cancelled) setShown(url);
      },
      () => {},
    );
    return () => {
      cancelled = true;
    };
  }, [near, url, shown, failed, attempt]);
  const picture = src ? shown : null;
  return (
    <div
      ref={boxRef}
      className={cn("relative aspect-[2/3] w-full overflow-hidden rounded-[6px] bg-muted", className)}
      style={{ boxShadow: "var(--cover-shadow)" }}
    >
      {picture ? (
        <img
          src={picture}
          alt=""
          loading={priority ? "eager" : "lazy"}
          decoding="async"
          draggable={false}
          // The failed picture is dropped and its URL is not requested again
          // by the background load; the text comes back until a bump or retry.
          onError={() => {
            setFailed(picture);
            setShown((s) => (s === picture ? null : s));
          }}
          className="size-full object-cover"
        />
      ) : (
        <span className="flex size-full flex-col justify-end gap-0.5 p-2 text-[11px] leading-4 text-muted-foreground">
          <span className="line-clamp-3 break-words">{title}</span>
          {label && <span className="tabular-nums">{label}</span>}
        </span>
      )}
      {unread !== undefined && unread > 0 && (
        <span className="absolute bottom-2 right-1.5 min-w-5 rounded-full bg-brand-fill px-1.5 text-center text-[11px] leading-5 text-brand-foreground tabular-nums">
          {unread}
        </span>
      )}
      {read && (
        <span className="absolute bottom-2 right-1.5 rounded-full bg-black/70 px-1.5 text-[11px] leading-5 text-white">
          {t("read")}
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
