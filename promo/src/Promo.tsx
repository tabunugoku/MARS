import type { CSSProperties, FC, ReactNode } from "react";
import {
  AbsoluteFill,
  Audio,
  Img,
  Sequence,
  Series,
  interpolate,
  spring,
  staticFile,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import { captions as captionsEn } from "./captions.en";
import { captions as captionsJa } from "./captions.ja";

export type Lang = "en" | "ja";
type CapKey =
  | "intro"
  | "tagline"
  | "folder"
  | "library"
  | "reader"
  | "webtoon"
  | "phone"
  | "apps"
  | "light"
  | "cta";

const CAPS: Record<Lang, Record<CapKey, string>> = { en: captionsEn, ja: captionsJa };

// The app's own stack (web/src/index.css), with Inter first for Latin text.
const FONT =
  '"Inter", "Inter Variable", "Hiragino Sans", "Hiragino Kaku Gothic ProN", "Yu Gothic UI", Meiryo, "Noto Sans JP", system-ui, sans-serif';
const MONO = 'ui-monospace, "SF Mono", "Cascadia Mono", "JetBrains Mono", Consolas, monospace';

const INK = "#f2eeec";
const MUTED = "#aba29e";
const CARD = "#221a17";
const BRAND = "#E8542F";

// Scene table in frames (30 fps). Sums to 1650 (55 s).
const SCENES = {
  intro: 120,
  folder: 180,
  library: 210,
  reader: 300,
  webtoon: 180,
  phone: 180,
  apps: 180,
  light: 150,
  cta: 150,
} as const;

const WIN_W = 1280;
const WIN_H = 800;
const WIN_TOP = 40;

const spr = (frame: number, fps: number, delay = 0) =>
  spring({ frame: frame - delay, fps, config: { damping: 200 } });

const shot = (lang: Lang, name: string) => staticFile(`shots/${lang}-${name}.jpg`);
const phoneShot = (lang: Lang, name: string) => staticFile(`shots/${lang}-mobile-${name}.jpg`);

const fill: CSSProperties = {
  position: "absolute",
  left: 0,
  top: 0,
  width: "100%",
  height: "100%",
  objectFit: "cover",
};

const Win: FC<{ children: ReactNode }> = ({ children }) => (
  <div
    style={{
      position: "absolute",
      left: (1920 - WIN_W) / 2,
      top: WIN_TOP,
      width: WIN_W,
      height: WIN_H,
      borderRadius: 16,
      border: "1px solid rgba(255,255,255,0.08)",
      background: CARD,
      overflow: "hidden",
    }}
  >
    {children}
  </div>
);

const Subtitle: FC<{ lang: Lang; text: string; duration: number }> = ({ lang, text, duration }) => {
  const f = useCurrentFrame();
  const opacity = interpolate(f, [0, 12, duration - 12, duration], [0, 1, 1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  return (
    <div
      style={{
        position: "absolute",
        left: 0,
        right: 0,
        bottom: 40,
        display: "flex",
        justifyContent: "center",
        opacity,
      }}
    >
      <div
        style={{
          maxWidth: 1500,
          background: "rgba(18,13,11,0.78)",
          borderRadius: 12,
          padding: "12px 24px",
          color: INK,
          fontFamily: FONT,
          fontSize: lang === "ja" ? 44 : 40,
          fontWeight: 500,
          lineHeight: 1.4,
          textAlign: "center",
        }}
      >
        {text}
      </div>
    </div>
  );
};

// 0-120: logo mark, wordmark, intro and tagline lines.
const IntroScene: FC<{ lang: Lang }> = ({ lang }) => {
  const f = useCurrentFrame();
  const { fps } = useVideoConfig();
  const logo = spr(f, fps);
  const word = spr(f, fps, 10);
  const intro = spr(f, fps, 30);
  const tag = spr(f, fps, 60);
  const line = (p: number): CSSProperties => ({
    position: "absolute",
    left: 0,
    right: 0,
    textAlign: "center",
    fontFamily: FONT,
    opacity: p,
    transform: `translateY(${(1 - p) * 12}px)`,
  });
  return (
    <AbsoluteFill>
      <Sequence from={15} durationInFrames={90}>
        <Audio src={staticFile("audio/chime.mp3")} volume={0.6} />
      </Sequence>
      <Img
        src={staticFile("favicon.svg")}
        style={{
          position: "absolute",
          left: 860,
          top: 280,
          width: 200,
          height: 200,
          opacity: logo,
          transform: `scale(${0.7 + 0.3 * logo})`,
        }}
      />
      <div style={{ ...line(word), top: 540, color: INK, fontSize: 120, fontWeight: 600, lineHeight: 1 }}>
        MARS
      </div>
      <div style={{ ...line(intro), top: 690, color: MUTED, fontSize: 44, fontWeight: 500 }}>
        {CAPS[lang].intro}
      </div>
      <div style={{ ...line(tag), top: 760, color: MUTED, fontSize: 40, fontWeight: 500 }}>
        {CAPS[lang].tagline}
      </div>
    </AbsoluteFill>
  );
};

// 120-300: picker window with a slow push-in.
const FolderScene: FC<{ lang: Lang }> = ({ lang }) => {
  const f = useCurrentFrame();
  const scale = interpolate(f, [0, SCENES.folder], [1, 1.05], { extrapolateRight: "clamp" });
  return (
    <AbsoluteFill>
      <Win>
        <Img src={shot(lang, "picker1")} style={{ ...fill, transform: `scale(${scale})` }} />
      </Win>
      <Sequence from={60}>
        <Audio src={staticFile("audio/tap.mp3")} volume={0.6} />
      </Sequence>
      <Subtitle lang={lang} text={CAPS[lang].folder} duration={SCENES.folder} />
    </AbsoluteFill>
  );
};

// 300-510: library, then a dissolve to home.
const LibraryScene: FC<{ lang: Lang }> = ({ lang }) => {
  const f = useCurrentFrame();
  const { fps } = useVideoConfig();
  const p = spr(f, fps, 100);
  return (
    <AbsoluteFill>
      <Win>
        <Img src={shot(lang, "library")} style={{ ...fill, opacity: 1 - p }} />
        <Img
          src={shot(lang, "home")}
          style={{ ...fill, opacity: p, transform: `translateX(${(1 - p) * 40}px)` }}
        />
      </Win>
      <Sequence from={100}>
        <Audio src={staticFile("audio/whoosh.mp3")} volume={0.6} />
      </Sequence>
      <Subtitle lang={lang} text={CAPS[lang].library} duration={SCENES.library} />
    </AbsoluteFill>
  );
};

// 510-810: right-to-left page turns. The new page enters from the left and the
// old one leaves to the right, each change at local frame 90 and 180.
const READER_STARTS = [0, 90, 180] as const;
const ReaderScene: FC<{ lang: Lang }> = ({ lang }) => {
  const f = useCurrentFrame();
  const { fps } = useVideoConfig();
  const pages = [shot(lang, "reader"), shot(lang, "reader-57"), shot(lang, "reader-58")];
  let idx = 0;
  READER_STARTS.forEach((s, i) => {
    if (f >= s) idx = i;
  });
  const sinceTurn = f - READER_STARTS[idx];
  const p = idx === 0 ? 1 : spr(f, fps, READER_STARTS[idx]);
  const layers: ReactNode[] = [];
  if (idx > 0 && sinceTurn < 40) {
    layers.push(
      <Img
        key={`prev-${idx}`}
        src={pages[idx - 1]}
        style={{ ...fill, transform: `translateX(${p * WIN_W}px)` }}
      />,
    );
  }
  layers.push(
    <Img
      key={`cur-${idx}`}
      src={pages[idx]}
      style={{ ...fill, transform: `translateX(${(p - 1) * WIN_W}px)` }}
    />,
  );
  return (
    <AbsoluteFill>
      <Win>{layers}</Win>
      {READER_STARTS.slice(1).map((s) => (
        <Sequence key={s} from={s}>
          <Audio src={staticFile("audio/page-turn.mp3")} volume={0.6} />
        </Sequence>
      ))}
      <Subtitle lang={lang} text={CAPS[lang].reader} duration={SCENES.reader} />
    </AbsoluteFill>
  );
};

// 810-990: the central column of the webtoon page, panned upward.
const WebtoonScene: FC<{ lang: Lang }> = ({ lang }) => {
  const f = useCurrentFrame();
  const top = interpolate(f, [0, SCENES.webtoon], [0, -640], { extrapolateRight: "clamp" });
  return (
    <AbsoluteFill>
      <Win>
        <Img
          src={shot(lang, "webtoon")}
          style={{ position: "absolute", left: -504, top, width: 2304, height: 1440 }}
        />
      </Win>
      <Sequence from={0}>
        <Audio src={staticFile("audio/scroll.mp3")} volume={0.6} />
      </Sequence>
      <Subtitle lang={lang} text={CAPS[lang].webtoon} duration={SCENES.webtoon} />
    </AbsoluteFill>
  );
};

const PHONE_W = 380;
const PHONE_H = 780;
const PHONE_GAP = 48;
const PhoneFrame: FC<{ src: string; x: number; p: number }> = ({ src, x, p }) => (
  <div
    style={{
      position: "absolute",
      left: x,
      top: 40,
      width: PHONE_W,
      height: PHONE_H,
      borderRadius: 44,
      background: "#0b0807",
      border: "1px solid rgba(255,255,255,0.08)",
      padding: 12,
      boxSizing: "border-box",
      opacity: p,
      transform: `translateY(${(1 - p) * 120}px)`,
    }}
  >
    <div style={{ position: "relative", width: "100%", height: "100%", borderRadius: 34, overflow: "hidden" }}>
      <Img src={src} style={fill} />
    </div>
  </div>
);

// 990-1170: three phones enter one after another.
const PhoneScene: FC<{ lang: Lang }> = ({ lang }) => {
  const f = useCurrentFrame();
  const { fps } = useVideoConfig();
  const total = PHONE_W * 3 + PHONE_GAP * 2;
  const x0 = (1920 - total) / 2;
  const shots = ["home", "series", "reader"];
  return (
    <AbsoluteFill>
      {shots.map((name, i) => (
        <PhoneFrame
          key={name}
          src={phoneShot(lang, name)}
          x={x0 + i * (PHONE_W + PHONE_GAP)}
          p={spr(f, fps, i * 18)}
        />
      ))}
      <Subtitle lang={lang} text={CAPS[lang].phone} duration={SCENES.phone} />
    </AbsoluteFill>
  );
};

// 1170-1350: the Integrations section of the server settings, cropped and pushed in.
const AppsScene: FC<{ lang: Lang }> = ({ lang }) => {
  const f = useCurrentFrame();
  const scale = interpolate(f, [0, SCENES.apps], [1, 1.04], { extrapolateRight: "clamp" });
  // Source crop: the screenshot at 0.95 scale, window origin at (-532, -855).
  return (
    <AbsoluteFill>
      <Win>
        <Img
          src={shot(lang, "integrations")}
          style={{
            position: "absolute",
            left: -532,
            top: -855,
            width: 2736,
            height: 1710,
            transform: `scale(${scale})`,
            transformOrigin: "1172px 1255px",
          }}
        />
      </Win>
      <Subtitle lang={lang} text={CAPS[lang].apps} duration={SCENES.apps} />
    </AbsoluteFill>
  );
};

// 1350-1500: three flat labels, then the light caption as the subtitle.
const LABELS = ["No Java", "1 binary", "amd64 · arm64"] as const;
const LightScene: FC<{ lang: Lang }> = ({ lang }) => {
  const f = useCurrentFrame();
  const { fps } = useVideoConfig();
  return (
    <AbsoluteFill>
      {LABELS.map((label, i) => {
        const p = spr(f, fps, i * 14);
        return (
          <div
            key={label}
            style={{
              position: "absolute",
              left: 0,
              right: 0,
              top: 290 + i * 150,
              textAlign: "center",
              fontFamily: FONT,
              fontSize: 110,
              fontWeight: 600,
              lineHeight: 1.1,
              color: i === 2 ? INK : MUTED,
              opacity: p,
              transform: `translateY(${(1 - p) * 24}px)`,
            }}
          >
            {label}
          </div>
        );
      })}
      <Subtitle lang={lang} text={CAPS[lang].light} duration={SCENES.light} />
    </AbsoluteFill>
  );
};

// 1500-1650: logo, name, docker command, URL. Everything fades over the last 0.7 s.
const CtaScene: FC<{ lang: Lang }> = ({ lang }) => {
  const f = useCurrentFrame();
  const { fps } = useVideoConfig();
  const fadeOut = interpolate(f, [SCENES.cta - 21, SCENES.cta], [1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });
  const p = spr(f, fps);
  const pBox = spr(f, fps, 12);
  const pUrl = spr(f, fps, 24);
  return (
    <AbsoluteFill style={{ opacity: fadeOut }}>
      <Img
        src={staticFile("favicon.svg")}
        style={{
          position: "absolute",
          left: 885,
          top: 170,
          width: 150,
          height: 150,
          opacity: p,
          transform: `scale(${0.8 + 0.2 * p})`,
        }}
      />
      <div
        style={{
          position: "absolute",
          left: 0,
          right: 0,
          top: 340,
          textAlign: "center",
          fontFamily: FONT,
          fontSize: 110,
          fontWeight: 600,
          lineHeight: 1,
          color: INK,
          opacity: p,
        }}
      >
        MARS
      </div>
      <div
        style={{
          position: "absolute",
          left: 0,
          right: 0,
          top: 520,
          display: "flex",
          justifyContent: "center",
          opacity: pBox,
          transform: `translateY(${(1 - pBox) * 16}px)`,
        }}
      >
        <div
          style={{
            background: CARD,
            border: "1px solid rgba(255,255,255,0.08)",
            borderRadius: 12,
            padding: "20px 36px",
            color: INK,
            fontFamily: MONO,
            fontSize: 40,
            whiteSpace: "nowrap",
          }}
        >
          docker run -p 25600:25600 ghcr.io/tabunugoku/mars:1
        </div>
      </div>
      <div
        style={{
          position: "absolute",
          left: 0,
          right: 0,
          top: 700,
          textAlign: "center",
          fontFamily: MONO,
          fontSize: 44,
          color: MUTED,
          opacity: pUrl,
        }}
      >
        tabunugoku.github.io/MARS
      </div>
      <Subtitle lang={lang} text={CAPS[lang].cta} duration={SCENES.cta} />
    </AbsoluteFill>
  );
};

export const Promo: FC<{ lang: Lang }> = ({ lang }) => {
  const bgmVolume = (f: number) =>
    interpolate(f, [0, 30], [0, 0.55], { extrapolateLeft: "clamp", extrapolateRight: "clamp" }) *
    interpolate(f, [1605, 1650], [1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });

  return (
    <AbsoluteFill
      style={{
        background: "#120d0b",
        backgroundImage: "radial-gradient(rgba(255,255,255,0.07) 1px, transparent 1.4px)",
        backgroundSize: "12px 12px",
        color: INK,
      }}
    >
      <Series>
        <Series.Sequence durationInFrames={SCENES.intro}>
          <IntroScene lang={lang} />
        </Series.Sequence>
        <Series.Sequence durationInFrames={SCENES.folder}>
          <FolderScene lang={lang} />
        </Series.Sequence>
        <Series.Sequence durationInFrames={SCENES.library}>
          <LibraryScene lang={lang} />
        </Series.Sequence>
        <Series.Sequence durationInFrames={SCENES.reader}>
          <ReaderScene lang={lang} />
        </Series.Sequence>
        <Series.Sequence durationInFrames={SCENES.webtoon}>
          <WebtoonScene lang={lang} />
        </Series.Sequence>
        <Series.Sequence durationInFrames={SCENES.phone}>
          <PhoneScene lang={lang} />
        </Series.Sequence>
        <Series.Sequence durationInFrames={SCENES.apps}>
          <AppsScene lang={lang} />
        </Series.Sequence>
        <Series.Sequence durationInFrames={SCENES.light}>
          <LightScene lang={lang} />
        </Series.Sequence>
        <Series.Sequence durationInFrames={SCENES.cta}>
          <CtaScene lang={lang} />
        </Series.Sequence>
      </Series>
      <Audio src={staticFile("audio/bgm.mp3")} volume={bgmVolume} />
    </AbsoluteFill>
  );
};
