// Copies the production build into the Go server's embed folder
// (server/internal/webui/dist), keeping its .gitkeep. Run via `npm run build:embed`.
import fs from "node:fs";
import path from "node:path";

const web = path.resolve(import.meta.dirname, "..");
const from = path.join(web, "dist");
const to = path.resolve(web, "../server/internal/webui/dist");

for (const entry of fs.readdirSync(to)) {
  if (entry !== ".gitkeep") fs.rmSync(path.join(to, entry), { recursive: true, force: true });
}
fs.cpSync(from, to, { recursive: true });
console.log(`copied ${path.relative(process.cwd(), from)} -> ${path.relative(process.cwd(), to)}`);
