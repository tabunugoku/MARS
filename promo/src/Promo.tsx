// MARS promo: 58 s, 1920x1080, captions only (no narration).
// Ten scenes, each a Sequence that overlaps the next by OVERLAP frames.
// The look is the app's: a night chart in screentone dots, flat surfaces,
// oxide red for read progress and actions, blue only for "you are here".
// Every motion is a spring from motion.ts; nothing is timed by hand.
import type { CSSProperties, FC, ReactNode } from "react";
import { AbsoluteFill, Audio, Img, Sequence, continueRender, delayRender, interpolate, staticFile, useCurrentFrame } from "remotion";
import { useState } from "react";
import { COMMAND, REPO, TAGLINE, type Captions, type Lang } from "./captions";
import { en } from "./captions.en";
import { ja } from "./captions.ja";
import { lerp, sp } from "./motion";

// ---------- Look (the app's dark tokens) ----------
const C = {
  bg: "oklch(0.14 0.0084 40)",
  fg: "oklch(0.95 0.0056 40)",
  muted: "oklch(0.7 0.0154 40)",
  surface: "oklch(0.18 0.0112 40)",
  card: "oklch(0.215 0.014 40)",
  tint: "oklch(0.305 0.0154 40)",
  selected: "oklch(0.34 0.014 40)",
  line: "rgba(255,255,255,0.09)",
  dot: "rgba(255,255,255,0.07)",
  brand: "oklch(0.64 0.19 35)",
  brandFill: "oklch(0.55 0.19 33)",
  here: "oklch(0.74 0.13 245)",
  paper: "oklch(0.97 0.006 60)",
  ink: "oklch(0.25 0.01 40)",
};
const SANS = (lang: Lang) =>
  lang === "ja" ? '"Geist", "Noto Sans JP", "Yu Gothic UI", sans-serif' : '"Geist", "Noto Sans JP", sans-serif';
const MONO = '"Cascadia Mono", Consolas, ui-monospace, monospace';
const SHADOW = "0 0 0 1px rgba(255,255,255,0.05), 0 40px 80px -40px rgba(0,0,0,0.85)";

// ---------- Timeline (frames at 30 fps) ----------
const OVERLAP = 8;
const SCENES = [
  ["title", 0, 105],
  ["folders", 105, 195],
  ["series", 300, 180],
  ["reader", 480, 210],
  ["direction", 690, 165],
  ["resume", 855, 165],
  ["phone", 1020, 180],
  ["apps", 1200, 165],
  ["docker", 1365, 180],
  ["end", 1545, 195],
] as const;
export const DURATION = 1740;

// ---------- Shared pieces ----------
const Ground: FC = () => (
  <AbsoluteFill
    style={{
      background: C.bg,
      backgroundImage: `radial-gradient(${C.dot} 1.6px, transparent 2.2px)`,
      backgroundSize: "20px 20px",
    }}
  />
);

/** Fades a whole scene out on a soft spring over its last frames. */
const Scene: FC<{ dur: number; children: ReactNode }> = ({ dur, children }) => {
  const f = useCurrentFrame();
  const out = sp(f, "soft", dur - OVERLAP - 6);
  return <AbsoluteFill style={{ opacity: 1 - out }}>{children}</AbsoluteFill>;
};

const Caption: FC<{ text: string; lang: Lang }> = ({ text, lang }) => {
  const f = useCurrentFrame();
  const p = sp(f, "soft", 10);
  return (
    <div
      lang={lang}
      style={{
        position: "absolute",
        left: 0,
        right: 0,
        bottom: 64,
        textAlign: "center",
        fontFamily: SANS(lang),
        fontSize: 48,
        lineHeight: "64px",
        fontWeight: 600,
        color: C.fg,
        opacity: p,
        transform: `translateY(${lerp(14, 0, p)}px)`,
      }}
    >
      {text}
    </div>
  );
};

