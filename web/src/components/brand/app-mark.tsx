// The ComiNavi mark: a compass star on a violet tile, flat. The north arm is
// cyan, the same "you are here" colour the app uses for reading position.
// Literal colours (not theme tokens) so the mark is identical everywhere,
// including the favicon (public/favicon.svg mirrors this drawing).
export function AppMark({ size = 24, className }: { size?: number; className?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden className={className} focusable="false">
      <rect width="32" height="32" rx="8" fill="#8B47FF" />
      {Array.from({ length: 12 }, (_, i) => {
        const a = (i / 12) * Math.PI * 2;
        return <circle key={i} cx={16 + Math.sin(a) * 11.5} cy={16 - Math.cos(a) * 11.5} r={i % 3 === 0 ? 1.2 : 0.8} fill="#C9B3FF" />;
      })}
      <path d="M16 5.5 18.3 13.7 26.5 16 18.3 18.3 16 26.5 13.7 18.3 5.5 16 13.7 13.7Z" fill="#EDE6FF" />
      <path d="M16 5.5 18.3 13.7 16 16 13.7 13.7Z" fill="#47BFFF" />
      <circle cx="16" cy="16" r="1.7" fill="#8B47FF" />
    </svg>
  );
}
