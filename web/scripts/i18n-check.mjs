// Fails when Japanese text is left outside a defineMessages `ja:` block in
// web/src (see docs/i18n.md). Comments are ignored, and so are the invented
// mock data, the mock client and the i18n module itself.
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative, sep } from "node:path";

const root = new URL("../src/", import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, "$1");
const SKIP = ["mock" + sep, join("api", "mock.ts"), "i18n" + sep];
// Language names are shown in their own language whatever the UI language.
const ALLOWED = ["日本語"];
const JP = /[぀-ヿ㐀-鿿！-｠]/;

function* files(dir) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) yield* files(p);
    else if (/\.(ts|tsx)$/.test(name) && !name.endsWith(".d.ts")) yield p;
  }
}

// Blank out comments and every `ja: { ... }` object, keeping line breaks so
// line numbers still match. Strings and template literals are skipped over
// when matching braces.
function strip(src) {
  const out = src.split("");
  const blank = (a, b) => {
    for (let i = a; i < b; i++) if (out[i] !== "\n") out[i] = " ";
  };
  let i = 0;
  const skipString = (q) => {
    i++;
    while (i < src.length && src[i] !== q) {
      if (src[i] === "\\") i++;
      i++;
    }
    i++;
  };
  while (i < src.length) {
    const c = src[i];
    if (c === "/" && src[i + 1] === "/") {
      const e = src.indexOf("\n", i);
      const end = e < 0 ? src.length : e;
      blank(i, end);
      i = end;
    } else if (c === "/" && src[i + 1] === "*") {
      const e = src.indexOf("*/", i + 2);
      const end = e < 0 ? src.length : e + 2;
      blank(i, end);
      i = end;
    } else if (c === '"' || c === "'" || c === "`") {
      skipString(c);
    } else if (c === "{" && /\bja\s*:\s*$/.test(src.slice(Math.max(0, i - 20), i))) {
      // c opens a ja: block; find its match.
      const start = i;
      let depth = 0;
      while (i < src.length) {
        const d = src[i];
        if (d === '"' || d === "'" || d === "`") {
          skipString(d);
          continue;
        }
        if (d === "{") depth++;
        if (d === "}" && --depth === 0) {
          i++;
          break;
        }
        i++;
      }
      blank(start, i);
    } else {
      i++;
    }
  }
  return out.join("");
}

let bad = 0;
for (const p of files(root)) {
  const rel = relative(root, p);
  if (SKIP.some((s) => rel.startsWith(s) || rel === s)) continue;
  let text = strip(readFileSync(p, "utf8"));
  for (const a of ALLOWED) text = text.split(a).join("");
  text.split("\n").forEach((line, n) => {
    if (JP.test(line)) {
      bad++;
      console.log(`src/${rel.split(sep).join("/")}:${n + 1}: ${line.trim().slice(0, 100)}`);
    }
  });
}
if (bad) {
  console.log(`\n${bad} line(s) with Japanese outside a defineMessages ja: block. See docs/i18n.md.`);
  process.exit(1);
}
console.log("i18n:check OK");
