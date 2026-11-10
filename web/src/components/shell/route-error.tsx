import { Link, useRouteError } from "react-router";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";

/** Shown when a route throws or its chunk fails to load (e.g. after a deploy). */
export function RouteError() {
  const err = useRouteError() as Error | undefined;
  const chunk = /dynamically imported module|Importing a module script failed/i.test(err?.message ?? "");
  return (
    <div className="flex min-h-full flex-col items-center justify-center gap-3 bg-background p-8 text-center">
      <p className={typeClass("title")} style={{ fontVariationSettings: fontWeights.semibold }}>
        {chunk ? "新しいバージョンがあります" : "画面を表示できませんでした"}
      </p>
      <p className={cn(typeClass("body"), "max-w-sm text-muted-foreground")}>
        {chunk ? "再読み込みすると最新の画面になります。" : "時間をおいてもう一度お試しください。"}
      </p>
      <div className="mt-2 flex gap-3">
        <button type="button" onClick={() => location.reload()} className={cn(typeClass("body"), "inline-flex h-9 items-center rounded-lg bg-foreground px-4 text-background")}>
          <span className="[text-box:trim-both_cap_alphabetic]">再読み込み</span>
        </button>
        <Link to="/" className={cn(typeClass("body"), "flex h-9 items-center rounded-lg px-4 text-muted-foreground hover:text-foreground")}>
          <span className="[text-box:trim-both_cap_alphabetic]">ホームへ</span>
        </Link>
      </div>
    </div>
  );
}
