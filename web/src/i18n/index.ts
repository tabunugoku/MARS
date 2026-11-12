import { useCallback, useSyncExternalStore } from "react";

// The interface language. See docs/i18n.md. Japanese is the source language;
// each file declares its own messages in both languages with defineMessages,
// so a lazy screen's strings load with its chunk.

export type Locale = "ja" | "en";
export type Params = Record<string, string | number>;
/** A message: text with {param} placeholders, or a function of the params. */
export type Msg = string | ((p: Params) => string);
export type Messages<K extends string> = { ja: Record<K, Msg>; en: Record<K, Msg> };

/** Both languages of one file's messages. The en side must have exactly the
 *  ja keys; the type checker reports a missing or extra one. */
export function defineMessages<const J extends Record<string, Msg>>(m: {
  ja: J;
  en: { [K in keyof J]: Msg };
}): Messages<Extract<keyof J, string>> {
  return m as Messages<Extract<keyof J, string>>;
}

const STORE_KEY = "cominavi.locale";

/** The browser's language, if it is one we have. */
export function browserLocale(): Locale | null {
  const langs = typeof navigator === "undefined" ? [] : navigator.languages?.length ? navigator.languages : [navigator.language];
  for (const l of langs) {
    const t = l?.toLowerCase() ?? "";
    if (t.startsWith("ja")) return "ja";
    if (t.startsWith("en")) return "en";
  }
  return null;
}

function stored(): Locale | null {
  try {
    const v = localStorage.getItem(STORE_KEY);
    return v === "ja" || v === "en" ? v : null;
  } catch {
    return null;
  }
}

let current: Locale = stored() ?? browserLocale() ?? "ja";
const listeners = new Set<() => void>();
if (typeof document !== "undefined") document.documentElement.lang = current;

export function getLocale(): Locale {
  return current;
}

/** Switch the language now; every useT/useLocale caller re-renders. */
export function setLocale(l: Locale) {
  try {
    localStorage.setItem(STORE_KEY, l);
  } catch {
    /* private mode: the choice lasts for this page only */
  }
  if (l === current) return;
  current = l;
  document.documentElement.lang = l;
  for (const f of listeners) f();
}

/** Apply a user's saved choice ("" follows the browser). */
export function applyUserLocale(choice: "" | Locale | undefined) {
  setLocale(choice || browserLocale() || "ja");
}

function subscribe(f: () => void) {
  listeners.add(f);
  return () => listeners.delete(f);
}

export function useLocale(): Locale {
  return useSyncExternalStore(subscribe, getLocale, getLocale);
}

/** The BCP 47 tag for Intl and toLocaleString. */
export function intlLocale(l: Locale = current): string {
  return l === "en" ? "en-US" : "ja-JP";
}

function format(msg: Msg, params?: Params): string {
  if (typeof msg === "function") return msg(params ?? {});
  if (!params) return msg;
  return msg.replace(/\{(\w+)\}/g, (all, k: string) => (k in params ? String(params[k]) : all));
}

/** A message in the current language, outside React. */
export function translate<K extends string>(m: Messages<K>, key: K, params?: Params): string {
  return format(m[current][key], params);
}

/** The translate function for one file's messages; re-renders on a
 *  language change. */
export function useT<K extends string>(m: Messages<K>) {
  const l = useLocale();
  return useCallback((key: K, params?: Params) => format(m[l][key], params), [m, l]);
}
