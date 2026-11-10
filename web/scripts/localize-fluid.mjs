// Fluid components ship a few hard-coded English strings (screen-reader
// names, one tooltip) with no prop to override them. Rather than hand-edit
// components/ui/, this script swaps them for Japanese. It is idempotent: run
// it again after any `shadcn add --overwrite` (npm run localize:fluid).
// When the app gets a real i18n layer, these become lookups instead.
import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const ui = (f) => fileURLToPath(new URL(`../src/components/ui/${f}`, import.meta.url));

const table = {
  "dialog.tsx": [['<span className="sr-only">Close</span>', '<span className="sr-only">閉じる</span>']],
  "sidebar-core.tsx": [
    ['"Collapse sidebar"', '"サイドバーを閉じる"'],
    ['"Expand sidebar"', '"サイドバーを開く"'],
    ['aria-label="Toggle Sidebar"', 'aria-label="サイドバーの表示切り替え"'],
    ['aria-label="Resize or collapse sidebar"', 'aria-label="サイドバーの幅を変える"'],
    ['aria-label="Peek sidebar"', 'aria-label="サイドバーを一時表示"'],
  ],
  "sidebar.tsx": [['className="sr-only">Sidebar<', 'className="sr-only">サイドバー<']],
  "sidebar-menu.tsx": [['<span className="sr-only">, unread</span>', '<span className="sr-only">、未読あり</span>']],
  "mobile-drawer.tsx": [['aria-label="Navigation"', 'aria-label="ナビゲーション"']],
  "input.tsx": [['direction === 1 ? "Increase" : "Decrease"', 'direction === 1 ? "増やす" : "減らす"']],
};

let changed = 0;
for (const [file, pairs] of Object.entries(table)) {
  const path = ui(file);
  let src = readFileSync(path, "utf8");
  for (const [en, ja] of pairs) {
    if (src.includes(en)) {
      src = src.split(en).join(ja);
      changed++;
    } else if (!src.includes(ja)) {
      console.warn(`localize-fluid: "${en}" not found in ${file} (component changed upstream?)`);
    }
  }
  writeFileSync(path, src);
}
console.log(`localize-fluid: ${changed} string(s) replaced`);
