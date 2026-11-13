# MARS promo video

A 55-second promo for MARS, built with [Remotion](https://www.remotion.dev).
It renders two versions from one component: `promo-en` and `promo-ja`
(1920x1080, 30 fps, 1650 frames). The Japanese version is the original; the
English captions are translations of it.

## Files

- `src/Promo.tsx` — the scenes, subtitles and audio cues
- `src/Root.tsx` — the two compositions (`promo-en`, `promo-ja`)
- `src/captions.ja.ts`, `src/captions.en.ts` — subtitle text (keep the keys in sync)
- `public/shots/` — app screenshots (gitignored)
- `public/audio/` — background music and sound effects (gitignored)
- `out/` — rendered MP4s (gitignored)

## Regenerate the audio

The music and sound effects come from the ElevenLabs API:

```sh
ELEVENLABS_API_KEY=... npm run audio
```

The script writes into `public/audio/` and skips files that already exist.
Pass `--force` to replace them. The key is read from the environment and never printed.

## Regenerate the screenshots

The screenshots are captured from the mock data, not from a real library:

1. In `web/`, run `npm run dev:mock`.
2. Use a 1440x900 viewport at 2x device scale, with the dark theme.
3. Capture each screen in English and in Japanese, and save them as JPEG as
   `public/shots/{en,ja}-<name>.jpg`. The phone shots are WebP as
   `public/shots/mobile-<name>.webp`.

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
extracted `@remotion/compositor-win32-x64-msvc` package) to `remotion render`.
