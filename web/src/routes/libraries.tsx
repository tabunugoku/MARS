import type { ReactNode } from "react";
import { Link } from "react-router";
import { ChevronRight, LibraryBig } from "@keyline-icons/react";
import { api, useApi, type Library } from "@/api";
import { KindIcon } from "@/components/media/kind-icon";
import { EmptyState, Page, PageHeader } from "@/components/media/layout";
import { Block } from "@/components/media/skeletons";
import { LoadError } from "@/components/shell/load-error";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";
import { defineMessages, useT } from "@/i18n";

// The library list: where the phone's Library tab goes, like Search. One row
// for all libraries, then one per library, each opening its series grid.

const m = defineMessages({
  ja: {
    title: "ライブラリ",
    all: "すべてのライブラリ",
    counts: ({ series, books }) => `シリーズ ${series} · 本 ${books}`,
    emptyTitle: "ライブラリがありません",
    emptyBody: "管理 → ライブラリ設定でフォルダを追加すると、ここに表示されます",
  },
  en: {
    title: "Libraries",
    all: "All libraries",
    counts: ({ series, books }) => `${series} series · ${books} books`,
    emptyTitle: "No libraries yet",
    emptyBody: "Add a folder in Admin → Libraries and it shows up here",
  },
});

function Row({ to, icon, name, meta }: { to: string; icon: ReactNode; name: string; meta: string }) {
  return (
    <li>
      <Link to={to} className="flex min-h-14 items-center gap-3 px-4 py-2 transition-colors duration-80 hover:bg-hover">
        {icon}
        <span className="min-w-0 flex-1">
          <span className={cn(typeClass("body"), "block truncate text-foreground")} style={{ fontVariationSettings: fontWeights.semibold }}>
            {name}
          </span>
          <span className={cn(typeClass("caption"), "block font-mono tabular-nums text-muted-foreground")}>{meta}</span>
        </span>
        <ChevronRight size={16} className="shrink-0 text-muted-foreground" />
      </Link>
    </li>
  );
}

export default function Screen() {
  const t = useT(m);
  const { data: libs, error, reload } = useApi(() => api.libraries(), []);
  const total = (libs ?? []).reduce((a, l: Library) => ({ series: a.series + l.seriesCount, books: a.books + l.bookCount }), { series: 0, books: 0 });

  return (
    <Page>
      <PageHeader title={t("title")} />
      {error ? (
        <LoadError message={error} onRetry={reload} />
      ) : !libs ? (
        <div className="space-y-2">
          {[0, 1, 2].map((i) => (
            <Block key={i} className="h-14 rounded-[12px]" />
          ))}
        </div>
      ) : libs.length === 0 ? (
        <EmptyState title={t("emptyTitle")} body={t("emptyBody")} />
      ) : (
        <ul className="divide-y divide-border overflow-hidden rounded-[12px] bg-card">
          <Row
            to="/libraries/all"
            name={t("all")}
            meta={t("counts", total)}
            icon={
              <span aria-hidden className="grid size-8 shrink-0 place-items-center rounded-[9px] bg-brand-soft text-brand">
                <LibraryBig size={16} strokeWidth={1.75} />
              </span>
            }
          />
          {libs.map((l) => (
            <Row
              key={l.id}
              to={`/libraries/${l.id}`}
              name={l.name}
              meta={t("counts", { series: l.seriesCount, books: l.bookCount })}
              icon={<KindIcon kind={l.kind} />}
            />
          ))}
        </ul>
      )}
    </Page>
  );
}
