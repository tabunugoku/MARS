// Checks the built bundle against the JS budget in CLAUDE.md.
// Run after `vite build` (`npm run budget` does both).
//   initial: the entry chunk plus everything it imports statically, i.e. what
//            the browser must load before the first screen: ≤ 260 KB gzip.
//   lazy:    every other chunk (screens and what they share): ≤ 30 KB gzip each.
// Exits 1 when either budget is exceeded.
import fs from "node:fs";
import path from "node:path";
import zlib from "node:zlib";

const INITIAL_KB = 260;
const LAZY_KB = 30;

const web = path.resolve(import.meta.dirname, "..");
const dir = path.join(web, "dist/assets");
const html = fs.readFileSync(path.join(web, "dist/index.html"), "utf8");
const entry = html.match(/assets\/(index-[^"]+\.js)/)?.[1];
if (!entry) throw new Error("No entry chunk in dist/index.html. Run vite build first.");

const kb = (f) => zlib.gzipSync(fs.readFileSync(path.join(dir, f))).length / 1024;

// Static imports only: `import ... from "./x.js"` and bare `import "./x.js"`.
// Dynamic `import("./x.js")` is lazy and deliberately not followed.
const initial = new Set();
const walk = (f) => {
  if (initial.has(f)) return;
  initial.add(f);
  const src = fs.readFileSync(path.join(dir, f), "utf8");
  for (const m of src.matchAll(/(?:^|[;}\n])import\s*(?:[^'"()]*?from\s*)?["']\.\/([^"']+\.js)["']/g)) walk(m[1]);
};
walk(entry);

const initialKb = [...initial].reduce((sum, f) => sum + kb(f), 0);
const lazy = fs
  .readdirSync(dir)
  .filter((f) => f.endsWith(".js") && !initial.has(f))
  .map((f) => ({ f, size: kb(f) }))
  .sort((a, b) => b.size - a.size);
const over = lazy.filter((c) => c.size > LAZY_KB);

const line = (ok, text) => console.log(`${ok ? "ok  " : "OVER"}  ${text}`);
line(initialKb <= INITIAL_KB, `initial ${initialKb.toFixed(1)} / ${INITIAL_KB} KB gzip (${initial.size} files)`);
line(over.length === 0, `largest lazy chunk ${lazy[0]?.size.toFixed(1) ?? 0} / ${LAZY_KB} KB gzip (${lazy[0]?.f ?? "none"})`);
for (const c of over) console.log(`      ${c.size.toFixed(1).padStart(6)} KB  ${c.f}`);

if (initialKb > INITIAL_KB || over.length) process.exit(1);
