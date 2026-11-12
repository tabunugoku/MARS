// Live changes from GET /api/v1/events. One EventSource for the whole app,
// opened while some screen listens and closed a while after the last one
// leaves (or the tab has been hidden for a while). Import it only from
// screens (lazy chunks), never from the shell, so the initial bundle stays
// as it is.
import { useEffect, useRef, useState } from "react";
import { retryCovers } from "@/lib/cover-retry";
import type { components } from "./schema";

type S = components["schemas"];
export type LibraryEvent = S["LibraryEvent"];
export type BookEvent = S["BookEvent"];
export type ProgressEvent = S["ProgressEvent"];

export type LiveEvent =
  | { type: "task"; data: S["Task"] }
  | { type: "library"; data: LibraryEvent }
  | { type: "book"; data: BookEvent }
  | { type: "progress"; data: ProgressEvent }
  /** The stream came back after a gap: events may have been missed. */
  | { type: "reconnect" };

type Listener = (e: LiveEvent) => void;

const TYPES = ["task", "library", "book", "progress"] as const;
const LINGER_MS = 10_000; // keep the stream across a route change
const HIDDEN_MS = 30_000; // let a background tab give its connection back

const listeners = new Set<Listener>();
const upListeners = new Set<(up: boolean) => void>();
let source: EventSource | null = null;
let up = false;
let hadOpen = false; // a later open means a gap: tell screens to refetch
let retries = 0;
let retryTimer: ReturnType<typeof setTimeout> | undefined;
let closeTimer: ReturnType<typeof setTimeout> | undefined;

const enabled = () => import.meta.env.VITE_API !== "mock" && typeof EventSource !== "undefined";

let coverTimer: ReturnType<typeof setTimeout> | undefined;

function emit(e: LiveEvent) {
  for (const l of [...listeners]) l(e);
  // New thumbnails: covers that had none try again, at most every 2 s.
  if ((e.type === "book" || e.type === "reconnect") && !coverTimer) {
    coverTimer = setTimeout(() => {
      coverTimer = undefined;
      retryCovers();
    }, 2000);
  }
}

function setUp(v: boolean) {
  if (up === v) return;
  up = v;
  for (const l of [...upListeners]) l(v);
}

function open() {
  if (source || !enabled() || listeners.size === 0) return;
  if (typeof document !== "undefined" && document.visibilityState === "hidden") return;
  clearTimeout(retryTimer);
  retryTimer = undefined;
  const es = new EventSource("/api/v1/events");
  source = es;
  es.onopen = () => {
    if (hadOpen) emit({ type: "reconnect" });
    hadOpen = true;
    retries = 0;
    setUp(true);
  };
  es.onerror = () => {
    setUp(false);
    // The browser retries network errors by itself; an HTTP error (401, or
    // a proxy's 502 while the server restarts) closes the source for good.
    if (es.readyState === EventSource.CLOSED && source === es) {
      source = null;
      const wait = Math.min(30_000, 1000 * 2 ** retries++);
      retryTimer = setTimeout(open, wait);
    }
  };
  for (const t of TYPES) {
    es.addEventListener(t, (m) => {
      try {
        emit({ type: t, data: JSON.parse((m as MessageEvent<string>).data) } as LiveEvent);
      } catch {
        // A malformed message is skipped; the next one may be fine.
      }
    });
  }
}

/** Closes the stream. keepGap: the next open counts as a reconnect. */
function close(keepGap: boolean) {
  clearTimeout(retryTimer);
  retryTimer = undefined;
  source?.close();
  source = null;
  if (!keepGap) hadOpen = false;
  setUp(false);
}

if (typeof document !== "undefined") {
  document.addEventListener("visibilitychange", () => {
    clearTimeout(closeTimer);
    if (document.visibilityState === "hidden") {
      closeTimer = setTimeout(() => close(true), HIDDEN_MS);
    } else if (!source) {
      open();
    }
  });
}

function subscribe(l: Listener): () => void {
  listeners.add(l);
  clearTimeout(closeTimer);
  open();
  return () => {
    listeners.delete(l);
    if (listeners.size === 0) {
      clearTimeout(closeTimer);
      closeTimer = setTimeout(() => close(false), LINGER_MS);
    }
  };
}

/** Calls handler for every live event while the component is mounted. */
export function useLiveEvents(handler: Listener): void {
  const ref = useRef(handler);
  useEffect(() => {
    ref.current = handler;
  });
  useEffect(() => subscribe((e) => ref.current(e)), []);
}

/** Whether the stream is open now. */
export function useLiveConnected(): boolean {
  const [v, setV] = useState(up);
  useEffect(() => {
    upListeners.add(setV);
    setV(up);
    return () => {
      upListeners.delete(setV);
    };
  }, []);
  return v;
}

/** Runs reload when match says an event concerns the screen (and after a
 *  reconnect). Bursts are folded: it waits for `quiet` ms without a
 *  matching event, but never more than `maxWait` ms, so a long scan still
 *  refreshes the screen every few seconds. */
export function useLiveReload(
  reload: () => void,
  match: (e: Exclude<LiveEvent, { type: "reconnect" }>) => boolean,
  { quiet = 1000, maxWait = 5000 }: { quiet?: number; maxWait?: number } = {},
): void {
  const reloadRef = useRef(reload);
  const matchRef = useRef(match);
  useEffect(() => {
    reloadRef.current = reload;
    matchRef.current = match;
  });
  const timer = useRef<{ t?: ReturnType<typeof setTimeout>; first?: number }>({});
  useEffect(() => () => clearTimeout(timer.current.t), []);
  useLiveEvents((e) => {
    if (e.type !== "reconnect" && !matchRef.current(e)) return;
    const s = timer.current;
    const now = Date.now();
    s.first ??= now;
    clearTimeout(s.t);
    s.t = setTimeout(
      () => {
        s.first = undefined;
        s.t = undefined;
        reloadRef.current();
      },
      Math.max(0, Math.min(quiet, s.first + maxWait - now)),
    );
  });
}
