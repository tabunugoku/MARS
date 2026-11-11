// The one place that talks HTTP to the Go server (relative /api/v1, so the
// dev server's proxy and the embedded build both work).

export class ApiError extends Error {
  readonly status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
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
}

export async function request<T>(method: string, path: string, body?: unknown, opts: RequestOptions = {}): Promise<T> {
  const res = await fetch(BASE + path, {
    method,
    credentials: "same-origin",
    keepalive: opts.keepalive,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (res.ok) {
    const text = await res.text();
    return (text ? JSON.parse(text) : undefined) as T;
  }
  if (res.status === 404 && opts.nullOn404) return null as T;
  if (res.status === 401 && !opts.public) toLogin();
  let message = "サーバーに接続できませんでした";
  try {
    const e = (await res.json()) as { message?: string };
    if (e.message) message = e.message;
  } catch {
    /* not JSON: keep the generic message */
  }
  throw new ApiError(res.status, message);
}

/** Builds a query string; arrays are comma-joined (the contract uses explode: false). */
export function query(params: Record<string, string | number | boolean | string[] | undefined>): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || (Array.isArray(v) && v.length === 0)) continue;
    q.set(k, Array.isArray(v) ? v.join(",") : String(v));
  }
  const s = q.toString();
  return s ? `?${s}` : "";
}
