// Remotion ships its headless Chromium and its compositor (ffmpeg) for x64
// only. On Windows on arm64 the render runs x64 binaries under emulation:
// an installed Chrome, and the x64 compositor that `npm run setup:arm64`
// extracts into .x64/package. Both can be overridden with
// MARS_PROMO_BROWSER and MARS_PROMO_BINARIES. Other machines use Remotion's
// own downloads, as before.
import { Config } from "@remotion/cli/config";
import { existsSync } from "node:fs";
import { join } from "node:path";

const winArm64 = process.platform === "win32" && process.arch === "arm64";

const browser =
  process.env.MARS_PROMO_BROWSER ??
  (winArm64 ? "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe" : undefined);
if (browser) {
  Config.setBrowserExecutable(browser);
}

const binaries =
  process.env.MARS_PROMO_BINARIES ??
  (winArm64 ? join(process.cwd(), ".x64", "package") : undefined);
if (binaries) {
  if (!existsSync(binaries)) {
    throw new Error(`No x64 compositor in ${binaries}. Run "npm run setup:arm64" first.`);
  }
  Config.setBinariesDirectory(binaries);
}