const Mark: FC<{ size: number }> = ({ size }) => {
  // The app's logo (web/src/components/brand/app-mark.tsx), same geometry.
  const RX = 13, RY = 5, ROT = (-20 * Math.PI) / 180, N = 40, PR = 7;
  const on = (deg: number) => {
    const t = (deg * Math.PI) / 180;
    const x = RX * Math.cos(t), y = RY * Math.sin(t);
    return { x: 16 + x * Math.cos(ROT) - y * Math.sin(ROT), y: 16 + x * Math.sin(ROT) + y * Math.cos(ROT), front: Math.sin(t) > 0 };
  };
  const near = on(40), far = on(200);
  const dots = Array.from({ length: N }, (_, i) => on((360 * i) / N)).filter(
    (d) => !(!d.front && Math.hypot(d.x - 16, d.y - 16) < PR + 0.9) && ![near, far].some((m) => Math.hypot(d.x - m.x, d.y - m.y) < 2.6),
  );
  return (
    <svg width={size} height={size} viewBox="0 0 32 32">
      <rect width="32" height="32" rx="8" fill="#1f1210" />
      {dots.filter((d) => !d.front).map((d, i) => <circle key={`b${i}`} cx={d.x} cy={d.y} r={0.5} fill="#f3c9bd" fillOpacity={0.5} />)}
      <circle cx="16" cy="16" r={PR} fill="#e8542f" />
      {dots.filter((d) => d.front).map((d, i) => <circle key={`f${i}`} cx={d.x} cy={d.y} r={0.5} fill="#f3c9bd" fillOpacity={0.85} />)}
      <circle cx={far.x} cy={far.y} r="1.2" fill="#f3c9bd" />
      <circle cx={near.x} cy={near.y} r="2.1" fill="#5db2f7" />
    </svg>
  );
};

/** A browser window around a 1440x900 screenshot. */
const Win: FC<{ w: number; style?: CSSProperties; children: ReactNode }> = ({ w, style, children }) => (
  <div style={{ position: "absolute", width: w, borderRadius: 16, overflow: "hidden", background: C.card, boxShadow: SHADOW, ...style }}>
    <div style={{ height: 44, display: "flex", alignItems: "center", padding: "0 16px", background: C.surface, borderBottom: `1px solid ${C.line}`, position: "relative" }}>
      <div style={{ display: "flex", gap: 8 }}>
        {[0, 1, 2].map((i) => <div key={i} style={{ width: 11, height: 11, borderRadius: 6, background: C.tint }} />)}
      </div>
      <div style={{ position: "absolute", left: "50%", transform: "translateX(-50%)", padding: "3px 16px", borderRadius: 8, background: C.bg, fontFamily: MONO, fontSize: 18, color: C.muted }}>
        localhost:25600
      </div>
    </div>
    <div style={{ position: "relative", width: w, height: (w * 900) / 1440, overflow: "hidden" }}>{children}</div>
  </div>
);

const Shot: FC<{ src: string; style?: CSSProperties }> = ({ src, style }) => (
  <Img src={staticFile(`shots/${src}`)} style={{ position: "absolute", inset: 0, width: "100%", height: "100%", ...style }} />
);

const Icon: FC<{ d: string; size?: number; color?: string }> = ({ d, size = 28, color = "currentColor" }) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
    <path d={d} />
  </svg>
);
const I = {
  folder: "M3 7C3 5.3431 4.3431 4 6 4L8.6716 4C9.202 4 9.7107 4.2107 10.0858 4.5858L11.4142 5.9142C11.7893 6.2893 12.298 6.5 12.8284 6.5L18 6.5C19.6569 6.5 21 7.8431 21 9.5L21 17C21 18.6569 19.6569 20 18 20L6 20C4.3431 20 3 18.6569 3 17Z",
  file: "M14 2H8C5.79086 2 4 3.79086 4 6V18C4 20.2091 5.79086 22 8 22H16C18.2091 22 20 20.2091 20 18V8L14 2ZM14 2V5C14 6.65685 15.3431 8 17 8H20",
  fileText: "M14 2H8C5.79086 2 4 3.79086 4 6V18C4 20.2091 5.79086 22 8 22H16C18.2091 22 20 20.2091 20 18V8L14 2ZM14 2V5C14 6.65685 15.3431 8 17 8H20M8 13H12M8 17H16",
  down: "M6 9L12 15L18 9",
  right: "M9 6L15 12L9 18",
  arrowLeft: "M19 12H5.94129M11.8771 5L5.17385 11.5875C4.94205 11.8153 4.94205 12.1847 5.17385 12.4125L11.8771 19",
  arrowRight: "M5 12H18.0587M12.1229 5L18.8261 11.5875C19.058 11.8153 19.058 12.1847 18.8261 12.4125L12.1229 19",
};

