// The MARS mark: a red planet on a night tile, a dotted orbit around it and
// two moons. The near moon is blue, the app's "you are here" colour (a
// Martian sunset is blue); the far one is pale dust. Flat, drawn in dots like
// the rest of the app. Literal colours (not theme tokens) so the mark is
// identical everywhere. public/favicon.svg and the PWA PNGs are drawn from
// the same geometry; keep the numbers below in step with the generator.

const TILE = "#1f1210";
const PLANET = "#e8542f";
const DUST = "#f3c9bd";
const HERE = "#5db2f7";
const RX = 13, RY = 5, ROT = -20, N = 40, DOT = 0.5, PR = 7;

function onOrbit(deg: number) {
  const t = (deg * Math.PI) / 180;
  const x = RX * Math.cos(t), y = RY * Math.sin(t);
  const r = (ROT * Math.PI) / 180;
  return { x: 16 + x * Math.cos(r) - y * Math.sin(r), y: 16 + x * Math.sin(r) + y * Math.cos(r), front: Math.sin(t) > 0 };
}

const near = onOrbit(40);
const far = onOrbit(200);
const dots = Array.from({ length: N }, (_, i) => onOrbit((360 * i) / N)).filter(
  (d) =>
    !(!d.front && Math.hypot(d.x - 16, d.y - 16) < PR + 0.9) &&
    ![near, far].some((m) => Math.hypot(d.x - m.x, d.y - m.y) < 2.6),
);

export function AppMark({ size = 24, className }: { size?: number; className?: string }) {
  return (
    <svg data-app-mark width={size} height={size} viewBox="0 0 32 32" aria-hidden className={className} focusable="false">
      <rect width="32" height="32" rx="8" fill={TILE} />
      {dots.filter((d) => !d.front).map((d, i) => (
        <circle key={`b${i}`} cx={d.x} cy={d.y} r={DOT} fill={DUST} fillOpacity={0.5} />
      ))}
      <circle cx="16" cy="16" r={PR} fill={PLANET} />
      {dots.filter((d) => d.front).map((d, i) => (
        <circle key={`f${i}`} cx={d.x} cy={d.y} r={DOT} fill={DUST} fillOpacity={0.85} />
      ))}
      <circle cx={far.x} cy={far.y} r="1.2" fill={DUST} />
      <circle cx={near.x} cy={near.y} r="2.1" fill={HERE} />
    </svg>
  );
}
