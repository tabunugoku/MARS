// Generates the promo's background music and sound effects with the
// ElevenLabs API. The key is read from ELEVENLABS_API_KEY and never printed.
// Output goes to public/audio/ (gitignored). Existing files are kept; pass
// --force to replace them, or name files to make only those
// (`node scripts/audio.mjs --force bgm.mp3`).
import { mkdir, writeFile, access } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";

const key = process.env.ELEVENLABS_API_KEY;
if (!key) {
  console.error("ELEVENLABS_API_KEY is not set.");
  process.exit(1);
}
const args = process.argv.slice(2);
const force = args.includes("--force");
const only = args.filter((a) => a.endsWith(".mp3"));
const outDir = path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "public", "audio");
await mkdir(outDir, { recursive: true });

const API = "https://api.elevenlabs.io/v1";

// The video is 58 s; the music is made a little longer and faded out in
// the composition.
const music = {
  file: "bgm.mp3",
  url: `${API}/music`,
  body: {
    prompt:
      "Quiet, warm night-time instrumental for a short video about a reading app. Soft felt piano, " +
      "gentle analog synth pads and a light, steady pulse around 90 BPM with brushed percussion. " +
      "Starts sparse, a soft beat enters after about 8 seconds, lifts a little around 40 seconds, " +
      "and ends on a held chord. Calm and unhurried. No vocals.",
    music_length_ms: 60000,
    force_instrumental: true,
  },
};

const effects = [
  ["page-turn.mp3", "A single crisp paper manga page being turned by hand, close and clean, no room noise", 0.8],
  ["tap.mp3", "A soft, short click of a trackpad tap, muted and quiet, then silence", 0.5],
  ["whoosh.mp3", "A very soft, airy whoosh for a window sliding into place, short and subtle", 0.8],
  ["typing.mp3", "Quiet laptop keyboard typing, steady and even, about ten keystrokes per second, close and dry", 3.0],
  ["chime.mp3", "A small, warm two-note bell chime, soft attack, gentle decay, for a logo at the end of a video", 2.0],
].map(([file, text, seconds]) => ({
  file,
  url: `${API}/sound-generation`,
  body: { text, duration_seconds: seconds, prompt_influence: 0.6 },
}));

async function exists(p) {
  try {
    await access(p);
    return true;
  } catch {
    return false;
  }
}

for (const job of [music, ...effects]) {
  if (only.length && !only.includes(job.file)) continue;
  const dest = path.join(outDir, job.file);
  if (!force && (await exists(dest))) {
    console.log(`skip ${job.file} (exists)`);
    continue;
  }
  const res = await fetch(job.url, {
    method: "POST",
    headers: { "xi-api-key": key, "Content-Type": "application/json", Accept: "audio/mpeg" },
    body: JSON.stringify(job.body),
  });
  if (!res.ok) {
    const detail = (await res.text()).slice(0, 400);
    console.error(`failed ${job.file}: HTTP ${res.status} ${detail}`);
    process.exitCode = 1;
    continue;
  }
  const buf = Buffer.from(await res.arrayBuffer());
  await writeFile(dest, buf);
  console.log(`wrote ${job.file} (${Math.round(buf.length / 1024)} KB)`);
}
