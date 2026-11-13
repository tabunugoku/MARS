// Downloads Remotion's x64 compositor for Windows (ffmpeg, ffprobe and
// remotion.exe) into .x64/package, for rendering on Windows on arm64. See
// remotion.config.ts. The version follows the installed @remotion/cli.
import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, rmSync } from "node:fs";
import { join } from "node:path";

const root = new URL("..", import.meta.url);
const cli = JSON.parse(
  readFileSync(new URL("node_modules/@remotion/cli/package.json", root), "utf8"),
);
const dir = join(process.cwd(), ".x64");
rmSync(dir, { recursive: true, force: true });
mkdirSync(dir, { recursive: true });

const pkg = `@remotion/compositor-win32-x64-msvc@${cli.version}`;
const tgz = execFileSync("npm", ["pack", pkg, "--pack-destination", dir], {
  encoding: "utf8",
  shell: true,
})
  .trim()
  .split("\n")
  .pop();
// Relative paths only: GNU tar (Git Bash) reads "C:..." as a remote host.
execFileSync("tar", ["-xzf", tgz], { cwd: dir });
console.log(`Extracted ${pkg} into ${join(dir, "package")}`);
