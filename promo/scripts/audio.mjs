// Generates the promo's background music and sound effects with the
// ElevenLabs API. The key is read from ELEVENLABS_API_KEY and never printed.
// Output goes to public/audio/ (gitignored); rerun with --force to replace
// files that already exist.
import { mkdir, writeFile, access } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";

const key = process.env.ELEVENLABS_API_KEY;
if (!key) {
  console.error("ELEVENLABS_API_KEY is not set.");
  process.exit(1);
}
const force = process.argv.includes("--force");
const outDir = path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "public", "audio");
await mkdir(outDir, { recursive: true });

const API = "https://api.elevenlabs.io/v1";

const music = {
  file: "bgm.mp3",
  url: `${API}/music`,
  body: {
    prompt:
      "Calm, warm instrumental electronic track for a software product video. Soft analog synth pads, " +
      "gentle plucked arpeggio, light hand-played percussion, a slow steady pulse around 96 BPM. " +
      "Builds slightly in the middle, resolves softly at the end. No vocals.",
    music_length_ms: 56000,
    force_instrumental: true,
  },
};

const effects = [
  ["page-turn.mp3", "A single crisp paper manga page turning, close and clean, no room noise", 1.0],
  ["tap.mp3", "A soft, short UI tap click, muted and pleasant, like a quiet button press", 0.5],
  ["whoosh.mp3", "A gentle airy whoosh for a smooth screen transition, soft and short", 1.0],
  ["scroll.mp3", "Smooth continuous paper sliding sound, like scrolling a long vertical comic strip", 1.5],
  ["chime.mp3", "A small bright two-note chime, warm and friendly, for a logo reveal", 1.5],
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
