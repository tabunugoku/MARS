import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";

/** Shown in place of a screen's content when its data could not be loaded
 *  (useApi's error). The message comes from the server or the client. */
export function LoadError({ message, onRetry, className }: { message: string; onRetry?: () => void; className?: string }) {
  return (
    <div role="alert" className={cn("flex flex-col items-start gap-3 py-8", className)}>
      <p className={typeClass("body")}>読み込めませんでした：{message}</p>
      {onRetry && (
        <Button variant="secondary" onClick={onRetry}>
          もう一度読み込む
        </Button>
      )}
    </div>
  );
}
