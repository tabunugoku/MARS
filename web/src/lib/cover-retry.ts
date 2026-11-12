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
