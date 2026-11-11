import { useState } from "react";
import { RefreshCw, SquarePen, Bin } from "@keyline-icons/react";
import { api, errorMessage, formatBytes, useApi, type Library, type LibraryCreate, type LibraryUpdate } from "@/api";
import { LoadError } from "@/components/shell/load-error";
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
  importComicInfo: boolean;
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
  importComicInfo: true,
  exclude: "",
};

const toForm = (l: Library): LibForm => ({
  name: l.name,
  root: l.root,
  kind: l.kind,
  scanInterval: l.scanInterval,
  importComicInfo: l.importComicInfo,
  exclude: l.exclude.join("\n"),
});

const parseExclude = (s: string) => s.split("\n").map((x) => x.trim()).filter(Boolean);
const sameList = (a: string[], b: string[]) => a.length === b.length && a.every((x, i) => x === b[i]);

export default function Screen() {
  const { data: libs, error, reload } = useApi(() => api.libraries(), []);
  const [dlg, setDlg] = useState<{ open: boolean; id: string | null }>({ open: false, id: null });
  const [form, setForm] = useState<LibForm>(BLANK);
  const [saving, setSaving] = useState(false);
  const [busyId, setBusyId] = useState<string | null>(null);
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

  const save = async () => {
    const exclude = parseExclude(form.exclude);
    setSaving(true);
    try {
      if (dlg.id) {
        const original = libs?.find((l) => l.id === dlg.id);
        const changes: LibraryUpdate = {};
        if (original) {
          if (form.name.trim() !== original.name) changes.name = form.name.trim();
          if (form.kind !== original.kind) changes.kind = form.kind;
          if (form.scanInterval !== original.scanInterval) changes.scanInterval = form.scanInterval;
          if (form.importComicInfo !== original.importComicInfo) changes.importComicInfo = form.importComicInfo;
          if (!sameList(exclude, original.exclude)) changes.exclude = exclude;
        }
        if (Object.keys(changes).length > 0) {
          await api.updateLibrary(dlg.id, changes);
          toast.success("ライブラリを保存しました");
        }
      } else {
        const body: LibraryCreate = {
          name: form.name.trim(),
          root: form.root.trim(),
          kind: form.kind,
          scanInterval: form.scanInterval,
          importComicInfo: form.importComicInfo,
          exclude,
        };
        await api.createLibrary(body);
        toast.success("ライブラリを追加しました");
      }
      setDlg({ open: false, id: null });
      reload();
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const scan = async (l: Library) => {
    setBusyId(l.id);
    try {
      await api.scanLibrary(l.id);
      toast.success("スキャンを開始しました");
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusyId(null);
    }
  };

  const remove = async (l: Library) => {
    setBusyId(l.id);
    try {
      await api.deleteLibrary(l.id);
      toast.success("ライブラリを削除しました");
      reload();
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusyId(null);
    }
  };

  const add = (
    <Button variant="primary" onClick={openAdd}>
      ライブラリを追加
    </Button>
  );

  return (
    <Page>
      <PageHeader title="ライブラリ設定" actions={libs && libs.length > 0 ? add : undefined} />

      {error && !libs && <LoadError message={error} onRetry={reload} />}

      {!error && !libs && <TableSkeleton heads={HEADS} />}

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
                const busy = busyId === l.id;
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
                    <TableCell className="whitespace-nowrap">{l.lastScan ? relTime(l.lastScan) : "未スキャン"}</TableCell>
                    <TableCell>
                      <div className="flex justify-end gap-1">
                        <IconAction label="スキャン" icon={RefreshCw} onClick={() => !busy && scan(l)} />
                        <IconAction label="編集" icon={SquarePen} onClick={() => !busy && openEdit(l)} />
                        <IconAction
                          label="削除"
                          icon={Bin}
                          onClick={() => {
                            if (busy) return;
                            setRemoving(l);
                            setRemoveOpen(true);
                          }}
                        />
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      )}

      <Dialog open={dlg.open} onOpenChange={(open) => !saving && setDlg((d) => ({ ...d, open }))}>
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
              <Input
                id="lib-root"
                className="font-mono"
                value={form.root}
                disabled={dlg.id !== null}
                onChange={(e) => set({ root: e.target.value })}
              />
              <FieldDescription>
                {dlg.id ? "フォルダは作成後に変更できません" : "サーバー上の絶対パス。例: /data/comics"}
              </FieldDescription>
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
              <Switch
                label="ComicInfo.xml を読み込む"
                checked={form.importComicInfo}
                onToggle={() => set({ importComicInfo: !form.importComicInfo })}
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
            <Button
              variant="primary"
              disabled={saving || !form.name.trim() || !form.root.trim()}
              onClick={save}
            >
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
          if (removing) void remove(removing);
        }}
      />
    </Page>
  );
}
