import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { flushSync } from "react-dom";

type Theme = "dark" | "light";
const ThemeCtx = createContext<{ theme: Theme; toggle: (from?: Element | null) => void }>({ theme: "dark", toggle: () => {} });

function applyTheme(t: Theme) {
  const root = document.documentElement;
  root.classList.toggle("dark", t === "dark");
  root.style.colorScheme = t;
  // The browser / installed app's title bar takes the page ground (the
  // --background tokens in styles/tokens.css, as hex).
  document.querySelector('meta[name="theme-color"]')?.setAttribute("content", t === "dark" ? "#09080e" : "#f6f6fa");
}

// Where the wipe starts: the element that asked for it, else the last pointer
// press (a menu item that has already closed), else the top-right corner.
let lastPress: { x: number; y: number; at: number } | null = null;
if (typeof window !== "undefined") {
  window.addEventListener("pointerdown", (e) => (lastPress = { x: e.clientX, y: e.clientY, at: performance.now() }), { capture: true, passive: true });
}
function originOf(from?: Element | null) {
  if (from && from.isConnected) {
    const r = from.getBoundingClientRect();
    if (r.width) return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
  }
  if (lastPress && performance.now() - lastPress.at < 1500) return lastPress;
  return { x: window.innerWidth - 32, y: 28 };
}

/** Dark is the default (night chart); light is chart paper. Persisted per
 *  browser. Switching prints the new theme over the old one as screentone
 *  dots spreading from the control (see `.cn-tone-wipe` in tokens.css). */
export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<Theme>(() =>
    (localStorage.getItem("cn-theme") as Theme | null) ?? "dark",
  );
  useEffect(() => {
    applyTheme(theme);
    localStorage.setItem("cn-theme", theme);
  }, [theme]);

  const toggle = useCallback(
    (from?: Element | null) => {
      const next: Theme = theme === "dark" ? "light" : "dark";
      const swap = () => {
        flushSync(() => setTheme(next));
        applyTheme(next);
      };
      const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
      if (reduce || !document.startViewTransition) return swap();
      const { x, y } = originOf(from);
      const reach = Math.hypot(Math.max(x, window.innerWidth - x), Math.max(y, window.innerHeight - y));
      const root = document.documentElement;
      root.style.setProperty("--cn-wipe-x", `${x}px`);
      root.style.setProperty("--cn-wipe-y", `${y}px`);
      root.style.setProperty("--cn-wipe-reach", `${Math.ceil(reach)}px`);
      root.classList.add("cn-tone-wipe");
      const vt = document.startViewTransition(swap);
      vt.finished.finally(() => root.classList.remove("cn-tone-wipe"));
    },
    [theme],
  );
  return <ThemeCtx.Provider value={{ theme, toggle }}>{children}</ThemeCtx.Provider>;
}

export const useTheme = () => useContext(ThemeCtx);