// ---------- Scenes ----------
const Title: FC<{ t: Captions; lang: Lang }> = ({ t, lang }) => {
  const f = useCurrentFrame();
  const m = sp(f, "window", 4);
  const w = sp(f, "soft", 14);
  const s = sp(f, "soft", 24);
  return (
    <AbsoluteFill style={{ alignItems: "center", justifyContent: "center", fontFamily: SANS(lang) }}>
      <div style={{ opacity: m, transform: `scale(${lerp(0.9, 1, m)})` }}>
        <Mark size={168} />
      </div>
      <div style={{ marginTop: 36, fontSize: 132, lineHeight: "132px", fontWeight: 600, color: C.fg, opacity: w, transform: `translateY(${lerp(18, 0, w)}px)` }}>MARS</div>
      <div style={{ marginTop: 20, fontSize: 40, color: C.muted, opacity: s, transform: `translateY(${lerp(12, 0, s)}px)` }}>{t.ui.subtitle}</div>
    </AbsoluteFill>
  );
};

const SERIES = ["ことのは図書館", "千年喫茶ほたる", "遠雷マーチ", "ネオン金魚", "砂時計の騎士"];

const Folders: FC<{ t: Captions; lang: Lang; dur: number }> = ({ t, lang, dur }) => {
  const f = useCurrentFrame();
  const rows: { kind: "dir" | "file" | "xml"; name: string; open?: boolean; series?: number }[] = [
    { kind: "dir", name: SERIES[0], open: true, series: 0 },
    { kind: "file", name: "vol01.cbz" },
    { kind: "file", name: "vol02.cbz" },
    { kind: "file", name: "vol03.cbz" },
    { kind: "xml", name: "ComicInfo.xml" },
    ...SERIES.slice(1).map((n, i) => ({ kind: "dir" as const, name: n, series: i + 1 })),
  ];
  const win = sp(f, "window", 48);
  return (
    <Scene dur={dur}>
      <div style={{ position: "absolute", left: 140, top: 150, width: 600, borderRadius: 16, background: C.surface, border: `1px solid ${C.line}`, padding: "20px 0", fontFamily: SANS(lang) }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12, padding: "0 28px 16px", color: C.muted, fontFamily: MONO, fontSize: 24, borderBottom: `1px solid ${C.line}`, marginBottom: 8 }}>
          <Icon d={I.folder} size={24} /> /comics/manga
        </div>
        {rows.map((r, i) => {
          const p = sp(f, "crisp", 6 + i * 4);
          const read = r.series !== undefined ? sp(f, "crisp", 64 + r.series * 6) : 0;
          const indent = r.kind === "dir" ? 0 : 44;
          return (
            <div key={i} style={{ display: "flex", alignItems: "center", gap: 12, height: 56, padding: `0 28px 0 ${28 + indent}px`, opacity: p, transform: `translateY(${lerp(8, 0, p)}px)` }}>
              {r.kind === "dir" ? (
                <>
                  <Icon d={r.open ? I.down : I.right} size={20} color={C.muted} />
                  <Icon d={I.folder} size={26} color={C.fg} />
                </>
              ) : (
                <Icon d={r.kind === "xml" ? I.fileText : I.file} size={24} color={C.muted} />
              )}
              <span lang="ja" style={{ fontFamily: r.kind === "dir" ? SANS("ja") : MONO, fontSize: r.kind === "dir" ? 28 : 24, color: r.kind === "dir" ? C.fg : C.muted }}>{r.name}</span>
              {r.series !== undefined && (
                <div style={{ marginLeft: "auto", width: 12, height: 12, borderRadius: 6, background: C.brand, opacity: read, transform: `scale(${read})` }} />
              )}
            </div>
          );
        })}
      </div>
      <Win w={980} style={{ left: 800, top: 150, opacity: win, transform: `translateX(${lerp(90, 0, win)}px)` }}>
        <Shot src={`${lang}-library.jpg`} />
      </Win>
      <Caption text={t.folders} lang={lang} />
      <Sequence from={48}><Audio src={staticFile("audio/whoosh.mp3")} volume={0.35} /></Sequence>
    </Scene>
  );
};

