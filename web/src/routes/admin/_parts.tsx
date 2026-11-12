import type { ComponentType, ReactNode } from "react";
import { Block } from "@/components/media/skeletons";
import { defineMessages, intlLocale, useT } from "@/i18n";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tooltip } from "@/components/ui/tooltip";

const m = defineMessages({
  ja: {
    cancel: "キャンセル",
  },
  en: {
    cancel: "Cancel",
  },
});

const UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ["day", 86400],
  ["hour", 3600],
  ["minute", 60],
];

/** 「3時間前」style relative time. `null` (never happened) shows a dash. */
export function relTime(iso: string | null): string {
  if (!iso) return "—";
  const rtf = new Intl.RelativeTimeFormat(intlLocale(), { numeric: "auto" });
  const diff = (Date.parse(iso) - Date.now()) / 1000;
  for (const [unit, sec] of UNITS) {
    if (Math.abs(diff) >= sec) return rtf.format(Math.round(diff / sec), unit);
  }
  return rtf.format(Math.round(diff), "second");
}

/** Icon-only row action: 36px hit target, Japanese aria-label, tooltip. */
export function IconAction({
  label,
  icon: Icon,
  onClick,
}: {
  label: string;
  icon: ComponentType<{ size?: number }>;
  onClick: () => void;
}) {
  return (
    <Tooltip content={label}>
      <Button variant="ghost" size="icon" aria-label={label} onClick={onClick}>
        <Icon size={16} />
      </Button>
    </Tooltip>
  );
}

/** Destructive confirmation. Cancel is ghost, the confirm is destructive. */
export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  body,
  confirmLabel,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  body: ReactNode;
  confirmLabel: string;
  onConfirm: () => void;
}) {
  const t = useT(m);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="sm">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{body}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <DialogClose render={<Button variant="ghost">{t("cancel")}</Button>} />
          <Button
            variant="destructive"
            onClick={() => {
              onConfirm();
              onOpenChange(false);
            }}
          >
            {confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** Loading state for a table: the real header row and skeleton body rows. */
export function TableSkeleton({ heads, rows = 4 }: { heads: string[]; rows?: number }) {
  return (
    <div className="overflow-x-auto" aria-hidden>
      <Table>
        <TableHeader>
          <TableRow>
            {heads.map((h) => (
              <TableHead key={h}>{h}</TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {Array.from({ length: rows }, (_, r) => (
            <TableRow key={r} index={r}>
              {heads.map((h) => (
                <TableCell key={h}>
                  <Block className="h-4 w-2/3" />
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
