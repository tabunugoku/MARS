// The ComiNavi mark: a compass on a violet tile, flat, drawn like the compass
// backdrop — a ring of dots and one needle. The needle's north half is cyan
// (the app's "you are here" colour), the south half a quiet lavender, tilted
// so it reads as a needle pointing somewhere rather than a cross.
// Literal colours (not theme tokens) so the mark is identical everywhere;
// public/favicon.svg mirrors this drawing.
export const MARK_TILT = 35;

export function AppMark({ size = 24, className }: { size?: number; className?: string }) {
  return (
    <svg data-app-mark width={size} height={size} viewBox="0 0 32 32" aria-hidden className={className} focusable="false">
      <rect width="32" height="32" rx="8" fill="#8B47FF" />
      {Array.from({ length: 12 }, (_, i) => {
        const a = (i / 12) * Math.PI * 2;
        return <circle key={i} cx={16 + Math.sin(a) * 11.5} cy={16 - Math.cos(a) * 11.5} r={i % 3 === 0 ? 1.3 : 0.8} fill="#C9B3FF" />;
      })}
      <g transform={`rotate(${MARK_TILT} 16 16)`}>
        <path d="M16 5.2 19.4 16 12.6 16Z" fill="#47BFFF" />
        <path d="M16 26.8 19.4 16 12.6 16Z" fill="#EDE6FF" />
      </g>
      <circle cx="16" cy="16" r="1.8" fill="#8B47FF" />
    </svg>
  );
}
