import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { defineMessages, useT } from "@/i18n";

const m = defineMessages({
  ja: {
    failed: "読み込めませんでした：",
    retry: "もう一度読み込む",
  },
  en: {
    failed: "Could not load: ",
    retry: "Try again",
  },
});

/** Shown in place of a screen's content when its data could not be loaded
 *  (useApi's error). The message comes from the server or the client. */
export function LoadError({ message, onRetry, className }: { message: string; onRetry?: () => void; className?: string }) {
  const t = useT(m);
  return (
    <div role="alert" className={cn("flex flex-col items-start gap-3 py-8", className)}>
      <p className={typeClass("body")}>
        {t("failed")}
        {message}
      </p>
      {onRetry && (
        <Button variant="secondary" onClick={onRetry}>
          {t("retry")}
        </Button>
      )}
    </div>
  );
}
