// Library-kind tile: the kind's Keyline glyph in violet on a flat violet-tint
// tile printed with screentone. Used where a library is introduced (the
// library header, the admin library list); navigation rows use the bare glyph.
import type { Library } from "@/api";
import { LibraryGlyph } from "@/components/shell/library-glyph";
import { cn } from "@/lib/utils";

export { AppMark } from "@/components/brand/app-mark";

export function KindIcon({ kind, size = 32, className }: { kind: Library["kind"]; size?: number; className?: string }) {
  const Glyph = LibraryGlyph(kind);
  return (
    <span
      aria-hidden
      className={cn("tone-strong relative grid shrink-0 place-items-center rounded-[10px] bg-brand-soft text-brand", className)}
      style={{ width: size, height: size, borderRadius: Math.round(size * 0.28) }}
    >
      <Glyph size={Math.round(size * 0.5)} strokeWidth={1.75} />
    </span>
  );
}