const Series: FC<{ t: Captions; lang: Lang; dur: number }> = ({ t, lang, dur }) => {
  const f = useCurrentFrame();
  const enter = sp(f, "window", 0);
  const push = sp(f, "calm", 40);
  return (
    <Scene dur={dur}>
      <Win w={1240} style={{ left: 340, top: 70, opacity: enter, transform: `translateY(${lerp(40, 0, enter)}px)` }}>
        <Shot src={`${lang}-series.jpg`} style={{ transformOrigin: "62% 78%", transform: `scale(${lerp(1, 1.22, push)})` }} />
      </Win>
      <Caption text={t.series} lang={lang} />
    </Scene>
  );
};

// The page in the reader screenshots spans this share of the width.
const PAGE_L = 0.2915, PAGE_R = 0.7082;

const Reader: FC<{ t: Captions; lang: Lang; dur: number }> = ({ t, lang, dur }) => {
  const f = useCurrentFrame();
  const W = 1240, H = (W * 900) / 1440;
  const enter = sp(f, "window", 0);
  const TURN = 78;
  const key = sp(f, "crisp", TURN - 6) - sp(f, "crisp", TURN + 2);
  const p = f >= TURN ? sp(f, "page", TURN) : 0;
  const left = PAGE_L * W, width = (PAGE_R - PAGE_L) * W;
  return (
    <Scene dur={dur}>
      <Win w={W} style={{ left: 340, top: 70, opacity: enter, transform: `translateY(${lerp(40, 0, enter)}px)` }}>
        <Shot src={f >= TURN ? `${lang}-reader-57.jpg` : `${lang}-reader.jpg`} />
        {f >= TURN && (
          <div style={{ position: "absolute", left, top: 0, width, height: H, overflow: "hidden" }}>
            {/* Right-to-left: the page you read leaves to the right, the next one comes in from the left. */}
            <Img src={staticFile(`shots/${lang}-reader.jpg`)} style={{ position: "absolute", top: 0, left: -left, width: W, height: H, transform: `translateX(${lerp(0, width + 24, p)}px)` }} />
            <Img src={staticFile(`shots/${lang}-reader-57.jpg`)} style={{ position: "absolute", top: 0, left: -left, width: W, height: H, transform: `translateX(${lerp(-width - 24, 0, p)}px)` }} />
          </div>
        )}
      </Win>
      <div style={{ position: "absolute", left: 340 + W + 24, top: 70 + 44 + H - 72, width: 72, height: 72, borderRadius: 14, background: C.card, border: `1px solid ${C.line}`, borderBottomWidth: 4, display: "flex", alignItems: "center", justifyContent: "center", color: C.fg, opacity: enter, transform: `translateY(${key * 3}px) scale(${1 - key * 0.06})` }}>
        <Icon d={I.arrowLeft} size={32} />
      </div>
      <Caption text={t.reader} lang={lang} />
      <Sequence from={TURN}><Audio src={staticFile("audio/page-turn.mp3")} volume={0.8} /></Sequence>
    </Scene>
  );
};

