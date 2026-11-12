import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "@yusufceng1z/ding-react";
import { errorMessage } from "@/api";

/** The server takes at most this many ids per bulk call. */
export const BULK_LIMIT = 1000;

type Picked = ReadonlySet<string>;
const NONE: Picked = new Set();

/** Selection of cards on one screen. `visible` is the ids in display order
 *  (shift-click selects the range between two of them). `scopeKey` changes
 *  whenever the list changes (library, sort, filter): the selection then ends.
 *  `complete` says `visible` is the whole list, not a loaded window of it. */
export function useSelection(visible: string[], scopeKey: string, complete = true) {
  const [state, setState] = useState<{ on: boolean; key: string; picked: Picked }>({ on: false, key: scopeKey, picked: NONE });
  // The shift-click anchor only counts within the scope it was set in.
  const anchor = useRef<{ key: string; id: string } | null>(null);
  // A new scope ends the selection for good: going back to the old scope must not
  // bring its picks back. Reset while rendering, so the first frame already shows it off.
  const [seenKey, setSeenKey] = useState(scopeKey);
  if (seenKey !== scopeKey) {
    setSeenKey(scopeKey);
    setState({ on: false, key: scopeKey, picked: NONE });
  }
  const active = state.on && state.key === scopeKey;
  const picked = active ? state.picked : NONE;

  const enter = useCallback(() => {
    anchor.current = null;
    setState({ on: true, key: scopeKey, picked: NONE });
  }, [scopeKey]);

  const exit = useCallback(() => {
    anchor.current = null;
    setState((s) => ({ on: false, key: s.key, picked: NONE }));
  }, []);

  const clear = useCallback(() => {
    anchor.current = null;
    setState((s) => (s.on ? { ...s, picked: NONE } : s));
  }, []);

  const selectAll = useCallback(() => {
    setState((s) => (s.on && s.key === scopeKey ? { ...s, picked: new Set(visible) } : s));
  }, [scopeKey, visible]);

  // A pick whose id left the list (a scan removed the series or book) is dropped:
  // the bulk calls fail whole on a missing id. Absence proves removal only when
  // both renders held the whole list. A paged window can shift (a new series sorts
  // ahead of it) or be capped on reload, so there a pick that left the window
  // stays; if it was really removed, the bulk call fails and the picks are kept
  // for the user to change. A new scope already ended the selection above.
  const lastVisible = useRef<Set<string> | null>(null);
  useEffect(() => {
    const now = complete ? new Set(visible) : null;
    const before = lastVisible.current;
    lastVisible.current = now;
    if (!before || !now) return;
    const gone = [...before].filter((id) => !now.has(id));
    if (gone.length === 0) return;
    setState((s) => {
      if (!gone.some((id) => s.picked.has(id))) return s;
      const next = new Set(s.picked);
      for (const id of gone) next.delete(id);
      return { ...s, picked: next };
    });
  }, [visible, complete]);

  const toggle = (id: string, shift = false) => {
    const from = anchor.current?.key === scopeKey ? anchor.current.id : null;
    const fromIndex = from ? visible.indexOf(from) : -1;
    const toIndex = visible.indexOf(id);
    anchor.current = { key: scopeKey, id };
    setState((s) => {
      if (!s.on || s.key !== scopeKey) return s;
      const next = new Set(s.picked);
      if (shift && fromIndex >= 0 && toIndex >= 0) {
        // The range takes the clicked item's new state.
        const on = !s.picked.has(id);
        const [lo, hi] = fromIndex < toIndex ? [fromIndex, toIndex] : [toIndex, fromIndex];
        for (let i = lo; i <= hi; i++) {
          if (on) next.add(visible[i]);
          else next.delete(visible[i]);
        }
      } else if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return { ...s, picked: next };
    });
  };

  // Esc leaves the mode, unless a dialog is open (it closes on Esc itself).
  useEffect(() => {
    if (!active) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || e.defaultPrevented) return;
      if (document.querySelector('[role="dialog"]')) return;
      exit();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [active, exit]);

  return {
    active,
    enter,
    exit,
    clear,
    selectAll,
    toggle,
    has: (id: string) => picked.has(id),
    count: picked.size,
    ids: Array.from(picked),
  };
}

export type Selection = ReturnType<typeof useSelection>;

/** Runs one bulk call with a toast. `after(ok)` runs whether the call worked or not
 *  (the visible data may have changed); only a successful call should clear the
 *  selection, so a failed one can be retried with the same ids. */
export function useBulkRun(after: (ok: boolean) => void) {
  const [busy, setBusy] = useState(false);
  const run = async (call: () => Promise<number>, done: (n: number) => string) => {
    setBusy(true);
    let ok = false;
    try {
      const n = await call();
      toast.success(done(n));
      ok = true;
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusy(false);
      after(ok);
    }
  };
  return { busy, run };
}

/** Splits ids into calls of at most BULK_LIMIT. */
export function chunked<T>(xs: T[], size = BULK_LIMIT): T[][] {
  const out: T[][] = [];
  for (let i = 0; i < xs.length; i += size) out.push(xs.slice(i, i + size));
  return out;
}

/** Runs `call` on each chunk one after another; the sum of what each reports. */
export async function inChunks(ids: string[], call: (chunk: string[]) => Promise<number>): Promise<number> {
  let total = 0;
  for (const chunk of chunked(ids)) total += await call(chunk);
  return total;
}

/** A comma or Japanese comma separated list, trimmed, without repeats. */
export function splitList(text: string): string[] {
  return [...new Set(text.split(/[,、]/).map((s) => s.trim()).filter(Boolean))];
}

