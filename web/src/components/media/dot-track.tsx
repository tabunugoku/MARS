import { useLayoutEffect, useRef, useState } from "react";
import { motion, useReducedMotion } from "motion/react";
import { cn } from "@/lib/utils";

// Progress drawn as one row of dots, like a route on a chart: read dots are
// violet, the current position is one cyan dot ("you are here"), the rest are
// faint. The row holds as many dots as fit its width (never more than `max`,
// e.g. a book's page count), and `percent` (0..100) colours that share of
// them. Dots fill in left to right on mount (a short stagger), once.

const DOT = { sm: 6, md: 8 };
const GAP = { sm: 4, md: 6 };
// Room around the row for the current dot's ring (it is not clipped: the dot
// count already fits the width), taken back by a negative
// margin so the dots still line up with the text above.
const PAD = { sm: 2, md: 4 };

export function DotTrack({
  percent,
  max = Infinity,
  label,
  size = "md",
  className,
}: {
  percent: number;
  max?: number;
  label: string;
  size?: "sm" | "md";
  className?: string;
}) {
  const reduce = useReducedMotion() ?? false;
  const row = useRef<HTMLDivElement>(null);
  const [fit, setFit] = useState(0);
  useLayoutEffect(() => {
    const el = row.current;
    if (!el) return;
    const measure = () => setFit(Math.floor((el.clientWidth - 2 * PAD[size] + GAP[size]) / (DOT[size] + GAP[size])));
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [size]);

  const pct = Math.max(0, Math.min(100, percent));
  const n = Math.max(1, Math.min(fit, Math.floor(max)));
  const done = Math.round((pct / 100) * n);
  const here = done < n ? done : -1;
  const d = size === "sm" ? "size-1.5" : "size-2";
  return (
    <div
      ref={row}
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(pct)}
      className={cn("flex", n === fit && "justify-between", size === "sm" ? "-mx-0.5 gap-1 p-0.5" : "-mx-1 gap-1.5 p-1", className)}
    >
      {fit > 0 &&
        Array.from({ length: n }, (_, i) => (
          <motion.span
            key={i}
            className={cn(
              "shrink-0 rounded-full",
              d,
              i < done ? "bg-brand" : i === here ? "bg-here ring-[3px] ring-here/25" : "bg-foreground/12",
            )}
            initial={reduce ? false : { scale: 0.2, opacity: 0 }}
            animate={{ scale: i === here ? 1.25 : 1, opacity: 1 }}
            transition={{ type: "spring", mass: 0.4, stiffness: 420, damping: 18, delay: reduce ? 0 : Math.min(i, 40) * 0.012 }}
          />
        ))}
    </div>
  );
}