const Page: FC<{ n: number }> = ({ n }) => (
  <div style={{ width: "100%", height: "100%", background: C.paper, borderRadius: 6, padding: 10, display: "grid", gridTemplateRows: n % 2 ? "1fr 1.4fr 1fr" : "1.3fr 1fr 1fr", gap: 8 }}>
    {[0, 1, 2].map((i) => (
      <div key={i} style={{ border: `2.5px solid ${C.ink}`, borderRadius: 2, display: "grid", gridTemplateColumns: (n + i) % 3 === 0 ? "1fr" : "1fr 1fr", gap: 8 }}>
        {(n + i) % 3 !== 0 && <div style={{ borderRight: `2.5px solid ${C.ink}` }} />}
      </div>
    ))}
  </div>
);

const Direction: FC<{ t: Captions; lang: Lang; dur: number }> = ({ t, lang, dur }) => {
  const f = useCurrentFrame();
  const enter = sp(f, "window", 0);
  const CHANGE = 72;
  const sel = sp(f, "crisp", CHANGE); // selection slides from 右→左 to 縦スクロール
  const morph = sp(f, "window", CHANGE + 4);
  const SEG = 220, PW = 132, PH = 186;
  // Row (right to left: page 1 on the right) → one continuous column.
  const pages = [0, 1, 2, 3, 4].map((i) => {
    const row = { x: 640 - (i - 2) * (PW + 24) - PW / 2, y: 40 };
    const col = { x: 470 - PW / 2 - 6, y: 40 + i * (PH + 4) - 30 };
    return { x: lerp(row.x, col.x, morph), y: lerp(row.y, col.y, morph), w: lerp(PW, PW + 12, morph) };
  });
  return (
    <Scene dur={dur}>
      <div style={{ position: "absolute", left: 460, top: 90, width: 1000, borderRadius: 20, background: C.card, boxShadow: SHADOW, padding: 40, fontFamily: SANS(lang), opacity: enter, transform: `translateY(${lerp(40, 0, enter)}px)` }}>
        <div lang={lang} style={{ fontSize: 30, fontWeight: 600, color: C.fg, marginBottom: 20 }}>{t.ui.readingDirection}</div>
        <div style={{ position: "relative", display: "flex", padding: 6, borderRadius: 14, background: C.surface, width: SEG * 4 + 12 }}>
          <div style={{ position: "absolute", top: 6, left: 6, width: SEG, height: 64, borderRadius: 10, background: C.selected, transform: `translateX(${lerp(0, SEG * 3, sel)}px)` }} />
          {t.ui.modes.map((m, i) => {
            const on = i === 0 ? 1 - sel : i === 3 ? sel : 0;
            return (
              <div key={m} lang={lang} style={{ position: "relative", width: SEG, height: 64, display: "flex", alignItems: "center", justifyContent: "center", fontSize: 26, fontWeight: 500, color: on > 0.5 ? C.fg : C.muted }}>{m}</div>
            );
          })}
        </div>
        <div style={{ position: "relative", height: 420, marginTop: 28, borderRadius: 14, background: C.bg, overflow: "hidden" }}>
          {pages.map((p, i) => (
            <div key={i} style={{ position: "absolute", left: p.x, top: p.y, width: p.w, height: PH }}>
              <Page n={i} />
            </div>
          ))}
          <div style={{ position: "absolute", left: 640 - 3 * (PW + 24) + 40, top: 260, color: C.muted, opacity: 1 - morph, display: "flex", alignItems: "center", gap: 10, fontFamily: MONO, fontSize: 22 }}>
            <Icon d={I.arrowLeft} size={30} />
          </div>
          <div style={{ position: "absolute", left: 640, top: 150, color: C.muted, opacity: morph, transform: "rotate(90deg)" }}>
            <Icon d={I.arrowRight} size={40} />
          </div>
        </div>
      </div>
      <Caption text={t.direction} lang={lang} />
      <Sequence from={CHANGE - 2}><Audio src={staticFile("audio/tap.mp3")} volume={0.6} /></Sequence>
    </Scene>
  );
};

