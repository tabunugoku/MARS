// The one place that talks HTTP to the Go server (relative /api/v1, so the
// dev server's proxy and the embedded build both work).

import { defineMessages, getLocale, translate } from "@/i18n";

const m = defineMessages({
  ja: {
    offline: "サーバーに接続できませんでした",
  },
  en: {
    offline: "Could not reach the server",
  },
});

export class ApiError extends Error {
  readonly status: number;
  /** The server's stable error code (e.g. "not_found.book"); "" when absent. */
  readonly code: string;
  constructor(status: number, message: string, code = "") {
    super(message);
    this.status = status;
    this.code = code;
  }
}

const BASE = "/api/v1";

/** Where a signed-out request sends the user. Public calls (sign-in page,
 *  setup) opt out, and the sign-in page itself never redirects to itself. */
function toLogin() {
  if (location.pathname === "/login") return;
  const next = location.pathname + location.search;
  location.assign(`/login?next=${encodeURIComponent(next)}`);
}

/** A same-origin relative path to return to after sign-in, or "/". */
export function safeNext(raw: string | null): string {
  if (!raw || !raw.startsWith("/") || raw.startsWith("//") || raw.startsWith("/\\")) return "/";
  return raw;
}

export interface RequestOptions {
  /** Do not redirect to the sign-in page on 401. */
  public?: boolean;
  /** Answer null instead of throwing on 404 (by-id reads). */
  nullOn404?: boolean;
  /** Survive page unload (progress saved on pagehide). */
  keepalive?: boolean;
  /** Sees the response headers of a successful call. */
  onHeaders?: (headers: Headers) => void;
}

export async function request<T>(method: string, path: string, body?: unknown, opts: RequestOptions = {}): Promise<T> {
  const res = await fetch(BASE + path, {
    method,
    credentials: "same-origin",
    keepalive: opts.keepalive,
    headers: {
      "Accept-Language": getLocale(),
      ...(body === undefined ? {} : { "Content-Type": body instanceof Blob ? "application/octet-stream" : "application/json" }),
    },
    // A Blob (an uploaded file) goes as it is; anything else as JSON.
    body: body === undefined ? undefined : body instanceof Blob ? body : JSON.stringify(body),
  });
  if (res.ok) {
    opts.onHeaders?.(res.headers);
    const text = await res.text();
    return (text ? JSON.parse(text) : undefined) as T;
  }
  if (res.status === 404 && opts.nullOn404) return null as T;
  if (res.status === 401 && !opts.public) toLogin();
  let message = translate(m, "offline");
  let code = "";
  try {
    const e = (await res.json()) as { message?: string; code?: string };
    if (e.message) message = e.message;
    if (e.code) code = e.code;
  } catch {
    /* not JSON: keep the generic message */
  }
  throw new ApiError(res.status, message, code);
}

/** GETs one page of a list; X-Total-Count is the length of the whole list. */
export async function requestPage<T>(path: string): Promise<{ items: T[]; total: number }> {
  let total: number | undefined;
  const items = await request<T[]>("GET", path, undefined, {
    onHeaders: (h) => {
      const n = Number(h.get("X-Total-Count"));
      if (h.has("X-Total-Count") && Number.isFinite(n)) total = n;
    },
  });
  return { items, total: total ?? items.length };
}

/** Free-text list filters the contract sends as repeated parameters
 *  (explode: true), since their values may contain commas. */
const REPEATED = new Set(["genres", "tags", "publishers", "languages", "authors"]);

/** Builds a query string. Arrays are comma-joined (explode: false), except
 *  the keys in REPEATED, which repeat the parameter once per value. */
export function query(params: Record<string, string | number | boolean | string[] | undefined>): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || (Array.isArray(v) && v.length === 0)) continue;
    if (Array.isArray(v) && REPEATED.has(k)) for (const x of v) q.append(k, x);
    else q.set(k, Array.isArray(v) ? v.join(",") : String(v));
  }
  const s = q.toString();
  return s ? `?${s}` : "";
}
