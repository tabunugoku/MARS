import { api, useApi } from "@/api";
import { LoadError } from "@/components/shell/load-error";
import { EmptyState, Page, PageHeader } from "@/components/media/layout";
import { defineMessages, useT } from "@/i18n";
import { ListCard, ListGrid, ListGridSkeleton, NewListDialog, ViewSwitch } from "../collections/_parts";
import { ImportCblDialog } from "./import-dialog";

const m = defineMessages({
  ja: {
    title: "リーディングリスト",
    count: ({ n }) => `${n} 件`,
    books: ({ n }) => `本 ${n}`,
    emptyTitle: "リーディングリストがありません",
    emptyBody: "新規作成から巻を順番に並べると、ここに表示されます",
  },
  en: {
    title: "Read lists",
    count: ({ n }) => (n === 1 ? "1 read list" : `${n} read lists`),
    books: ({ n }) => (n === 1 ? "1 book" : `${n} books`),
    emptyTitle: "No read lists",
    emptyBody: "Create one and put books in order. It will show up here",
  },
});

export default function Screen() {
  const t = useT(m);
  const { data, error, reload } = useApi(async () => {
    const [lists, me] = await Promise.all([api.readLists(), api.me()]);
    // Only the first four books of each list are shown as covers.
    const ids = [...new Set(lists.flatMap((l) => l.bookIds.slice(0, 4)))];
    const books = await api.booksByIds(ids);
    return { lists, isAdmin: me.role === "admin", byId: new Map(books.map((b) => [b.id, b])) };
  }, []);

  return (
    <Page>
      <PageHeader
        title={t("title")}
        meta={data ? t("count", { n: data.lists.length }) : undefined}
        stackActions
        actions={
          data?.isAdmin ? (
            <div className="flex flex-wrap gap-2 sm:flex-row-reverse">
              {/* "New" where the collections page has it: first on a phone, rightmost when wider. */}
              <NewListDialog kind="readlist" />
              <ImportCblDialog />
            </div>
          ) : undefined
        }
      />
      <ViewSwitch current="readlists" />

      {error ? (
        <LoadError message={error} onRetry={reload} />
      ) : !data ? (
        <ListGridSkeleton />
      ) : data.lists.length === 0 ? (
        <EmptyState title={t("emptyTitle")} body={t("emptyBody")} />
      ) : (
        <ListGrid>
          {data.lists.map((l) => (
            <ListCard
              key={l.id}
              to={`/readlists/${l.id}`}
              name={l.name}
              count={t("books", { n: l.bookIds.length })}
              covers={l.bookIds
                .slice(0, 4)
                .map((id) => data.byId.get(id))
                .filter((b) => b !== undefined)
                .map((b) => ({ src: b.thumbnailUrl, title: b.title, label: String(b.number) }))}
            />
          ))}
        </ListGrid>
      )}
    </Page>
  );
}
