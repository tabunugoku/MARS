import { useState } from "react";
import { useNavigate, useParams } from "react-router";
import { toast } from "@yusufceng1z/ding-react";
import { MoreHorizontal, SquarePen } from "@keyline-icons/react";
import { api, useApi } from "@/api";
import { SeriesCard } from "@/components/media/cards";
import { CoverGrid, EmptyState, Page, PageHeader } from "@/components/media/layout";
import { Block, GridSkeleton } from "@/components/media/skeletons";
import { Button } from "@/components/ui/button";
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownContent, DropdownMenu, DropdownTrigger } from "@/components/ui/dropdown";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { MenuItem } from "@/components/ui/menu-item";
import { Tooltip } from "@/components/ui/tooltip";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";

export default function Screen() {
  const { collectionId = "" } = useParams();
  const navigate = useNavigate();
  const [dialog, setDialog] = useState<"rename" | "delete" | null>(null);
  const [name, setName] = useState("");

  const { data } = useApi(async () => {
    const c = await api.collection(collectionId);
    const series = c ? await api.seriesByIds(c.seriesIds) : [];
    return { c, series };
  }, [collectionId]);

  if (!data) {
    return (
      <Page>
        <Block className="mb-6 mt-2 h-8 w-56" />
        <GridSkeleton />
      </Page>
    );
  }

  const { c, series } = data;
  if (!c) {
    return (
      <Page>
        <EmptyState title="コレクションが見つかりません" action={<Button variant="secondary" onClick={() => navigate("/collections")}>コレクションへ戻る</Button>} />
      </Page>
    );
  }

  const openDialog = (kind: "rename" | "delete") => {
    setName(c.name);
    setDialog(kind);
  };

  return (
    <Page>
      <PageHeader
        back={{ to: "/collections", label: "コレクション" }}
        title={c.name}
        meta={`シリーズ ${c.seriesIds.length} · ${c.ordered ? "手動の並び順" : "自動の並び順"}`}
        actions={
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
                <Button
                  variant="primary"
                  disabled={!name.trim()}
                  onClick={() => {
                    toast.success("名前を変更しました");
                    setDialog(null);
                  }}
                >
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
                <Button
                  variant="destructive"
                  onClick={() => {
                    toast.success("削除しました");
                    navigate("/collections");
                  }}
                >
                  削除
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>

      {series.length === 0 ? (
        <EmptyState title="シリーズがありません" body="シリーズの詳細からこのコレクションに追加できます" />
      ) : (
        <CoverGrid>
          {series.map((s, i) => (
            <SeriesCard key={s.id} series={s} priority={i < 8} />
          ))}
        </CoverGrid>
      )}
    </Page>
  );
}
