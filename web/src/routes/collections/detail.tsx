import { useState } from "react";
import { useNavigate, useParams } from "react-router";
import { toast } from "@yusufceng1z/ding-react";
import { ArrowDown, ArrowUp, MoreHorizontal, SquarePen, X } from "@keyline-icons/react";
import { api, ApiError, errorMessage, useApi, type Collection, type Series } from "@/api";
import { SeriesCard } from "@/components/media/cards";
import { CoverGrid, EmptyState, Page, PageHeader } from "@/components/media/layout";
import { Block, GridSkeleton } from "@/components/media/skeletons";
import { LoadError } from "@/components/shell/load-error";
import { Button } from "@/components/ui/button";
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownContent, DropdownMenu, DropdownTrigger } from "@/components/ui/dropdown";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { MenuItem } from "@/components/ui/menu-item";
import { Tooltip } from "@/components/ui/tooltip";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { defineMessages, useT } from "@/i18n";

const m = defineMessages({
  ja: {
    notFound: "コレクションが見つかりません",
    backToCollections: "コレクションへ戻る",
    collections: "コレクション",
    renamed: "名前を変更しました",
    deleted: "削除しました",
    conflict: "ほかの画面で変更されました。読み込み直します",
    orderSaved: "並び順を保存しました",
    removedFromCollection: "コレクションから外しました",
    removeFromCollection: "コレクションから外す",
    seriesCount: ({ n }) => `シリーズ ${n}`,
    manualOrder: "手動の並び順",
    autoOrder: "自動の並び順",
    moreActions: "その他の操作",
    rename: "名前を変更",
    delete: "削除",
    deleteTitle: "コレクションを削除",
    name: "名前",
    cancel: "キャンセル",
    save: "保存",
    deleteBody: "「{name}」を削除します。シリーズは削除されません。",
    emptyTitle: "シリーズがありません",
    emptyAdmin: "シリーズの「その他」メニューの「コレクションに追加」から追加できます。",
    emptyUser: "管理者がシリーズを追加すると、ここに表示されます",
    up: "上へ",
    down: "下へ",
  },
  en: {
    notFound: "Collection not found",
    backToCollections: "Back to collections",
    collections: "Collections",
    renamed: "Name changed",
    deleted: "Deleted",
    conflict: "Changed on another screen. Reloading",
    orderSaved: "Order saved",
    removedFromCollection: "Removed from collection",
    removeFromCollection: "Remove from collection",
    seriesCount: ({ n }) => (n === 1 ? "1 series" : `${n} series`),
    manualOrder: "Manual order",
    autoOrder: "Automatic order",
    moreActions: "More actions",
    rename: "Rename",
    delete: "Delete",
    deleteTitle: "Delete collection",
    name: "Name",
    cancel: "Cancel",
    save: "Save",
    deleteBody: 'Delete "{name}". Series are not deleted.',
    emptyTitle: "No series",
    emptyAdmin: "Add series from a series' More menu, with Add to collection",
    emptyUser: "Series added by an admin will show up here",
    up: "Up",
    down: "Down",
  },
});

/** The items in the order of ids; items whose id is not listed are dropped. */
function orderBy<T extends { id: string }>(xs: T[], ids: string[]): T[] {
  const byId = new Map(xs.map((x) => [x.id, x] as const));
  return ids.flatMap((id) => byId.get(id) ?? []);
}

