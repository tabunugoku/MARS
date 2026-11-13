import { useLocation } from "react-router";
import { useTheme } from "@/components/shell/theme";
import { CompassRose } from "./compass-rose";

// The chart behind every screen: a screentone dot grid over the content panel
// and a large compass rose sitting a quarter in the bottom-right corner, drawn
// in dots and hairlines at low contrast, so its lines stay off the content. It
// is not drawn on phones (the content fills the width there). Its needle turns
// to a new heading on each route — the app navigating. Switching theme swings
// it half a turn.
// Pointer-transparent and hidden from assistive tech.

/** Stable heading (deg) per route section, so returning to a screen points the
 *  needle the same way. */
function heading(pathname: string) {
  const key = pathname.split("/").slice(0, 3).join("/");
  let h = 0;
  for (const ch of key) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return (h % 360) - 180;
}

export function CompassBackdrop() {
  const { pathname } = useLocation();
  const { theme } = useTheme();
  // Day and night point opposite ways: switching theme swings the needle 180°.
  const deg = heading(pathname) + (theme === "light" ? 180 : 0);

  return (
    <div aria-hidden className="pointer-events-none absolute inset-0 overflow-hidden">
      <div className="tone absolute inset-0 [mask-image:linear-gradient(to_bottom,#000,transparent_70%)] opacity-70" />
      <CompassRose
        deg={deg}
        className="fixed -bottom-[280px] -right-[280px] size-[560px] opacity-[0.11] max-md:hidden dark:opacity-[0.13]"
      />
    </div>
  );
}
