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

/** The items in the order of ids; items whose id is not listed are dropped. */
function orderBy<T extends { id: string }>(xs: T[], ids: string[]): T[] {
  const byId = new Map(xs.map((x) => [x.id, x] as const));
  return ids.flatMap((id) => byId.get(id) ?? []);
}

export default function Screen() {
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
        <EmptyState title="コレクションが見つかりません" action={<Button variant="secondary" onClick={() => navigate("/collections")}>コレクションへ戻る</Button>} />
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
        toast.success("名前を変更しました");
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
        toast.success("削除しました");
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
          toast.error("ほかの画面で変更されました。読み込み直します");
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
    commit(api.reorderCollection(c.id, ids), "並び順を保存しました");
  };

  const removeFromCollection = (seriesId: string) => {
    commit(api.removeCollectionSeries(c.id, seriesId), "コレクションから外しました");
  };

  return (
    <Page>
      <PageHeader
        back={{ to: "/collections", label: "コレクション" }}
        title={c.name}
        meta={`シリーズ ${shown.length} · ${c.ordered ? "手動の並び順" : "自動の並び順"}`}
        actions={
          isAdmin && (
            <DropdownMenu>
              <Tooltip content="その他の操作">
                <span className="inline-flex">
                  <DropdownTrigger render={<Button variant="ghost" size="icon" aria-label="その他の操作" />}>
                    <MoreHorizontal size={16} />
                  </DropdownTrigger>
                </span>
              </Tooltip>
              <DropdownContent align="end">
                <MenuItem index={0} label="名前を変更" icon={SquarePen} onSelect={() => openDialog("rename")} />
                <MenuItem index={1} label="削除" onSelect={() => openDialog("delete")} />
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
                <DialogTitle>名前を変更</DialogTitle>
              </DialogHeader>
              <Field>
                <FieldLabel htmlFor="collection-name">名前</FieldLabel>
                <Input id="collection-name" value={name} onChange={(e) => setName(e.target.value)} />
              </Field>
              <DialogFooter>
                <DialogClose render={<Button variant="ghost">キャンセル</Button>} />
                <Button variant="primary" disabled={!name.trim() || busy} onClick={save}>
                  保存
                </Button>
              </DialogFooter>
            </>
          ) : (
            <>
              <DialogHeader>
                <DialogTitle>コレクションを削除</DialogTitle>
              </DialogHeader>
              <p className={cn(typeClass("body"), "text-muted-foreground")}>
                「{c.name}」を削除します。シリーズは削除されません。
              </p>
              <DialogFooter>
                <DialogClose render={<Button variant="ghost">キャンセル</Button>} />
                <Button variant="destructive" disabled={busy} onClick={remove}>
                  削除
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>

      {shown.length === 0 ? (
        <EmptyState
          title="シリーズがありません"
          body={isAdmin ? "シリーズの「その他」メニューの「コレクションに追加」から追加できます。" : "管理者がシリーズを追加すると、ここに表示されます"}
        />
      ) : (
        <CoverGrid>
          {shown.map((s, i) => (
            <div key={s.id} className="flex flex-col gap-2">
              <SeriesCard series={s} priority={i < 8} />
              {isAdmin && (
                <div className="flex items-center gap-1">
                  {c.ordered && (
                    <>
                      <Tooltip content="上へ">
                        <span className="inline-flex">
                          <Button
                            variant="ghost"
                            size="icon"
                            aria-label="上へ"
                            disabled={busy || i === 0}
                            onClick={() => move(s.id, -1)}
                          >
                            <ArrowUp size={16} />
                          </Button>
                        </span>
                      </Tooltip>
                      <Tooltip content="下へ">
                        <span className="inline-flex">
                          <Button
                            variant="ghost"
                            size="icon"
                            aria-label="下へ"
                            disabled={busy || i === shown.length - 1}
                            onClick={() => move(s.id, 1)}
                          >
                            <ArrowDown size={16} />
                          </Button>
                        </span>
                      </Tooltip>
                    </>
                  )}
                  <Tooltip content="コレクションから外す">
                    <span className="inline-flex">
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label="コレクションから外す"
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
