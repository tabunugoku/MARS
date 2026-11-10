import { useMemo } from "react";
import { useLocation } from "react-router";
import { motion } from "motion/react";
import { useTheme } from "@/components/shell/theme";

// The chart behind every screen: a screentone dot grid over the content panel
// and a large compass rose half off the bottom-right corner, drawn in dots and
// hairlines at low contrast. Its needle turns to a new heading on each route —
// the app navigating — on a spring that overshoots a little and settles; it
// never loops. Switching theme swings it half a turn. Pointer-transparent and hidden from assistive tech.

/** Stable heading (deg) per route section, so returning to a screen points the
 *  needle the same way. */
function heading(pathname: string) {
  const key = pathname.split("/").slice(0, 3).join("/");
  let h = 0;
  for (const ch of key) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return (h % 360) - 180;
}

const R = 260;

export function CompassBackdrop() {
  const { pathname } = useLocation();
  const { theme } = useTheme();
  // Day and night point opposite ways: switching theme swings the needle 180°.
  const deg = heading(pathname) + (theme === "light" ? 180 : 0);
  const ticks = useMemo(
    () =>
      Array.from({ length: 72 }, (_, i) => {
        const a = (i / 72) * Math.PI * 2;
        const major = i % 18 === 0;
        const mid = i % 6 === 0;
        const r = R - (major ? 14 : 8);
        return { x: Math.sin(a) * r, y: -Math.cos(a) * r, s: major ? 4 : mid ? 2.4 : 1.4, major };
      }),
    [],
  );
  const rings = useMemo(
    () =>
      [R * 0.72, R * 0.44].map((rr) => {
        const n = Math.round(rr / 7);
        return Array.from({ length: n }, (_, i) => {
          const a = (i / n) * Math.PI * 2;
          return { x: Math.sin(a) * rr, y: -Math.cos(a) * rr };
        });
      }),
    [],
  );

  return (
    <div aria-hidden className="pointer-events-none absolute inset-0 overflow-hidden">
      <div className="tone absolute inset-0 [mask-image:linear-gradient(to_bottom,#000,transparent_70%)] opacity-70" />
      <svg
        data-compass
        viewBox={`${-R - 8} ${-R - 8} ${2 * R + 16} ${2 * R + 16}`}
        className="fixed -bottom-[200px] -right-[180px] size-[560px] text-foreground opacity-[0.11] max-md:-right-[300px] max-md:-bottom-[260px] dark:opacity-[0.13]"
      >
        <circle r={R} fill="none" stroke="currentColor" strokeWidth="1" />
        {ticks.map((t, i) => (
          <circle key={i} cx={t.x} cy={t.y} r={t.s} fill="currentColor" />
        ))}
        {rings.map((ring, j) => ring.map((p, i) => <circle key={`${j}-${i}`} cx={p.x} cy={p.y} r="1.2" fill="currentColor" />))}
        <path d={`M0 ${-R * 0.9} L0 ${R * 0.9} M${-R * 0.9} 0 L${R * 0.9} 0`} stroke="currentColor" strokeWidth="1" strokeDasharray="2 8" />
        <motion.g
          initial={false}
          animate={{ rotate: deg }}
          transition={{ type: "spring", mass: 2.2, stiffness: 60, damping: 9 }}
        >
          <path d={`M0 ${-R * 0.82} L18 0 L0 18 L-18 0Z`} fill="currentColor" />
          <path d={`M0 ${R * 0.82} L18 0 L0 -18 L-18 0Z`} fill="none" stroke="currentColor" strokeWidth="1.5" />
        </motion.g>
        <circle r="6" fill="currentColor" />
      </svg>
    </div>
  );
}