const Cursor: FC<{ x: number; y: number; press: number; opacity: number }> = ({ x, y, press, opacity }) => (
  <svg width={34} height={40} viewBox="0 0 17 20" style={{ position: "absolute", left: x, top: y, opacity, transform: `scale(${1 - press * 0.12})`, transformOrigin: "2px 2px", filter: "drop-shadow(0 2px 3px rgba(0,0,0,0.5))" }}>
    <path d="M1.5 1.5 L1.5 15.5 L5.2 12 L7.8 18 L10.2 17 L7.7 11.1 L12.8 11.1 Z" fill="#fff" stroke="#111" strokeWidth={1.1} strokeLinejoin="round" />
  </svg>
);

const Resume: FC<{ t: Captions; lang: Lang; dur: number }> = ({ t, lang, dur }) => {
  const f = useCurrentFrame();
  const W = 1240, H = (W * 900) / 1440, s = W / 1440;
  const enter = sp(f, "window", 0);
  const zoom = sp(f, "calm", 18);
  const Z = lerp(1, 1.55, zoom);
  const ox = 0.42 * W, oy = 0.27 * H;
  // The "Resume" button in the 1440x900 screenshot.
  const btn = { x: 500 * s, y: 245 * s };
  const arrive = sp(f, "soft", 62);
  const CLICK = 104;
  const press = sp(f, "crisp", CLICK) - sp(f, "crisp", CLICK + 5);
  const cx = lerp(btn.x + 260, btn.x - 6, arrive), cy = lerp(btn.y + 170, btn.y - 4, arrive);
  return (
    <Scene dur={dur}>
      <Win w={W} style={{ left: 340, top: 70, opacity: enter, transform: `translateY(${lerp(40, 0, enter)}px)` }}>
        <div style={{ position: "absolute", inset: 0, transformOrigin: `${ox}px ${oy}px`, transform: `scale(${Z})` }}>
          <Shot src={`${lang}-home.jpg`} />
          <Cursor x={cx} y={cy} press={press} opacity={sp(f, "soft", 56)} />
        </div>
      </Win>
      <Caption text={t.resume} lang={lang} />
      <Sequence from={CLICK}><Audio src={staticFile("audio/tap.mp3")} volume={0.6} /></Sequence>
    </Scene>
  );
};

const Phone: FC<{ src: string; h: number; style?: CSSProperties }> = ({ src, h, style }) => {
  const w = (h * 1170) / 2532;
  return (
    <div style={{ position: "absolute", padding: 10, borderRadius: 56, background: "oklch(0.2 0.01 40)", border: `1px solid ${C.line}`, boxShadow: SHADOW, ...style }}>
      <Img src={staticFile(`shots/${src}`)} style={{ display: "block", width: w, height: h, borderRadius: 46 }} />
    </div>
  );
};

const PhoneScene: FC<{ t: Captions; lang: Lang; dur: number }> = ({ t, lang, dur }) => {
  const f = useCurrentFrame();
  const a = sp(f, "window", 0), b = sp(f, "window", 9);
  const H = 760, w = (H * 1170) / 2532 + 20;
  return (
    <Scene dur={dur}>
      <Phone src={`${lang}-m-home.jpg`} h={H} style={{ left: 960 - w - 40, top: 70, opacity: a, transform: `translateY(${lerp(80, 0, a)}px)` }} />
      <Phone src={`${lang}-m-reader.jpg`} h={H} style={{ left: 960 + 40, top: 70, opacity: b, transform: `translateY(${lerp(80, 0, b)}px)` }} />
      <Caption text={t.phone} lang={lang} />
      <Sequence from={2}><Audio src={staticFile("audio/whoosh.mp3")} volume={0.3} /></Sequence>
    </Scene>
  );
};

