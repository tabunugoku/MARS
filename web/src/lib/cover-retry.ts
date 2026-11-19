// Covers show text until their thumbnail loads (the book was not analysed
// yet, or its thumbnail is being made). Two things bring them up to date.
// Kept free of imports so the Cover primitive costs nothing in the initial
// bundle.
//
// 1. A book event bumps the revision of the book's and its series' thumbnail
//    URL. Live events go through queueCoverBump, which folds a burst into one
//    bump per cover; bumpCover is immediate, for a user action that has just
//    changed a cover. A cover showing that URL loads the new picture under
//    ?r=<revision>. A text cover is subscribed to the same revision, so a bump
//    of its book or series is its retry.
// 2. A reconnect of the live stream may have missed events: retryCovers() lets
//    every text cover try once more.
const waiting = new Set<() => void>();

/** Calls retry once at the next retryCovers(); returns an unsubscribe. */
export function onCoverRetry(retry: () => void): () => void {
  waiting.add(retry);
  return () => {
    waiting.delete(retry);
  };
}

export function retryCovers(): void {
  const all = [...waiting];
  waiting.clear();
  for (const f of all) f();
}

// Thumbnails made again (a new cover page, a regeneration, a re-analysis)
// keep their URL, so a loaded <img> would keep the old picture. A bump of the
// book's and its series' thumbnail URL makes covers showing one load it again
// with ?r=<revision>.
const revisions = new Map<string, number>();
const watchers = new Map<string, Set<() => void>>();

/** How long a burst of live book events is folded into one bump per cover. */
const BUMP_DELAY_MS = 1000;
const queued = new Set<string>();
let flushTimer: ReturnType<typeof setTimeout> | undefined;

/** The key revisions are kept under: the URL's path, without the query
 *  (thumbnail URLs carry ?v=<hash>, bumps use the bare path). */
export function coverKey(src: string): string {
  const cut = src.search(/[?#]/);
  return cut === -1 ? src : src.slice(0, cut);
}

/** The revision of a thumbnail URL: 0 until it changed in this tab. */
export function coverRevision(src: string): number {
  return revisions.get(coverKey(src)) ?? 0;
}

/** Calls f whenever src's revision changes; returns an unsubscribe. */
export function onCoverRevision(src: string, f: () => void): () => void {
  const key = coverKey(src);
  let set = watchers.get(key);
  if (!set) watchers.set(key, (set = new Set()));
  set.add(f);
  return () => {
    set.delete(f);
    if (set.size === 0) watchers.delete(key);
  };
}

/** The thumbnail URL paths of a book and, when given, of its series. */
function thumbKeys(bookId: string, seriesId?: string): string[] {
  const urls = [`/api/v1/books/${encodeURIComponent(bookId)}/thumbnail`];
  if (seriesId) urls.push(`/api/v1/series/${encodeURIComponent(seriesId)}/thumbnail`);
  return urls;
}

function bump(u: string): void {
  revisions.set(u, (revisions.get(u) ?? 0) + 1);
  for (const f of [...(watchers.get(u) ?? [])]) f();
}

/** Marks a book's thumbnail (and its series') as changed, now. */
export function bumpCover(bookId: string, seriesId?: string): void {
  for (const u of thumbKeys(bookId, seriesId)) bump(u);
}

/** Marks a book's thumbnail (and its series') as changed, once the burst of
 *  live events has passed: a scan that makes many thumbnails sends one event
 *  per book, and a series cover would otherwise change once per book. */
export function queueCoverBump(bookId: string, seriesId?: string): void {
  for (const u of thumbKeys(bookId, seriesId)) queued.add(u);
  flushTimer ??= setTimeout(flushCoverBumps, BUMP_DELAY_MS);
}

function flushCoverBumps(): void {
  flushTimer = undefined;
  const keys = [...queued];
  queued.clear();
  for (const u of keys) bump(u);
}
