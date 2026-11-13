import { api, useApi } from "@/api";
import { LoadError } from "@/components/shell/load-error";
import { EmptyState, Page, PageHeader } from "@/components/media/layout";
import { defineMessages, useT } from "@/i18n";
import { ListCard, ListGrid, ListGridSkeleton, NewListDialog, ViewSwitch } from "./_parts";

const m = defineMessages({
  ja: {
    title: "コレクション",
    count: ({ n }) => `${n} 件`,
    series: ({ n }) => `シリーズ ${n}`,
    emptyTitle: "コレクションがありません",
    emptyBody: "新規作成からシリーズをまとめると、ここに表示されます",
  },
  en: {
    title: "Collections",
    count: ({ n }) => (n === 1 ? "1 collection" : `${n} collections`),
    series: ({ n }) => (n === 1 ? "1 series" : `${n} series`),
    emptyTitle: "No collections",
    emptyBody: "Create one and group series in it. It will show up here",
  },
});

export default function Screen() {
  const t = useT(m);
  const { data, error, reload } = useApi(async () => {
    const [cols, me] = await Promise.all([api.collections(), api.me()]);
    // Covers come from the first few members of each collection only.
    const ids = [...new Set(cols.flatMap((c) => c.seriesIds.slice(0, 4)))];
    const series = await api.seriesByIds(ids);
    return { cols, isAdmin: me.role === "admin", byId: new Map(series.map((s) => [s.id, s])) };
  }, []);

  return (
    <Page>
      <PageHeader
        title={t("title")}
        meta={data ? t("count", { n: data.cols.length }) : undefined}
        stackActions
        actions={data?.isAdmin ? <NewListDialog kind="collection" /> : undefined}
      />
      <ViewSwitch current="collections" />

      {error ? (
        <LoadError message={error} onRetry={reload} />
      ) : !data ? (
        <ListGridSkeleton />
      ) : data.cols.length === 0 ? (
        <EmptyState title={t("emptyTitle")} body={t("emptyBody")} />
      ) : (
        <ListGrid>
          {data.cols.map((c) => (
            <ListCard
              key={c.id}
              to={`/collections/${c.id}`}
              name={c.name}
              count={t("series", { n: c.seriesIds.length })}
              covers={c.seriesIds
                .slice(0, 4)
                .map((id) => data.byId.get(id))
                .filter((s) => s !== undefined)
                .map((s) => ({ src: s.thumbnailUrl, title: s.title }))}
            />
          ))}
        </ListGrid>
      )}
    </Page>
  );
}
