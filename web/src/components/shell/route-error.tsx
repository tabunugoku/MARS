import { Link, useRouteError } from "react-router";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";
import { defineMessages, useT } from "@/i18n";

const m = defineMessages({
  ja: {
    newVersion: "新しいバージョンがあります",
    failed: "画面を表示できませんでした",
    reloadHint: "再読み込みすると最新の画面になります。",
    retryHint: "時間をおいてもう一度お試しください。",
    reload: "再読み込み",
    home: "ホームへ",
  },
  en: {
    newVersion: "A new version is available",
    failed: "This page could not be shown",
    reloadHint: "Reload to get the latest version of the page",
    retryHint: "Try again in a moment",
    reload: "Reload",
    home: "Go home",
  },
});

/** Shown when a route throws or its chunk fails to load (e.g. after a deploy). */
export function RouteError() {
  const err = useRouteError() as Error | undefined;
  const t = useT(m);
  const chunk = /dynamically imported module|Importing a module script failed/i.test(err?.message ?? "");
  return (
    <div className="flex min-h-full flex-col items-center justify-center gap-3 bg-background p-8 text-center">
      <p className={typeClass("title")} style={{ fontVariationSettings: fontWeights.semibold }}>
        {chunk ? t("newVersion") : t("failed")}
      </p>
      <p className={cn(typeClass("body"), "max-w-sm text-muted-foreground")}>
        {chunk ? t("reloadHint") : t("retryHint")}
      </p>
      <div className="mt-2 flex gap-3">
        <button type="button" onClick={() => location.reload()} className={cn(typeClass("body"), "inline-flex h-9 items-center rounded-lg bg-foreground px-4 text-background")}>
          <span className="[text-box:trim-both_cap_alphabetic]">{t("reload")}</span>
        </button>
        <Link to="/" className={cn(typeClass("body"), "flex h-9 items-center rounded-lg px-4 text-muted-foreground hover:text-foreground")}>
          <span className="[text-box:trim-both_cap_alphabetic]">{t("home")}</span>
        </Link>
      </div>
    </div>
  );
}
