// Library-kind tile: the kind's Keyline glyph in red on a flat red-tint
// tile (no screentone: the dots made the small glyph hard to read). Used in the
// admin library list and the libraries screen; navigation rows use the bare glyph.
import type { Library } from "@/api";
import { LibraryGlyph } from "@/components/shell/library-glyph";
import { cn } from "@/lib/utils";

export { AppMark } from "@/components/brand/app-mark";

export function KindIcon({ kind, size = 32, className }: { kind: Library["kind"]; size?: number; className?: string }) {
  const Glyph = LibraryGlyph(kind);
  return (
    <span
      aria-hidden
      className={cn("relative grid shrink-0 place-items-center bg-brand-soft text-brand", className)}
      style={{ width: size, height: size, borderRadius: Math.round(size * 0.25) }}
    >
      <Glyph size={Math.round(size * 0.5)} strokeWidth={1.75} />
    </span>
  );
}
