import { api, useApi } from "@/api";
import { EmptyState, Page, PageHeader } from "@/components/media/layout";
import { ListCard, ListGrid, ListGridSkeleton, NewListDialog, ViewSwitch } from "../collections/_parts";

export default function Screen() {
  const { data } = useApi(async () => {
    const lists = await api.readLists();
    // Only the first four books of each list are shown as covers.
    const ids = [...new Set(lists.flatMap((l) => l.bookIds.slice(0, 4)))];
    const books = await api.booksByIds(ids);
    return { lists, byId: new Map(books.map((b) => [b.id, b])) };
  }, []);

  return (
    <Page>
      <PageHeader
        title="リーディングリスト"
        meta={data ? `${data.lists.length} 件` : undefined}
        actions={<NewListDialog kind="readlist" />}
      />
      <ViewSwitch current="readlists" />

      {!data ? (
        <ListGridSkeleton />
      ) : data.lists.length === 0 ? (
        <EmptyState title="リーディングリストがありません" body="新規作成から巻を順番に並べると、ここに表示されます" />
      ) : (
        <ListGrid>
          {data.lists.map((l) => (
            <ListCard
              key={l.id}
              to={`/readlists/${l.id}`}
              name={l.name}
              count={`本 ${l.bookIds.length}`}
              covers={l.bookIds
                .slice(0, 4)
                .map((id) => data.byId.get(id))
                .filter((b) => b !== undefined)
                .map((b) => ({ seed: b.coverSeed, title: b.title, label: String(b.number) }))}
            />
          ))}
        </ListGrid>
      )}
    </Page>
  );
}
