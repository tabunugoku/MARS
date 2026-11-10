// Colour object icons (kubai-icons, exported as static SVG in 朱). They are
// pictures of things — the app mark and each library kind — so they appear
// where an object should read as an object: the logo, library headers, the
// admin library list. Navigation rows keep Keyline line icons.
import type { Library } from "@/api";
import cominavi from "@/assets/icons/cominavi.svg";
import manga from "@/assets/icons/manga.svg";
import webtoon from "@/assets/icons/webtoon.svg";
import comics from "@/assets/icons/comics.svg";
import { cn } from "@/lib/utils";

// Books (EPUB) has no object icon yet; it shares the tankobon until one is drawn.
const byKind: Record<Library["kind"], string> = { manga, webtoon, comics, books: manga };

export function AppMark({ size = 24, className }: { size?: number; className?: string }) {
  return <img src={cominavi} width={size} height={size} alt="" aria-hidden className={cn("shrink-0 select-none", className)} draggable={false} />;
}

export function KindIcon({ kind, size = 32, className }: { kind: Library["kind"]; size?: number; className?: string }) {
  return <img src={byKind[kind]} width={size} height={size} alt="" aria-hidden className={cn("shrink-0 select-none", className)} draggable={false} />;
}
