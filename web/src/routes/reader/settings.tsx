import { AnimatePresence, motion } from "motion/react";
import { Link } from "react-router";
import { bgLabel, directionLabel, fitLabel, profileForLabel, spreadLabel, activeProfile, type ReaderPrefs, type ReaderScope } from "@/api";
import { spring } from "@/lib/springs";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";
import { defineMessages, useT } from "@/i18n";

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

const m = defineMessages({
  ja: {
    title: "表示設定",
    close: "閉じる",
    useSettings: "使う設定",
    scopeGlobal: "全体設定",
    scopeSeries: "シリーズ共通",
    scopeBook: "この巻だけ",
    seriesNote: "このシリーズの全巻が下の設定で開きます。",
    bookNote: "この巻だけ下の設定で開きます。",
    globalNote: "全体設定（{profile}）の値を使っています。",
    openGlobal: "全体設定を開く",
    direction: "読み方向",
    spread: "見開き",
    pageSize: "表示サイズ",
    background: "背景",
    keyHint: "キー操作: ← → ページ送り / Space 次へ / M メニュー / Esc 閉じる",
    tapHint: "画面の左右をタップでページ送り、中央でメニュー。",
  },
  en: {
    title: "Display settings",
    close: "Close",
    useSettings: "Settings to use",
    scopeGlobal: "Global",
    scopeSeries: "Whole series",
    scopeBook: "This book only",
    seriesNote: "All volumes of this series open with the settings below",
    bookNote: "Only this volume opens with the settings below",
    globalNote: "Using the global settings ({profile})",
    openGlobal: "Open global settings",
    direction: "Reading direction",
    spread: "Spread",
    pageSize: "Page size",
    background: "Background",
    keyHint: "Keys: ← → turn pages / Space next / M menu / Esc close",
    tapHint: "Tap the left or right side to turn pages, the middle for the menu",
  },
});

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
  const t = useT(m);
  const set = <K extends keyof ReaderPrefs>(k: K) => (v: ReaderPrefs[K]) => onChange({ ...prefs, [k]: v });
  const paged = prefs.direction !== "webtoon";
  return (
    <AnimatePresence>
      {open && (
        <motion.aside
          key="settings"
          role="dialog"
          aria-label={t("title")}
          initial={{ x: "100%" }}
          animate={{ x: 0 }}
          exit={{ x: "100%", transition: spring.slow.exit }}
          transition={spring.slow}
          className="absolute inset-y-0 right-0 z-30 flex w-full max-w-xs flex-col gap-5 overflow-y-auto border-l border-white/8 bg-reader p-5 pb-[max(env(safe-area-inset-bottom),20px)] pr-[max(env(safe-area-inset-right),20px)] pt-[max(env(safe-area-inset-top),20px)] text-white"
          onPointerDown={(e) => e.stopPropagation()}
        >
          <div className="flex items-center justify-between">
            <p className={typeClass("title")} style={{ fontVariationSettings: fontWeights.semibold }}>
              {t("title")}
            </p>
            <button type="button" onClick={onClose} className={cn(typeClass("caption"), "-mr-3 -my-2 inline-flex h-11 items-center rounded px-3 text-white/70 hover:text-white")}>
              <span className="[text-box:trim-both_cap_alphabetic]">{t("close")}</span>
            </button>
          </div>

          <div className="space-y-2">
            <Segmented
              label={t("useSettings")}
              value={scope}
              onChange={onScope}
              options={[
                ["global", t("scopeGlobal")],
                ["series", t("scopeSeries")],
                ["book", t("scopeBook")],
              ]}
            />
            <p className={cn(typeClass("caption"), "text-white/60")}>
              {scope === "global" ? t("globalNote", { profile: profileForLabel()[activeProfile()] }) : t(scope === "series" ? "seriesNote" : "bookNote")}
              <Link to="/settings/reader" className="relative ml-1 text-white/80 underline underline-offset-2 touch-hit hover:text-white">
                {t("openGlobal")}
              </Link>
            </p>
          </div>

          <div className="h-px bg-white/8" />

          <Segmented label={t("direction")} value={prefs.direction} onChange={set("direction")} options={opts(directionLabel())} disabled={!custom} />
          {paged && <Segmented label={t("spread")} value={prefs.spread} onChange={set("spread")} options={opts(spreadLabel())} disabled={!custom} />}
          {paged && <Segmented label={t("pageSize")} value={prefs.fit} onChange={set("fit")} options={opts(fitLabel())} disabled={!custom} />}
          <Segmented label={t("background")} value={prefs.bg} onChange={set("bg")} options={opts(bgLabel())} disabled={!custom} />

          <div className={cn(typeClass("caption"), "mt-auto space-y-1 text-white/60")}>
            <p className="pointer-coarse:hidden">{t("keyHint")}</p>
            <p>{t("tapHint")}</p>
          </div>
        </motion.aside>
      )}
    </AnimatePresence>
  );
}
