import { useState } from "react";
import { RefreshCw, SquarePen, Bin } from "@keyline-icons/react";
import { api, formatBytes, useApi, type Library } from "@/api";
import { KindIcon } from "@/components/media/kind-icon";
import { EmptyState, Page, PageHeader } from "@/components/media/layout";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { toast } from "@yusufceng1z/ding-react";
import { fontWeights } from "@/lib/font-weight";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { ConfirmDialog, IconAction, TableSkeleton, relTime } from "./_parts";

type Kind = Library["kind"];
type Interval = Library["scanInterval"];
type LibForm = {
  name: string;
  root: string;
  kind: Kind;
  scanInterval: Interval;
  watch: boolean;
  importComicInfo: boolean;
  importLocalArtwork: boolean;
  exclude: string;
};

const KIND_LABEL: Record<Kind, string> = { manga: "マンガ", webtoon: "Webtoon", comics: "コミック", books: "書籍" };
const INTERVAL_LABEL: Record<Interval, string> = { off: "しない", hourly: "1時間ごと", daily: "毎日", weekly: "毎週" };
const HEADS = ["名前", "ルートフォルダ", "シリーズ", "本", "容量", "最終スキャン", "操作"];
const RIGHT = ["シリーズ", "本", "容量", "操作"];

const BLANK: LibForm = {
  name: "",
  root: "",
  kind: "manga",
  scanInterval: "daily",
  watch: true,
  importComicInfo: true,
  importLocalArtwork: true,
  exclude: "",
};

const toForm = (l: Library): LibForm => ({ ...l, exclude: l.exclude.join("\n") });

