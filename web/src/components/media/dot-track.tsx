import { motion, useReducedMotion } from "motion/react";
import { cn } from "@/lib/utils";

// Progress drawn as a row of dots, like a route on a chart: read dots are
// violet, the current position is one cyan dot ("you are here"), the rest are
// faint. Dots fill in left to right on mount (a short stagger), once.
// `total` is the number of dots; `value` is how many are done (0..total).

export function DotTrack({
  total,
  value,
  label,
  size = "md",
  className,
}: {
  total: number;
  value: number;
  label: string;
  size?: "sm" | "md";
  className?: string;
}) {
  const reduce = useReducedMotion() ?? false;
  const n = Math.max(1, Math.round(total));
  const done = Math.max(0, Math.min(n, Math.round(value)));
  const here = done < n ? done : -1;
  const d = size === "sm" ? "size-1.5" : "size-2";
  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={n}
      aria-valuenow={done}
      className={cn("flex flex-wrap", size === "sm" ? "gap-1" : "gap-1.5", className)}
    >
      {Array.from({ length: n }, (_, i) => (
        <motion.span
          key={i}
          className={cn(
            "rounded-full",
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
