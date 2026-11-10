import { AnimatePresence, motion } from "motion/react";
import type { ReadingDirection } from "@/api";
import { spring } from "@/lib/springs";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";

export interface ReaderPrefs {
  direction: ReadingDirection;
  spread: "off" | "on" | "cover";
  fit: "screen" | "width" | "height" | "original";
  bg: "black" | "gray" | "white";
}

const KEY = (seriesId: string) => `cn-reader-${seriesId}`;

/** Per-series reader preferences (a manga and a webtoon need different ones). */
export function loadPrefs(seriesId: string, direction: ReadingDirection): ReaderPrefs {
  const base: ReaderPrefs = { direction, spread: "off", fit: "screen", bg: "black" };
  try {
    return { ...base, ...JSON.parse(localStorage.getItem(KEY(seriesId)) ?? "{}") };
  } catch {
    return base;
  }
}
export function savePrefs(seriesId: string, p: ReaderPrefs) {
  localStorage.setItem(KEY(seriesId), JSON.stringify(p));
}

function Segmented<T extends string>({ label, value, options, onChange }: { label: string; value: T; options: [T, string][]; onChange: (v: T) => void }) {
  return (
    <fieldset className="space-y-2">
      <legend className={cn(typeClass("caption"), "text-white/60")}>{label}</legend>
      <div className="grid gap-1 rounded-lg bg-white/8 p-1" style={{ gridTemplateColumns: `repeat(${options.length}, minmax(0,1fr))` }} role="radiogroup">
        {options.map(([v, text]) => (
          <button
            key={v}
            type="button"
            role="radio"
            aria-checked={value === v}
            onClick={() => onChange(v)}
            className={cn(
              typeClass("caption"),
              "h-8 rounded-md px-2 transition-colors duration-80 outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)]",
              value === v ? "bg-white text-neutral-900" : "text-white/80 hover:bg-white/10",
            )}
            style={{ fontVariationSettings: value === v ? fontWeights.semibold : fontWeights.normal }}
          >
            {text}
          </button>
        ))}
      </div>
    </fieldset>
  );
}

export function ReaderSettings({ open, prefs, onChange, onClose }: { open: boolean; prefs: ReaderPrefs; onChange: (p: ReaderPrefs) => void; onClose: () => void }) {
  const set = <K extends keyof ReaderPrefs>(k: K) => (v: ReaderPrefs[K]) => onChange({ ...prefs, [k]: v });
  const paged = prefs.direction !== "webtoon";
  return (
    <AnimatePresence>
      {open && (
        <motion.aside
          key="settings"
          role="dialog"
          aria-label="表示設定"
          initial={{ x: "100%" }}
          animate={{ x: 0 }}
          exit={{ x: "100%", transition: spring.slow.exit }}
          transition={spring.slow}
          className="absolute inset-y-0 right-0 z-30 flex w-full max-w-xs flex-col gap-5 overflow-y-auto bg-neutral-900/95 p-5 pt-[max(env(safe-area-inset-top),20px)] text-white shadow-2xl backdrop-blur"
          onPointerDown={(e) => e.stopPropagation()}
        >
          <div className="flex items-center justify-between">
            <p className={typeClass("title")} style={{ fontVariationSettings: fontWeights.semibold }}>
              表示設定
            </p>
            <button type="button" onClick={onClose} className={cn(typeClass("caption"), "rounded px-2 py-1 text-white/70 hover:text-white")}>
              閉じる
            </button>
          </div>
          <Segmented
            label="読み方向"
            value={prefs.direction}
            onChange={set("direction")}
            options={[
              ["rtl", "右→左"],
              ["ltr", "左→右"],
              ["vertical", "縦"],
              ["webtoon", "縦スクロール"],
            ]}
          />
          {paged && (
            <Segmented
              label="見開き"
              value={prefs.spread}
              onChange={set("spread")}
              options={[
                ["off", "単ページ"],
                ["on", "見開き"],
                ["cover", "表紙だけ単"],
              ]}
            />
          )}
          {paged && (
            <Segmented
              label="表示サイズ"
              value={prefs.fit}
              onChange={set("fit")}
              options={[
                ["screen", "画面"],
                ["width", "幅"],
                ["height", "高さ"],
                ["original", "原寸"],
              ]}
            />
          )}
          <Segmented
            label="背景"
            value={prefs.bg}
            onChange={set("bg")}
            options={[
              ["black", "黒"],
              ["gray", "グレー"],
              ["white", "白"],
            ]}
          />
          <div className={cn(typeClass("caption"), "mt-auto space-y-1 text-white/60")}>
            <p>キー操作: ← → ページ送り / Space 次へ / M メニュー / Esc 閉じる</p>
            <p>画面の左右をタップでページ送り、中央でメニュー。</p>
          </div>
        </motion.aside>
      )}
    </AnimatePresence>
  );
}
