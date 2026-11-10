import { AnimatePresence, motion } from "motion/react";
import { Link } from "react-router";
import { BG_LABEL, DIRECTION_LABEL, FIT_LABEL, SPREAD_LABEL, type ReaderPrefs, type SeriesReaderPrefs } from "@/api";
import { spring } from "@/lib/springs";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";

const opts = <T extends string>(labels: Record<T, string>) => Object.entries(labels) as [T, string][];

function Segmented<T extends string>({ label, value, options, onChange, disabled }: { label: string; value: T; options: [T, string][]; onChange: (v: T) => void; disabled?: boolean }) {
  return (
    <fieldset className="space-y-2" disabled={disabled}>
      <legend className={cn(typeClass("caption"), "text-white/60")}>{label}</legend>
      <div
        className={cn("grid gap-1 rounded-lg bg-white/8 p-1", disabled && "opacity-50")}
        style={{ gridTemplateColumns: `repeat(${options.length}, minmax(max-content,1fr))` }}
        role="radiogroup"
      >
        {options.map(([v, text]) => (
          <button
            key={v}
            type="button"
            role="radio"
            aria-checked={value === v}
            onClick={() => onChange(v)}
            className={cn(
              typeClass("caption"),
              "inline-flex h-9 pointer-coarse:h-11 items-center justify-center whitespace-nowrap rounded-md px-2 transition-colors duration-80 outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)]",
              value === v ? "bg-white text-neutral-900" : "text-white/80 enabled:hover:bg-white/10",
            )}
            style={{ fontVariationSettings: value === v ? fontWeights.semibold : fontWeights.normal }}
          >
            <span className="[text-box:trim-both_cap_alphabetic]">{text}</span>
          </button>
        ))}
      </div>
    </fieldset>
  );
}

/** The reader's 表示設定 panel. A series either follows the global reader
 *  settings (the master, edited at /settings/reader) or keeps its own. While
 *  it follows, the controls show the values in effect but can't be changed;
 *  choosing この作品だけ starts from those same values. */
export function ReaderSettings({
  open,
  series,
  prefs,
  onChange,
  onClose,
}: {
  open: boolean;
  series: SeriesReaderPrefs;
  /** The values in effect (global resolved for this series, or its own). */
  prefs: ReaderPrefs;
  onChange: (p: SeriesReaderPrefs) => void;
  onClose: () => void;
}) {
  const custom = series.mode === "custom";
  const set = <K extends keyof ReaderPrefs>(k: K) => (v: ReaderPrefs[K]) => onChange({ mode: "custom", prefs: { ...prefs, [k]: v } });
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
          className="absolute inset-y-0 right-0 z-30 flex w-full max-w-xs flex-col gap-5 overflow-y-auto border-l border-white/8 bg-reader p-5 pt-[max(env(safe-area-inset-top),20px)] text-white"
          onPointerDown={(e) => e.stopPropagation()}
        >
          <div className="flex items-center justify-between">
            <p className={typeClass("title")} style={{ fontVariationSettings: fontWeights.semibold }}>
              表示設定
            </p>
            <button type="button" onClick={onClose} className={cn(typeClass("caption"), "inline-flex h-7 items-center rounded px-2 text-white/70 hover:text-white")}>
              <span className="[text-box:trim-both_cap_alphabetic]">閉じる</span>
            </button>
          </div>

          <div className="space-y-2">
            <Segmented
              label="この作品の設定"
              value={series.mode}
              onChange={(m) => onChange(m === "custom" ? { mode: "custom", prefs } : { mode: "global" })}
              options={[
                ["global", "全体設定に従う"],
                ["custom", "この作品だけ"],
              ]}
            />
            <p className={cn(typeClass("caption"), "text-white/60")}>
              {custom ? "この作品は下の設定で開きます。" : "全体設定の値を使っています。"}
              <Link to="/settings/reader" className="ml-1 text-white/80 underline underline-offset-2 hover:text-white">
                全体設定を開く
              </Link>
            </p>
          </div>

          <div className="h-px bg-white/8" />

          <Segmented label="読み方向" value={prefs.direction} onChange={set("direction")} options={opts(DIRECTION_LABEL)} disabled={!custom} />
          {paged && <Segmented label="見開き" value={prefs.spread} onChange={set("spread")} options={opts(SPREAD_LABEL)} disabled={!custom} />}
          {paged && <Segmented label="表示サイズ" value={prefs.fit} onChange={set("fit")} options={opts(FIT_LABEL)} disabled={!custom} />}
          <Segmented label="背景" value={prefs.bg} onChange={set("bg")} options={opts(BG_LABEL)} disabled={!custom} />

          <div className={cn(typeClass("caption"), "mt-auto space-y-1 text-white/60")}>
            <p>キー操作: ← → ページ送り / Space 次へ / M メニュー / Esc 閉じる</p>
            <p>画面の左右をタップでページ送り、中央でメニュー。</p>
          </div>
        </motion.aside>
      )}
    </AnimatePresence>
  );
}
