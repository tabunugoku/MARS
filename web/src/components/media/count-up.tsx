import { useEffect, useRef } from "react";
import { animate, useReducedMotion } from "motion/react";
import { intlLocale } from "@/i18n";

// A number that counts up to its value when it first appears or changes
// (Aimran "count-up number"). Writes text directly, no re-render per frame.
// Tabular figures so the width doesn't jitter while counting.
export function CountUp({ value, format = (n) => n.toLocaleString(intlLocale()), className }: { value: number; format?: (n: number) => string; className?: string }) {
  const ref = useRef<HTMLSpanElement>(null);
  const from = useRef(0);
  const reduce = useReducedMotion() ?? false;
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (reduce) {
      el.textContent = format(value);
      from.current = value;
      return;
    }
    const c = animate(from.current, value, {
      type: "spring",
      mass: 1,
      stiffness: 80,
      damping: 20,
      onUpdate: (v) => (el.textContent = format(Math.round(v))),
    });
    from.current = value;
    return () => c.stop();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value, reduce]);
  return (
    <span ref={ref} className={["tabular-nums", className].filter(Boolean).join(" ")}>
      {format(reduce ? value : 0)}
    </span>
  );
}
