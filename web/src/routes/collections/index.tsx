import { api, useApi } from "@/api";
import { EmptyState, Page, PageHeader } from "@/components/media/layout";
import { ListCard, ListGrid, ListGridSkeleton, NewListDialog, ViewSwitch } from "./_parts";

export default function Screen() {
  const { data } = useApi(async () => {
    const [cols, series] = await Promise.all([api.collections(), api.series()]);
    return { cols, byId: new Map(series.map((s) => [s.id, s])) };
  }, []);

  return (
    <Page>
      <PageHeader
        title="コレクション"
        meta={data ? `${data.cols.length} 件` : undefined}
        actions={<NewListDialog kind="collection" />}
      />
      <ViewSwitch current="collections" />

      {!data ? (
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
                .map((id) => data.byId.get(id))
                .filter((s) => s !== undefined)
                .map((s) => ({ seed: s.coverSeed, title: s.title }))}
            />
          ))}
        </ListGrid>
      )}
    </Page>
  );
}
