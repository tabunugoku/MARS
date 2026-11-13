import { useEffect, useMemo } from "react";
import { animate, motion, useMotionValue, useReducedMotion, useTransform } from "motion/react";
import { cn } from "@/lib/utils";

// The sign-in page's large mark: the logo's planet, orbit and moons drawn as
// a screentone print. The planet is a halftone sphere in brand red (each
// dot's size is the light falling on that spot, lit from the upper left), the
// orbit is a ring of dots tilted like the logo's, and the near moon is the
// blue "you are here". On mount the near moon travels along the orbit into
// the logo's position once, on a heavy spring; nothing loops. Decorative:
// always aria-hidden. Geometry follows components/brand/app-mark.tsx, scaled.

const R = 130; // planet
const RX = 270, RY = 104, ROT = -20; // orbit (the logo's 13 : 5 at -20°)
const GRID = 10; // halftone pitch
const NEAR = 40, FAR = 200; // moon angles (deg), as on the logo
const moonSpring = { type: "spring", mass: 2.2, stiffness: 40, damping: 14 } as const;

function onOrbit(deg: number) {
  const t = (deg * Math.PI) / 180;
  const x = RX * Math.cos(t), y = RY * Math.sin(t);
  const r = (ROT * Math.PI) / 180;
  return { x: x * Math.cos(r) - y * Math.sin(r), y: x * Math.sin(r) + y * Math.cos(r), front: Math.sin(t) > 0 };
}

// Light from the upper left and a little in front.
const L = (() => {
  const v = [-0.55, -0.6, 0.58];
  const n = Math.hypot(...v);
  return v.map((c) => c / n);
})();

export function PlanetOrbit({ className }: { className?: string }) {
  const reduce = useReducedMotion() ?? false;
  const halftone = useMemo(() => {
    const out: { x: number; y: number; r: number }[] = [];
    // Staggered rows (a hex screen), so the limb reads round, not stepped.
    const row = GRID * 0.866;
    for (let j = -Math.ceil(R / row); j * row <= R; j++) {
      const y = j * row;
      for (let x = -R + (j % 2 ? GRID / 2 : 0); x <= R; x += GRID) {
        const d2 = x * x + y * y;
        if (d2 > (R - 2) * (R - 2)) continue;
        const z = Math.sqrt(R * R - d2);
        const lit = Math.max(0, (x * L[0] + y * L[1] + z * L[2]) / R);
        out.push({ x, y, r: 0.9 + 4.1 * Math.pow(lit, 0.8) });
      }
    }
    return out;
  }, []);
  const orbit = useMemo(
    () =>
      Array.from({ length: 96 }, (_, i) => onOrbit((360 * i) / 96)).filter(
        (p) => !(!p.front && Math.hypot(p.x, p.y) < R + 8) && Math.hypot(p.x - onOrbit(FAR).x, p.y - onOrbit(FAR).y) > 18,
      ),
    [],
  );
  const far = onOrbit(FAR);

  // The near moon's angle on the orbit, animated once into place.
  const angle = useMotionValue(reduce ? NEAR : NEAR - 110);
  useEffect(() => {
    if (reduce) return;
    const c = animate(angle, NEAR, moonSpring);
    return () => c.stop();
  }, [angle, reduce]);
  const cx = useTransform(angle, (a) => onOrbit(a).x);
  const cy = useTransform(angle, (a) => onOrbit(a).y);

  return (
    <svg aria-hidden viewBox="-300 -300 600 600" className={cn("text-foreground", className)}>
      {orbit.filter((p) => !p.front).map((p, i) => (
        <circle key={`b${i}`} cx={p.x} cy={p.y} r="2" fill="currentColor" fillOpacity={0.28} />
      ))}
      <circle r={R} className="fill-brand" fillOpacity={0.1} />
      <g className="fill-brand">
        {halftone.map((d, i) => (
          <circle key={i} cx={d.x} cy={d.y} r={d.r} />
        ))}
      </g>
      {orbit.filter((p) => p.front).map((p, i) => (
        <circle key={`f${i}`} cx={p.x} cy={p.y} r="2" fill="currentColor" fillOpacity={0.55} />
      ))}
      <circle cx={far.x} cy={far.y} r="9" fill="currentColor" fillOpacity={0.6} />
      <motion.circle data-orbit-here cx={cx} cy={cy} r="15" className="fill-here" />
    </svg>
  );
}
