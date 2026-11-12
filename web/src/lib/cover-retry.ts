// Covers that failed to load (the book was not analysed yet) wait here for
// a sign that thumbnails may exist now: the live event stream calls
// retryCovers() when books are analysed. Kept free of imports so the Cover
// primitive can use it at no cost to the initial bundle.
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
// keep their URL, so a loaded <img> would keep the old picture. A book
// event bumps the revision of the book's and its series' thumbnail URL;
// covers showing one load it again with ?r=<revision>.
const revisions = new Map<string, number>();
const watchers = new Map<string, Set<() => void>>();

/** The key revisions are kept under: the URL's path, without the query
 *  (thumbnail URLs carry ?v=<hash>, bumps use the bare path). */
function coverKey(src: string): string {
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

/** Marks a book's thumbnail (and its series') as changed. */
export function bumpCover(bookId: string, seriesId?: string): void {
  const urls = [`/api/v1/books/${encodeURIComponent(bookId)}/thumbnail`];
  if (seriesId) urls.push(`/api/v1/series/${encodeURIComponent(seriesId)}/thumbnail`);
  for (const u of urls) {
    revisions.set(u, (revisions.get(u) ?? 0) + 1);
    for (const f of [...(watchers.get(u) ?? [])]) f();
  }
}
