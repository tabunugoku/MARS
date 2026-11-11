import { api, useApi } from "@/api";
import { LoadError } from "@/components/shell/load-error";
import { EmptyState, Page, PageHeader } from "@/components/media/layout";
import { ListCard, ListGrid, ListGridSkeleton, NewListDialog, ViewSwitch } from "./_parts";

export default function Screen() {
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
        title="コレクション"
        meta={data ? `${data.cols.length} 件` : undefined}
        actions={data?.isAdmin ? <NewListDialog kind="collection" /> : undefined}
      />
      <ViewSwitch current="collections" />

      {error ? (
        <LoadError message={error} onRetry={reload} />
      ) : !data ? (
        <ListGridSkeleton />
      ) : data.cols.length === 0 ? (
        <EmptyState title="コレクションがありません" body="新規作成からシリーズをまとめると、ここに表示されます" />
      ) : (
        <ListGrid>
          {data.cols.map((c) => (
            <ListCard
              key={c.id}
              to={`/collections/${c.id}`}
              name={c.name}
              count={`シリーズ ${c.seriesIds.length}`}
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