export default function Screen() {
  const t = useT(m);
  const { collectionId = "" } = useParams();
  const navigate = useNavigate();
  const [dialog, setDialog] = useState<"rename" | "delete" | null>(null);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  // The member order the last change returned, for the data it was made on.
  // It is ignored once the page has reloaded.
  const [local, setLocal] = useState<{ base: unknown; ids: string[] } | null>(null);

  const { data, error, reload } = useApi(async () => {
    const c = await api.collection(collectionId);
    if (!c) return { c: null, series: [] as Series[], isAdmin: false };
    const [series, me] = await Promise.all([api.collectionSeries(collectionId), api.me()]);
    return { c, series, isAdmin: me.role === "admin" };
  }, [collectionId]);

  if (error) {
    return (
      <Page>
        <LoadError message={error} onRetry={reload} />
      </Page>
    );
  }

  if (!data) {
    return (
      <Page>
        <Block className="mb-6 mt-2 h-8 w-56" />
        <GridSkeleton />
      </Page>
    );
  }

  const { c, series, isAdmin } = data;
  if (!c) {
    return (
      <Page>
        <EmptyState title={t("notFound")} action={<Button variant="secondary" onClick={() => navigate("/collections")}>{t("backToCollections")}</Button>} />
      </Page>
    );
  }

  // The members as the last change left them; the server's list otherwise.
  const shown = local && local.base === data ? orderBy(series, local.ids) : series;

  const openDialog = (kind: "rename" | "delete") => {
    setName(c.name);
    setDialog(kind);
  };

  const save = () => {
    setBusy(true);
    api
      .updateCollection(c.id, { name: name.trim() })
      .then(() => {
        toast.success(t("renamed"));
        setDialog(null);
        reload();
      })
      .catch((err: unknown) => toast.error(errorMessage(err)))
      .finally(() => setBusy(false));
  };

  const remove = () => {
    setBusy(true);
    api
      .deleteCollection(c.id)
      .then(() => {
        toast.success(t("deleted"));
        navigate("/collections");
      })
      .catch((err: unknown) => toast.error(errorMessage(err)))
      .finally(() => setBusy(false));
  };

  // Runs one member change. The returned list is applied at once, with the
  // controls still disabled until then, and the page reloads after it.
  const commit = (call: Promise<Collection>, done?: string) => {
    setBusy(true);
    call
      .then((updated) => {
        setLocal({ base: data, ids: updated.seriesIds });
        setBusy(false);
        if (done) toast.success(done);
        reload();
      })
      .catch((err: unknown) => {
        setBusy(false);
        if (err instanceof ApiError && err.status === 409) {
          toast.error(t("conflict"));
          reload();
        } else {
          toast.error(errorMessage(err));
        }
      });
  };

  const move = (seriesId: string, by: -1 | 1) => {
    const ids = shown.map((s) => s.id);
    const index = ids.indexOf(seriesId);
    const target = index + by;
    if (index < 0 || target < 0 || target >= ids.length) return;
    [ids[index], ids[target]] = [ids[target], ids[index]];
    commit(api.reorderCollection(c.id, ids), t("orderSaved"));
  };

  const removeFromCollection = (seriesId: string) => {
    commit(api.removeCollectionSeries(c.id, seriesId), t("removedFromCollection"));
  };

  return (
    <Page>
      <PageHeader
        back={{ to: "/collections", label: t("collections") }}
        title={c.name}
        meta={`${t("seriesCount", { n: shown.length })} · ${c.ordered ? t("manualOrder") : t("autoOrder")}`}
        actions={
          isAdmin && (
            <DropdownMenu>
              <Tooltip content={t("moreActions")}>
                <span className="inline-flex">
                  <DropdownTrigger render={<Button variant="ghost" size="icon" aria-label={t("moreActions")}><MoreHorizontal size={16} /></Button>} />
                </span>
              </Tooltip>
              <DropdownContent align="end" className="w-56">
                <MenuItem index={0} label={t("rename")} icon={SquarePen} onSelect={() => openDialog("rename")} />
                <MenuItem index={1} label={t("delete")} onSelect={() => openDialog("delete")} />
              </DropdownContent>
            </DropdownMenu>
          )
        }
      />

      <Dialog open={dialog !== null} onOpenChange={(o) => !o && setDialog(null)}>
        <DialogContent size="sm">
          {dialog === "rename" ? (
            <>
              <DialogHeader>
                <DialogTitle>{t("rename")}</DialogTitle>
              </DialogHeader>
              <Field>
                <FieldLabel htmlFor="collection-name">{t("name")}</FieldLabel>
                <Input id="collection-name" value={name} onChange={(e) => setName(e.target.value)} />
              </Field>
              <DialogFooter>
                <DialogClose render={<Button variant="ghost">{t("cancel")}</Button>} />
                <Button variant="primary" disabled={!name.trim() || busy} onClick={save}>
                  {t("save")}
                </Button>
              </DialogFooter>
            </>
          ) : (
            <>
              <DialogHeader>
                <DialogTitle>{t("deleteTitle")}</DialogTitle>
              </DialogHeader>
              <p className={cn(typeClass("body"), "text-muted-foreground")}>
                {t("deleteBody", { name: c.name })}
              </p>
              <DialogFooter>
                <DialogClose render={<Button variant="ghost">{t("cancel")}</Button>} />
                <Button variant="destructive" disabled={busy} onClick={remove}>
                  {t("delete")}
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>

      {shown.length === 0 ? (
        <EmptyState title={t("emptyTitle")} body={isAdmin ? t("emptyAdmin") : t("emptyUser")} />
      ) : (
        <CoverGrid>
          {shown.map((s, i) => (
            <div key={s.id} className="flex flex-col gap-2">
              <SeriesCard series={s} priority={i < 8} />
              {isAdmin && (
                <div className="flex items-center gap-1">
                  {c.ordered && (
                    <>
                      <Tooltip content={t("up")}>
                        <span className="inline-flex">
                          <Button
                            variant="ghost"
                            size="icon"
                            aria-label={t("up")}
                            disabled={busy || i === 0}
                            onClick={() => move(s.id, -1)}
                          >
                            <ArrowUp size={16} />
                          </Button>
                        </span>
                      </Tooltip>
                      <Tooltip content={t("down")}>
                        <span className="inline-flex">
                          <Button
                            variant="ghost"
                            size="icon"
                            aria-label={t("down")}
                            disabled={busy || i === shown.length - 1}
                            onClick={() => move(s.id, 1)}
                          >
                            <ArrowDown size={16} />
                          </Button>
                        </span>
                      </Tooltip>
                    </>
                  )}
                  <Tooltip content={t("removeFromCollection")}>
                    <span className="inline-flex">
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={t("removeFromCollection")}
                        disabled={busy}
                        onClick={() => removeFromCollection(s.id)}
                      >
                        <X size={16} />
                      </Button>
                    </span>
                  </Tooltip>
                </div>
              )}
            </div>
          ))}
        </CoverGrid>
      )}
    </Page>
  );
}
