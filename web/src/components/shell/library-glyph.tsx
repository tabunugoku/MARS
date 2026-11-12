import type { Library } from "@/api";
import type { IconComponent } from "@/lib/icon-context";
import { Book, BookOpenText, GalleryVerticalEnd } from "@keyline-icons/react";

import { ComicsGlyph } from "./comic-glyphs";

// Line glyph per library kind for navigation rows (text colour, 16px). The violet
// tiles for the same kinds (library header, admin list) live in components/media/kind-icon.tsx.
const byKind: Record<Library["kind"], unknown> = {
  manga: BookOpenText,
  webtoon: GalleryVerticalEnd,
  comics: ComicsGlyph,
  books: Book,
};

export const LibraryGlyph = (kind: Library["kind"]) => byKind[kind] as IconComponent;
