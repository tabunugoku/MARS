import type { Library } from "@/api";
import type { IconComponent } from "@/lib/icon-context";
import { Book, BookOpenText, Smartphone } from "@keyline-icons/react";

// Line glyph per library kind for navigation rows (text colour, 16px). The red
// tiles for the same kinds (library header, admin list) live in components/media/kind-icon.tsx.
const byKind: Record<Library["kind"], unknown> = {
  manga: BookOpenText,
  webtoon: Smartphone,
  // Manga and western comics are both books of panels: one glyph serves both.
  comics: BookOpenText,
  books: Book,
};

export const LibraryGlyph = (kind: Library["kind"]) => byKind[kind] as IconComponent;