const Apps: FC<{ t: Captions; lang: Lang; dur: number }> = ({ t, lang, dur }) => {
  const f = useCurrentFrame();
  const hub = { x: 960, y: 300 };
  const cards = [
    { x: 500, title: "Mihon", note: t.ui.mihonNote },
    { x: 960, title: "OPDS 1.2", note: t.ui.opdsNote },
    { x: 1420, title: t.ui.browser, note: t.ui.browserNote },
  ];
  const m = sp(f, "window", 0);
  const CARD_Y = 600, DOTS = 9;
  return (
    <Scene dur={dur}>
      <div style={{ position: "absolute", left: hub.x - 70, top: hub.y - 70, opacity: m, transform: `scale(${lerp(0.9, 1, m)})` }}>
        <Mark size={140} />
      </div>
      {cards.map((c, i) => {
        const p = sp(f, "crisp", 14 + i * 6);
        // A dotted route from the server to each app; the dots turn red one
        // after another, like read progress in the app.
        const from = { x: hub.x, y: hub.y + 86 }, to = { x: c.x, y: CARD_Y - 16 };
        return (
          <div key={c.title}>
            {Array.from({ length: DOTS }, (_, k) => {
              const t2 = (k + 0.5) / DOTS;
              const lit = sp(f, "crisp", 34 + i * 6 + k * 3);
              return (
                <div key={k} style={{ position: "absolute", left: lerp(from.x, to.x, t2) - 6, top: lerp(from.y, to.y, t2) - 6, width: 12, height: 12, borderRadius: 6, background: lit > 0.5 ? C.brand : C.tint, opacity: p, transform: `scale(${lerp(0.7, 1, lit)})` }} />
              );
            })}
            <div style={{ position: "absolute", left: c.x - 200, top: CARD_Y, width: 400, padding: "28px 32px", borderRadius: 18, background: C.surface, border: `1px solid ${C.line}`, fontFamily: SANS(lang), opacity: p, transform: `translateY(${lerp(12, 0, p)}px)` }}>
              <div lang={lang} style={{ fontSize: 38, fontWeight: 600, color: C.fg }}>{c.title}</div>
              <div lang={lang} style={{ fontSize: 26, color: C.muted, marginTop: 6 }}>{c.note}</div>
            </div>
          </div>
        );
      })}
      <Caption text={t.apps} lang={lang} />
    </Scene>
  );
};

const Docker: FC<{ t: Captions; lang: Lang; dur: number }> = ({ t, lang, dur }) => {
  const f = useCurrentFrame();
  const enter = sp(f, "window", 0);
  const START = 18, PER_FRAME = 2.2;
  const n = Math.max(0, Math.min(COMMAND.length, Math.floor((f - START) * PER_FRAME)));
  const done = n >= COMMAND.length;
  const doneAt = START + Math.ceil(COMMAND.length / PER_FRAME);
  const pill = sp(f, "crisp", doneAt + 14);
  return (
    <Scene dur={dur}>
      <div style={{ position: "absolute", left: 260, top: 200, width: 1400, borderRadius: 16, overflow: "hidden", background: "oklch(0.11 0.006 40)", boxShadow: SHADOW, opacity: enter, transform: `translateY(${lerp(40, 0, enter)}px)` }}>
        <div style={{ height: 44, display: "flex", alignItems: "center", gap: 8, padding: "0 16px", background: C.surface, borderBottom: `1px solid ${C.line}` }}>
          {[0, 1, 2].map((i) => <div key={i} style={{ width: 11, height: 11, borderRadius: 6, background: C.tint }} />)}
        </div>
        <div style={{ padding: "36px 44px 44px", fontFamily: MONO, fontSize: 36, lineHeight: "54px", color: C.fg, minHeight: 260, wordBreak: "break-all" }}>
          <span style={{ color: C.brand }}>$ </span>
          {COMMAND.slice(0, n)}
          <span style={{ display: "inline-block", width: 18, height: 40, verticalAlign: "-7px", marginLeft: 4, background: done ? C.muted : C.fg }} />
        </div>
      </div>
      <div style={{ position: "absolute", left: 260, top: 640, display: "flex", alignItems: "center", gap: 18, fontFamily: SANS(lang), opacity: pill, transform: `translateY(${lerp(10, 0, pill)}px)` }}>
        <span lang={lang} style={{ fontSize: 30, color: C.muted }}>{t.ui.open}</span>
        <Icon d={I.arrowRight} size={30} color={C.muted} />
        <span style={{ fontFamily: MONO, fontSize: 30, color: C.fg, padding: "8px 20px", borderRadius: 10, background: C.surface, border: `1px solid ${C.line}` }}>http://localhost:25600</span>
      </div>
      <Caption text={t.docker} lang={lang} />
      <Sequence from={START} durationInFrames={doneAt - START + 2}>
        <Audio src={staticFile("audio/typing.mp3")} volume={(fr) => interpolate(fr, [0, doneAt - START - 6, doneAt - START + 2], [0.4, 0.4, 0], { extrapolateRight: "clamp" })} />
      </Sequence>
    </Scene>
  );
};

