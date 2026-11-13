// Fluid components ship a few hard-coded English strings (screen-reader
// names, one tooltip) with no prop to override them, plus one layout rule
// MARS overrides (button icon padding). Rather than hand-edit
// components/ui/, this script swaps each string for a call to uiText("key")
// from src/i18n/ui.ts, which holds both languages. It is idempotent: run it
// again after any `shadcn add --overwrite` (npm run localize:fluid).
//
// Each entry: [upstream English, the Japanese text an earlier version of this
// script wrote (migrated on the first run), the uiText replacement].
import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const ui = (f) => fileURLToPath(new URL(`../src/components/ui/${f}`, import.meta.url));

const table = {
  "dialog.tsx": [
    ['<span className="sr-only">Close</span>', '<span className="sr-only">閉じる</span>', '<span className="sr-only">{uiText("close")}</span>'],
  ],
  "sidebar-core.tsx": [
    ['"Collapse sidebar"', '"サイドバーを閉じる"', 'uiText("collapseSidebar")'],
    ['"Expand sidebar"', '"サイドバーを開く"', 'uiText("expandSidebar")'],
    ['aria-label="Toggle Sidebar"', 'aria-label="サイドバーの表示切り替え"', 'aria-label={uiText("toggleSidebar")}'],
    ['aria-label="Resize or collapse sidebar"', 'aria-label="サイドバーの幅を変える"', 'aria-label={uiText("resizeSidebar")}'],
    ['aria-label="Peek sidebar"', 'aria-label="サイドバーを一時表示"', 'aria-label={uiText("peekSidebar")}'],
  ],
  "sidebar.tsx": [
    ['className="sr-only">Sidebar<', 'className="sr-only">サイドバー<', 'className="sr-only">{uiText("sidebar")}<'],
  ],
  "sidebar-menu.tsx": [
    ['<span className="sr-only">, unread</span>', '<span className="sr-only">、未読あり</span>', '<span className="sr-only">{uiText("unread")}</span>'],
  ],
  "mobile-drawer.tsx": [
    ['aria-label="Navigation"', 'aria-label="ナビゲーション"', 'aria-label={uiText("navigation")}'],
  ],
  "input.tsx": [
    ['direction === 1 ? "Increase" : "Decrease"', 'direction === 1 ? "増やす" : "減らす"', 'direction === 1 ? uiText("increase") : uiText("decrease")'],
  ],
};

// Not strings: MARS centres every button's content exactly, so an icon
// gets the same side padding as text (Fluid pulls it 4px closer).
const layout = {
  "button.tsx": [
    ['{ size: "compact", iconLeft: true, className: "pl-2" }', '{ size: "compact", iconLeft: true, className: "" }'],
    ['{ size: "default", iconLeft: true, className: "pl-3" }', '{ size: "default", iconLeft: true, className: "" }'],
    ['{ size: "compact", iconRight: true, className: "pr-2" }', '{ size: "compact", iconRight: true, className: "" }'],
    ['{ size: "default", iconRight: true, className: "pr-3" }', '{ size: "default", iconRight: true, className: "" }'],
  ],
};

const IMPORT = 'import { uiText } from "@/i18n/ui";';

/** Adds the uiText import after the last import, keeping the file's line endings. */
function addImport(src) {
  if (src.includes(IMPORT)) return src;
  const eol = src.includes("\r\n") ? "\r\n" : "\n";
  const lines = src.split(eol);
  let last = -1;
  for (let i = 0; i < lines.length; i++) {
    if (/^import\b/.test(lines[i]) || /^\} from ".*";\r?$/.test(lines[i])) last = i;
  }
  lines.splice(last + 1, 0, IMPORT);
  return lines.join(eol);
}

let changed = 0;

for (const [file, pairs] of Object.entries(table)) {
  const path = ui(file);
  let src = readFileSync(path, "utf8");
  let touched = false;
  for (const [en, ja, repl] of pairs) {
    if (src.includes(repl)) continue; // already localized
    if (src.includes(en)) src = src.split(en).join(repl);
    else if (src.includes(ja)) src = src.split(ja).join(repl);
    else {
      console.warn(`localize-fluid: "${en}" not found in ${file} (component changed upstream?)`);
      continue;
    }
    changed++;
    touched = true;
  }
  if (touched) src = addImport(src);
  writeFileSync(path, src);
}

for (const [file, pairs] of Object.entries(layout)) {
  const path = ui(file);
  let src = readFileSync(path, "utf8");
  for (const [en, repl] of pairs) {
    if (src.includes(en)) {
      src = src.split(en).join(repl);
      changed++;
    }
  }
  writeFileSync(path, src);
}
console.log(`localize-fluid: ${changed} replacement(s) made`);
