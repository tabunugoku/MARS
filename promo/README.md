# MARS promo video

A 58-second promo for MARS, built with [Remotion](https://www.remotion.dev).
It has no narration, only captions. Two versions render from one component:
`promo-en` and `promo-ja` (1920x1080, 30 fps, 1740 frames). The Japanese
captions are the original; the English captions are translations of them.

## Scenes

| Frames | Scene | Caption (ja) |
|---|---|---|
| 0–105 | Title: the mark, "MARS", "A Reading Server" | — |
| 105–300 | A comics folder beside the library | マンガのフォルダを、MARS が読み込みます |
| 300–480 | A series page, closing in on its books | シリーズと巻に、自動でまとまります |
| 480–690 | The reader turns a page, right to left | ブラウザで、そのまま読めます |
| 690–855 | Reading direction: right to left, then vertical scroll | 右から左も、縦スクロールも |
| 855–1020 | Home: the cursor clicks "Resume" | 続きは、読んだページから |
| 1020–1200 | Two phones: home and reader | スマホでは、ホーム画面に追加できます |
| 1200–1365 | Mihon, OPDS 1.2 and the browser | Mihon や OPDS のアプリからも読めます |
| 1365–1545 | The `docker run` command typed out | Docker のコマンド 1 行で起動します |
| 1545–1740 | Tagline and repository | — |

Each scene overlaps the next by 8 frames. Every motion is a spring
(`src/motion.ts`): mass, stiffness and damping come from the `feel` skill's
vocabulary, so no motion has a hand-picked duration (the typed command
advances at a steady rate of characters per frame).

## Files

- `src/Promo.tsx` — the scenes, captions and audio cues
- `src/Root.tsx` — the two compositions (`promo-en`, `promo-ja`)
- `src/motion.ts` — the spring presets
- `src/captions.ts` — the caption type and the text shared by both versions
- `src/captions.ja.ts`, `src/captions.en.ts` — caption text (keep the keys in sync)
- `public/fonts/` — Geist (SIL OFL 1.1). Japanese text uses Noto Sans JP from the system.
- `public/shots/` — app screenshots (gitignored)
- `public/audio/` — background music and sound effects (gitignored)
- `out/` — rendered MP4s (gitignored)

## Regenerate the audio

The music and sound effects come from the ElevenLabs API:

```sh
ELEVENLABS_API_KEY=... npm run audio
```

The script writes `bgm.mp3` (60 s), `page-turn.mp3`, `tap.mp3`,
`whoosh.mp3`, `typing.mp3` and `chime.mp3` into `public/audio/` and skips
files that already exist. Pass `--force` to replace them, or name files to
make only those (`npm run audio -- --force tap.mp3`). The key is read from the
environment and never printed.

## Regenerate the screenshots

The screenshots come from the mock data, never from a real library:

1. In `web/`, run `npm run dev:mock`.
2. Use a 1440x900 viewport at 2x device scale, dark theme. Phone shots use a
   390x844 viewport at 3x with touch emulation.
3. To pick the language, set `localStorage['mars.locale']` to `en` or `ja`
   **and** override `navigator.language`/`navigator.languages` before the app
   loads (an init script). The app falls back to the browser language, so
   the storage key alone is not enough.
4. Save JPEGs as `public/shots/{en,ja}-<name>.jpg`, where `<name>` is `home`,
   `library`, `series`, `reader`, `reader-57` (the reader on the next page),
   `m-home` and `m-reader`.

## Render

```sh
npm run render        # both videos
npm run render:en     # out/mars-promo-en.mp4
npm run render:ja     # out/mars-promo-ja.mp4
npm run studio        # preview in the Remotion studio
```

Remotion downloads a headless Chromium the first time it renders.

Remotion ships its headless Chromium and ffmpeg for x64 only. On a Windows
arm64 machine, run the x64 binaries under emulation: pass
`--browser-executable` (an installed Chrome) and `--binaries-directory` (the
extracted `@remotion/compositor-win32-x64-msvc` package, from
`npm pack @remotion/compositor-win32-x64-msvc@4.0.543`) to `remotion render`:

```sh
npx remotion render src/index.ts promo-ja out/mars-promo-ja.mp4 \
  --browser-executable="C:\Program Files\Google\Chrome\Application\chrome.exe" \
  --binaries-directory=<extracted>/package
```
