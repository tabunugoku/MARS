import { AnimatePresence, motion } from "motion/react";
import { Button } from "@/components/ui/button";
import { spring } from "@/lib/springs";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";

/** Unsaved-changes island for forms that save in one go. It floats at the
 *  bottom of the scroll area (on phones the scroll padding already clears the
 *  tabs, hence bottom-0), so save and discard are
 *  reachable from anywhere on the page. Render it last inside `Page`. */
export function SaveBar({ open, onDiscard, onSave }: { open: boolean; onDiscard: () => void; onSave: () => void }) {
  return (
    <div className="pointer-events-none sticky bottom-0 z-10 mt-8 flex justify-center md:bottom-6">
      <AnimatePresence initial={false}>
        {open && (
          <motion.div
            role="region"
            aria-label="未保存の変更"
            initial={{ opacity: 0, y: 12 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: 12, transition: spring.moderate.exit }}
            transition={spring.moderate}
            className="pointer-events-auto flex items-center gap-2 rounded-xl border border-border bg-popover py-2 pl-4 pr-2 text-popover-foreground shadow-[0_8px_24px_rgb(0_0_0/0.24)]"
          >
            <p className={cn(typeClass("body"), "mr-2 text-muted-foreground")}>未保存の変更があります</p>
            <Button variant="ghost" onClick={onDiscard}>
              破棄
            </Button>
            <Button variant="primary" onClick={onSave}>
              保存
            </Button>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}