const End: FC<{ lang: Lang }> = ({ lang }) => {
  const f = useCurrentFrame();
  const m = sp(f, "window", 2);
  const repo = sp(f, "soft", 78);
  return (
    <AbsoluteFill style={{ alignItems: "center", justifyContent: "center", fontFamily: SANS(lang) }}>
      <div style={{ opacity: m, transform: `scale(${lerp(0.9, 1, m)})` }}>
        <Mark size={128} />
      </div>
      <div lang="en" style={{ marginTop: 48, fontSize: 84, lineHeight: "96px", fontWeight: 600, display: "flex", gap: 24 }}>
        {TAGLINE.map((w, i) => {
          const lit = sp(f, "soft", 18 + i * 7);
          const last = i === TAGLINE.length - 1;
          return (
            <span key={i} style={{ color: lit > 0.5 ? (last ? C.brand : C.fg) : C.muted, opacity: lerp(0.35, 1, lit) }}>{w}</span>
          );
        })}
      </div>
      <div style={{ marginTop: 44, fontFamily: MONO, fontSize: 34, color: C.muted, opacity: repo, transform: `translateY(${lerp(10, 0, repo)}px)` }}>{REPO}</div>
      <Sequence from={14}><Audio src={staticFile("audio/chime.mp3")} volume={0.55} /></Sequence>
    </AbsoluteFill>
  );
};

// ---------- Fonts ----------
function useGeist() {
  const [handle] = useState(() => delayRender("Loading Geist"));
  useState(() => {
    const face = new FontFace("Geist", `url(${staticFile("fonts/geist.woff2")}) format("woff2")`, { weight: "100 900" });
    face.load().then((loaded) => {
      (document.fonts as unknown as { add: (f: FontFace) => void }).add(loaded);
      return document.fonts.ready;
    }).then(() => continueRender(handle), () => continueRender(handle));
    return null;
  });
}

export const Promo: FC<{ lang: Lang }> = ({ lang }) => {
  useGeist();
  const t = lang === "ja" ? ja : en;
  const f = useCurrentFrame();
  const scene = (name: string, from: number, dur: number) => {
    const d = dur + OVERLAP;
    const props = { t, lang, dur: d };
    const body =
      name === "title" ? <Scene dur={d}><Title t={t} lang={lang} /></Scene>
      : name === "folders" ? <Folders {...props} />
      : name === "series" ? <Series {...props} />
      : name === "reader" ? <Reader {...props} />
      : name === "direction" ? <Direction {...props} />
      : name === "resume" ? <Resume {...props} />
      : name === "phone" ? <PhoneScene {...props} />
      : name === "apps" ? <Apps {...props} />
      : name === "docker" ? <Docker {...props} />
      : <End lang={lang} />;
    return <Sequence key={name} from={from} durationInFrames={name === "end" ? dur : d} name={name}>{body}</Sequence>;
  };
  // The music fades in over a second and out over the last 2.5 seconds.
  const music = interpolate(f, [0, 30, DURATION - 75, DURATION - 1], [0, 0.5, 0.5, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <AbsoluteFill style={{ background: C.bg }}>
      <Ground />
      {SCENES.map(([name, from, dur]) => scene(name, from, dur))}
      <Audio src={staticFile("audio/bgm.mp3")} volume={music} />
    </AbsoluteFill>
  );
};
