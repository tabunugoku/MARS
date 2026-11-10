import type { Library } from "@/api";
import type { IconComponent } from "@/lib/icon-context";
import { Book, BookOpenText, Layers, Smartphone } from "@keyline-icons/react";

// One glyph per library kind. Placeholder until the kubai-icons set
// (folder-style animated library icons) lands — keep the mapping here.
const byKind: Record<Library["kind"], unknown> = {
  manga: BookOpenText,
  webtoon: Smartphone,
  comics: Layers,
  books: Book,
};

export const LibraryGlyph = (kind: Library["kind"]) => byKind[kind] as IconComponent;
