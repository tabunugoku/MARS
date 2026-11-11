import { useMemo } from "react";
import { motion } from "motion/react";
import { cn } from "@/lib/utils";

// The compass rose of the night chart, drawn in dots and hairlines in
// currentColor. Shared by the backdrop behind every screen and the sign-in
// page. The needle turns to `deg` on a heavy spring that overshoots a little
// and settles; it never loops. Decorative: always aria-hidden.

const R = 260;
const needleSpring = { type: "spring", mass: 2.2, stiffness: 60, damping: 9 } as const;

export function CompassRose({ deg, from, className }: { deg: number; from?: number; className?: string }) {
  const ticks = useMemo(
    () =>
      Array.from({ length: 72 }, (_, i) => {
        const a = (i / 72) * Math.PI * 2;
        const major = i % 18 === 0;
        const mid = i % 6 === 0;
        const r = R - (major ? 14 : 8);
        return { x: Math.sin(a) * r, y: -Math.cos(a) * r, s: major ? 4 : mid ? 2.4 : 1.4 };
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
    <svg
      aria-hidden
      data-compass
      viewBox={`${-R - 8} ${-R - 8} ${2 * R + 16} ${2 * R + 16}`}
      className={cn("text-foreground", className)}
    >
      <circle r={R} fill="none" stroke="currentColor" strokeWidth="1" />
      {ticks.map((t, i) => (
        <circle key={i} cx={t.x} cy={t.y} r={t.s} fill="currentColor" />
      ))}
      {rings.map((ring, j) => ring.map((p, i) => <circle key={`${j}-${i}`} cx={p.x} cy={p.y} r="1.2" fill="currentColor" />))}
      <path d={`M0 ${-R * 0.9} L0 ${R * 0.9} M${-R * 0.9} 0 L${R * 0.9} 0`} stroke="currentColor" strokeWidth="1" strokeDasharray="2 8" />
      <motion.g initial={from === undefined ? false : { rotate: from }} animate={{ rotate: deg }} transition={needleSpring}>
        <path d={`M0 ${-R * 0.82} L18 0 L0 18 L-18 0Z`} fill="currentColor" />
        <path d={`M0 ${R * 0.82} L18 0 L0 -18 L-18 0Z`} fill="none" stroke="currentColor" strokeWidth="1.5" />
      </motion.g>
      <circle r="6" fill="currentColor" />
    </svg>
  );
}
