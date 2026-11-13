import type { ReactNode } from "react";
import { motion } from "motion/react";
import { Check, X } from "@keyline-icons/react";
import { Button } from "@/components/ui/button";
import { DropdownContent, DropdownMenu, DropdownTrigger } from "@/components/ui/dropdown";
import { MenuItem } from "@/components/ui/menu-item";
import { Tooltip } from "@/components/ui/tooltip";
import type { IconComponent } from "@/lib/icon-context";
import { spring } from "@/lib/springs";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { defineMessages, useT } from "@/i18n";

const m = defineMessages({
  ja: {
    cancel: "キャンセル",
    count: ({ n }) => `${n} 件選択`,
    selectAll: "すべて選択",
    more: "その他",
    barLabel: "選択中の操作",
  },
  en: {
    cancel: "Cancel",
    count: ({ n }) => `${n} selected`,
    selectAll: "Select all",
    more: "More",
    barLabel: "Selection actions",
  },
});

/** Wraps one card. Outside selection mode it is only a div. In selection mode the
 *  card is inert and a button covers it: a click toggles, nothing navigates. */
export function SelectTile({
  selecting,
  selected,
  label,
  onToggle,
  children,
}: {
  selecting: boolean;
  selected: boolean;
  label: string;
  onToggle: (shift: boolean) => void;
  children: ReactNode;
}) {
  return (
    <div className="relative min-w-0">
      <div inert={selecting || undefined} className={cn(selecting && "select-none")}>
        {children}
      </div>
      {selecting && (
        <>
          <button
            type="button"
            aria-pressed={selected}
            aria-label={label}
            onMouseDown={(e) => e.shiftKey && e.preventDefault()}
            onClick={(e) => onToggle(e.shiftKey)}
            className={cn(
              "absolute inset-0 z-10 rounded-[8px] outline-none focus-visible:ring-2 ring-[var(--focus-ring)] transition-[box-shadow] duration-160 ease-[cubic-bezier(0.16,1,0.3,1)]",
              selected ? "ring-2 ring-brand" : "hover:ring-1 hover:ring-foreground/30",
            )}
          />
          <span
            aria-hidden
            className={cn(
              "pointer-events-none absolute left-2 top-2 z-20 flex size-6 items-center justify-center rounded-full border-2 transition-colors duration-160",
              selected ? "border-brand-fill bg-brand-fill text-brand-foreground" : "border-foreground/60 bg-background/80",
            )}
          >
            {selected && <Check size={14} strokeWidth={2.5} />}
          </span>
        </>
      )}
    </div>
  );
}

export interface BarAction {
  key: string;
  label: string;
  icon?: IconComponent;
  onSelect: () => void;
}

/** The floating bar of selection mode. Render inside AnimatePresence while active.
 *  Wide screens show the actions as buttons; phones fold them into その他. */
export function SelectionBar({
  count,
  busy,
  onSelectAll,
  selectAllLabel,
  selectAllBusy = false,
  onCancel,
  actions,
}: {
  count: number;
  busy: boolean;
  onSelectAll: () => void;
  /** Overrides the select-all label (e.g. "読み込み済みの N 件を選択"). */
  selectAllLabel?: string;
  /** Disables select-all while the screen loads what it needs for it. */
  selectAllBusy?: boolean;
  onCancel: () => void;
  actions: BarAction[];
}) {
  const t = useT(m);
  return (
    <div className="pointer-events-none fixed inset-x-0 bottom-[calc(4.5rem+env(safe-area-inset-bottom))] z-[var(--z-sticky)] flex justify-center px-4 md:bottom-6">
      <motion.div
        role="region"
        aria-label={t("barLabel")}
        initial={{ opacity: 0, y: 12 }}
        animate={{ opacity: 1, y: 0 }}
        exit={{ opacity: 0, y: 12, transition: spring.moderate.exit }}
        transition={spring.moderate}
        className="pointer-events-auto flex max-w-full items-center gap-2 rounded-[12px] border border-border bg-popover py-2 pl-4 pr-2 text-popover-foreground shadow-[0_8px_24px_rgb(0_0_0/0.24)]"
      >
        <p className={cn(typeClass("body"), "mr-1 shrink-0 tabular-nums")}>{t("count", { n: count })}</p>

        <div className="hidden items-center gap-1 md:flex">
          <Button variant="ghost" className="pointer-coarse:h-11" disabled={busy || selectAllBusy} onClick={onSelectAll}>
            {selectAllLabel ?? t("selectAll")}
          </Button>
          {actions.map((a) => (
            <Button
              key={a.key}
              variant="secondary"
              className="pointer-coarse:h-11"
              disabled={busy}
              leadingIcon={a.icon}
              onClick={a.onSelect}
            >
              {a.label}
            </Button>
          ))}
        </div>

        <DropdownMenu>
          <DropdownTrigger render={<Button variant="secondary" className="pointer-coarse:h-11 md:hidden" disabled={busy}>{t("more")}</Button>} />
          <DropdownContent align="end" sideOffset={4}>
            <MenuItem index={0} label={selectAllLabel ?? t("selectAll")} disabled={selectAllBusy} onSelect={onSelectAll} />
            {actions.map((a, i) => (
              <MenuItem key={a.key} index={i + 1} label={a.label} icon={a.icon} disabled={busy} onSelect={a.onSelect} />
            ))}
          </DropdownContent>
        </DropdownMenu>

        <Tooltip content={t("cancel")}>
          <span className="inline-flex">
            <Button variant="ghost" size="icon" aria-label={t("cancel")} className="pointer-coarse:h-11 pointer-coarse:w-11 md:hidden" onClick={onCancel}>
              <X size={16} />
            </Button>
          </span>
        </Tooltip>
        <Button variant="ghost" className="hidden pointer-coarse:h-11 md:inline-flex" onClick={onCancel}>
          {t("cancel")}
        </Button>
      </motion.div>
    </div>
  );
}

