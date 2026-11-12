import { AnimatePresence, motion } from "motion/react";
import { Link } from "react-router";
import { BG_LABEL, DIRECTION_LABEL, FIT_LABEL, PROFILE_FOR_LABEL, SPREAD_LABEL, activeProfile, type ReaderPrefs, type ReaderScope } from "@/api";
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

const SCOPE_NOTE: Record<Exclude<ReaderScope, "global">, string> = {
  series: "このシリーズの全巻が下の設定で開きます。",
  book: "この巻だけ下の設定で開きます。",
};

/** The reader's 表示設定 panel. Settings come from one of three scopes: the
 *  global set (the master, edited at /settings/reader), one shared by every
 *  volume of the series, or one for this volume only. Under 全体設定 the
 *  controls show the values in effect but are locked; switching scope starts
 *  the new scope from the values in effect. */
export function ReaderSettings({
  open,
  scope,
  prefs,
  onScope,
  onChange,
  onClose,
}: {
  open: boolean;
  scope: ReaderScope;
  /** The values in effect for this volume. */
  prefs: ReaderPrefs;
  onScope: (s: ReaderScope) => void;
  /** Edit the values of the current (non-global) scope. */
  onChange: (p: ReaderPrefs) => void;
  onClose: () => void;
}) {
  const custom = scope !== "global";
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
          className="absolute inset-y-0 right-0 z-30 flex w-full max-w-xs flex-col gap-5 overflow-y-auto border-l border-white/8 bg-reader p-5 pb-[max(env(safe-area-inset-bottom),20px)] pr-[max(env(safe-area-inset-right),20px)] pt-[max(env(safe-area-inset-top),20px)] text-white"
          onPointerDown={(e) => e.stopPropagation()}
        >
          <div className="flex items-center justify-between">
            <p className={typeClass("title")} style={{ fontVariationSettings: fontWeights.semibold }}>
              表示設定
            </p>
            <button type="button" onClick={onClose} className={cn(typeClass("caption"), "-mr-3 -my-2 inline-flex h-11 items-center rounded px-3 text-white/70 hover:text-white")}>
              <span className="[text-box:trim-both_cap_alphabetic]">閉じる</span>
            </button>
          </div>

          <div className="space-y-2">
            <Segmented
              label="使う設定"
              value={scope}
              onChange={onScope}
              options={[
                ["global", "全体設定"],
                ["series", "シリーズ共通"],
                ["book", "この巻だけ"],
              ]}
            />
            <p className={cn(typeClass("caption"), "text-white/60")}>
              {scope === "global" ? `全体設定（${PROFILE_FOR_LABEL[activeProfile()]}）の値を使っています。` : SCOPE_NOTE[scope]}
              <Link to="/settings/reader" className="relative ml-1 text-white/80 underline underline-offset-2 touch-hit hover:text-white">
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
            <p className="pointer-coarse:hidden">キー操作: ← → ページ送り / Space 次へ / M メニュー / Esc 閉じる</p>
            <p>画面の左右をタップでページ送り、中央でメニュー。</p>
          </div>
        </motion.aside>
      )}
    </AnimatePresence>
  );
}