export default function Screen() {
  const { data } = useApi(() => api.libraries(), []);
  // Mock layer has no mutations: edits live here until the API exists.
  const [edited, setEdited] = useState<Library[] | null>(null);
  const libs = edited ?? data;
  const [dlg, setDlg] = useState<{ open: boolean; id: string | null }>({ open: false, id: null });
  const [form, setForm] = useState<LibForm>(BLANK);
  const [removing, setRemoving] = useState<Library | null>(null);
  const [removeOpen, setRemoveOpen] = useState(false);

  const set = (patch: Partial<LibForm>) => setForm((f) => ({ ...f, ...patch }));
  const openAdd = () => {
    setForm(BLANK);
    setDlg({ open: true, id: null });
  };
  const openEdit = (l: Library) => {
    setForm(toForm(l));
    setDlg({ open: true, id: l.id });
  };

  const save = () => {
    const exclude = form.exclude.split("\n").map((s) => s.trim()).filter(Boolean);
    const list = libs ?? [];
    if (dlg.id) {
      setEdited(list.map((l) => (l.id === dlg.id ? { ...l, ...form, exclude } : l)));
      toast.success("ライブラリを保存しました");
    } else {
      const lib: Library = {
        ...form,
        exclude,
        id: `lib-${Date.now()}`,
        seriesCount: 0,
        bookCount: 0,
        sizeBytes: 0,
        lastScan: new Date().toISOString(),
      };
      setEdited([...list, lib]);
      toast.success("ライブラリを追加しました");
    }
    setDlg({ open: false, id: null });
  };

  const add = (
    <Button variant="primary" onClick={openAdd}>
      ライブラリを追加
    </Button>
  );

  return (
    <Page>
      <PageHeader title="ライブラリ設定" actions={libs && libs.length > 0 ? add : undefined} />

      {!libs && <TableSkeleton heads={HEADS} />}

      {libs && libs.length === 0 && (
        <EmptyState title="ライブラリがありません" body="フォルダを追加すると、ここからスキャンできます" action={add} />
      )}

      {libs && libs.length > 0 && (
        <div className="overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                {HEADS.map((h) => (
                  <TableHead key={h} className={RIGHT.includes(h) ? "text-right" : undefined}>
                    {h}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {libs.map((l, i) => {
                return (
                  <TableRow key={l.id} index={i}>
                    <TableCell>
                      <div className="flex items-center gap-2 whitespace-nowrap">
                        <KindIcon kind={l.kind} size={28} />
                        <span className="text-foreground" style={{ fontVariationSettings: fontWeights.semibold }}>
                          {l.name}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className={cn(typeClass("caption"), "max-w-[260px] truncate font-mono")}>{l.root}</TableCell>
                    <TableCell className="text-right tabular-nums">{l.seriesCount}</TableCell>
                    <TableCell className="text-right tabular-nums">{l.bookCount}</TableCell>
                    <TableCell className="text-right tabular-nums whitespace-nowrap">{formatBytes(l.sizeBytes)}</TableCell>
                    <TableCell className="whitespace-nowrap">{relTime(l.lastScan)}</TableCell>
                    <TableCell>
                      <div className="flex justify-end gap-1">
                        <IconAction label="スキャン" icon={RefreshCw} onClick={() => toast.success("スキャンを開始しました")} />
                        <IconAction label="編集" icon={SquarePen} onClick={() => openEdit(l)} />
                        <IconAction label="削除" icon={Bin} onClick={() => { setRemoving(l); setRemoveOpen(true); }} />
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      )}

      <Dialog open={dlg.open} onOpenChange={(open) => setDlg((d) => ({ ...d, open }))}>
        <DialogContent size="lg">
          <DialogHeader>
            <DialogTitle>{dlg.id ? "ライブラリを編集" : "ライブラリを追加"}</DialogTitle>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="lib-name">名前</FieldLabel>
              <Input id="lib-name" value={form.name} onChange={(e) => set({ name: e.target.value })} />
            </Field>
            <Field>
              <FieldLabel htmlFor="lib-root">フォルダ</FieldLabel>
              <div className="flex gap-2">
                <Input id="lib-root" className="font-mono" value={form.root} onChange={(e) => set({ root: e.target.value })} />
                <Button variant="tertiary" type="button">
                  参照
                </Button>
              </div>
            </Field>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor="lib-kind">種別</FieldLabel>
                <Select value={form.kind} onValueChange={(v) => set({ kind: v as Kind })}>
                  <SelectTrigger id="lib-kind" className="w-full" />
                  <SelectContent>
                    {(Object.keys(KIND_LABEL) as Kind[]).map((k, i) => (
                      <SelectItem key={k} index={i} value={k}>
                        {KIND_LABEL[k]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <Field>
                <FieldLabel htmlFor="lib-interval">スキャン間隔</FieldLabel>
                <Select value={form.scanInterval} onValueChange={(v) => set({ scanInterval: v as Interval })}>
                  <SelectTrigger id="lib-interval" className="w-full" />
                  <SelectContent>
                    {(Object.keys(INTERVAL_LABEL) as Interval[]).map((k, i) => (
                      <SelectItem key={k} index={i} value={k}>
                        {INTERVAL_LABEL[k]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            </div>
            <div className="flex flex-col">
              <Switch label="ファイルの変更を監視する" checked={form.watch} onToggle={() => set({ watch: !form.watch })} />
              <Switch
                label="ComicInfo.xml を読み込む"
                checked={form.importComicInfo}
                onToggle={() => set({ importComicInfo: !form.importComicInfo })}
              />
              <Switch
                label="フォルダ内の表紙画像を使う"
                checked={form.importLocalArtwork}
                onToggle={() => set({ importLocalArtwork: !form.importLocalArtwork })}
              />
            </div>
            <Field>
              <FieldLabel htmlFor="lib-exclude">除外パターン</FieldLabel>
              <Textarea
                id="lib-exclude"
                rows={3}
                className="font-mono"
                value={form.exclude}
                onChange={(e) => set({ exclude: e.target.value })}
              />
              <FieldDescription>1行に1パターン。例: @eaDir</FieldDescription>
            </Field>
          </FieldGroup>
          <DialogFooter>
            <DialogClose render={<Button variant="ghost">キャンセル</Button>} />
            <Button variant="primary" disabled={!form.name.trim() || !form.root.trim()} onClick={save}>
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={removeOpen}
        onOpenChange={setRemoveOpen}
        title="ライブラリを削除しますか"
        body={`「${removing?.name ?? ""}」の登録を外します。ファイルは削除されません。`}
        confirmLabel="削除"
        onConfirm={() => {
          if (removing && libs) setEdited(libs.filter((l) => l.id !== removing.id));
          toast.success("ライブラリを削除しました");
        }}
      />
    </Page>
  );
}
