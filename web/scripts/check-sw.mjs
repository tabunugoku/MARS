// Checks the built service worker (dist/sw.js) after `vite build`:
//   - it is stamped with this build (VERSION is not the source placeholder),
//     so a deploy installs a new worker and prunes old caches;
//   - every precached entry file exists in dist, or install would fail;
//   - it still leaves /api alone (data, page images, the event stream).
// Run by `npm run budget`. Exits 1 on a problem.
import fs from "node:fs";
import path from "node:path";

const dist = path.resolve(import.meta.dirname, "../dist");
const sw = fs.readFileSync(path.join(dist, "sw.js"), "utf8");
const problems = [];

const version = sw.match(/const VERSION = "([^"]+)";/)?.[1];
if (!version || version === "dev") problems.push("VERSION is not stamped with the build");
const entry = JSON.parse(sw.match(/const ENTRY = (\[.*?\]);/)?.[1] ?? "[]");
if (entry.length === 0) problems.push("ENTRY is empty: the offline shell would have no scripts");
for (const f of entry) if (!fs.existsSync(path.join(dist, f))) problems.push(`ENTRY lists a missing file: ${f}`);
if (!sw.includes('url.pathname.startsWith("/api/")')) problems.push("the /api/ bypass is gone");

for (const p of problems) console.log(`FAIL  sw.js: ${p}`);
if (problems.length) process.exit(1);
console.log(`ok    sw.js stamped ${version}, ${entry.length} entry files precached`);
