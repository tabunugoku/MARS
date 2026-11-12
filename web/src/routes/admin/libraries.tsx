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
import { defineMessages, useT } from "@/i18n";
import { ConfirmDialog, IconAction, TableSkeleton, relTime } from "./_parts";

type Kind = Library["kind"];
type Interval = Library["scanInterval"];
type LibForm = {
  name: string;
  root: string;
  kind: Kind;
  scanInterval: Interval;
  importComicInfo: boolean;
  importLocalArtwork: boolean;
  watch: boolean;
  exclude: string;
};

const m = defineMessages({
  ja: {
    title: "ライブラリ設定",
    colName: "名前",
    colRoot: "ルートフォルダ",
    colSeries: "シリーズ",
    colBooks: "本",
    colSize: "容量",
    colLastScan: "最終スキャン",
    colActions: "操作",
    addLibrary: "ライブラリを追加",
    noLibraries: "ライブラリがありません",
    noLibrariesBody: "フォルダを追加すると、ここからスキャンできます",
    kindManga: "マンガ",
    kindWebtoon: "Webtoon",
    kindComics: "コミック",
    kindBooks: "書籍",
    intervalOff: "しない",
    intervalHourly: "1時間ごと",
    intervalDaily: "毎日",
    intervalWeekly: "毎週",
    saved: "ライブラリを保存しました",
    added: "ライブラリを追加しました",
    deleted: "ライブラリを削除しました",
    scanStarted: "スキャンを開始しました",
    deepScanStarted: "詳細スキャンを開始しました",
    regenerateStarted: "表紙の作り直しを開始しました",
    notScanned: "未スキャン",
    scan: "スキャン",
    edit: "編集",
    delete: "削除",
    editLibrary: "ライブラリを編集",
    name: "名前",
    folder: "フォルダ",
    rootLocked: "フォルダは作成後に変更できません",
    rootHint: "サーバー上の絶対パス。Docker ではマウント先のパス（例: /comics）",
    typeField: "種別",
    scanInterval: "スキャン間隔",
    readComicInfo: "ComicInfo.xml を読み込む",
    useLocalArtwork: "フォルダ内の表紙画像を使う",
    useLocalArtworkNote: "シリーズのフォルダにある cover・folder・poster という名前の画像を、シリーズの表紙にします。",
    watchFiles: "ファイルの変更を監視して自動でスキャンする",
    watchNote: "ネットワーク共有（SMB/NFS）のフォルダでは変更を検知できないことがあります。定期スキャンと併用してください。",
    exclude: "除外パターン",
    excludeHint: "1行に1パターン。例: @eaDir",
    maintenance: "メンテナンス",
    deepScan: "詳細スキャン",
    regenerate: "表紙を作り直す",
    deepScanNote: "詳細スキャンは、変更のないファイルも含めてすべての本を解析し直します。冊数が多いと時間がかかります",
    cancel: "キャンセル",
    save: "保存",
    deleteTitle: "ライブラリを削除しますか",
    deleteBody: "「{name}」の登録を外します。ファイルは削除されません。",
  },
  en: {
    title: "Library settings",
    colName: "Name",
    colRoot: "Root folder",
    colSeries: "Series",
    colBooks: "Books",
    colSize: "Size",
    colLastScan: "Last scan",
    colActions: "Actions",
    addLibrary: "Add library",
    noLibraries: "No libraries",
    noLibrariesBody: "Add a folder to scan it from here",
    kindManga: "Manga",
    kindWebtoon: "Webtoon",
    kindComics: "Comics",
    kindBooks: "Books",
    intervalOff: "Never",
    intervalHourly: "Hourly",
    intervalDaily: "Daily",
    intervalWeekly: "Weekly",
    saved: "Library saved",
    added: "Library added",
    deleted: "Library deleted",
    scanStarted: "Scan started",
    deepScanStarted: "Deep scan started",
    regenerateStarted: "Cover regeneration started",
    notScanned: "Not scanned",
    scan: "Scan",
    edit: "Edit",
    delete: "Delete",
    editLibrary: "Edit library",
    name: "Name",
    folder: "Folder",
    rootLocked: "The folder cannot be changed after creation",
    rootHint: "Absolute path on the server. In Docker, the mount path (for example /comics)",
    typeField: "Type",
    scanInterval: "Scan interval",
    readComicInfo: "Read ComicInfo.xml",
    useLocalArtwork: "Use cover images in series folders",
    useLocalArtworkNote: "cover, folder or poster images in a series folder become its cover",
    watchFiles: "Watch for file changes and scan automatically",
    watchNote: "Changes may not be detected on network shares (SMB/NFS). Use it together with scheduled scans.",
    exclude: "Exclude patterns",
    excludeHint: "One pattern per line. For example: @eaDir",
    maintenance: "Maintenance",
    deepScan: "Deep scan",
    regenerate: "Regenerate covers",
    deepScanNote: "Deep scan re-analyzes every book, including unchanged files. It takes longer with many books.",
    cancel: "Cancel",
    save: "Save",
    deleteTitle: "Delete this library?",
    deleteBody: "Removes \"{name}\" from the library list. Files are not deleted.",
  },
});

const BLANK: LibForm = {
  name: "",
  root: "",
  kind: "manga",
  scanInterval: "daily",
  importComicInfo: true,
  importLocalArtwork: true,
  watch: false,
  exclude: "",
};

const toForm = (l: Library): LibForm => ({
  name: l.name,
  root: l.root,
  kind: l.kind,
  scanInterval: l.scanInterval,
  importComicInfo: l.importComicInfo,
  importLocalArtwork: l.importLocalArtwork,
  watch: l.watch,
  exclude: l.exclude.join("\n"),
});

const parseExclude = (s: string) => s.split("\n").map((x) => x.trim()).filter(Boolean);
const sameList = (a: string[], b: string[]) => a.length === b.length && a.every((x, i) => x === b[i]);

export default function Screen() {
  const t = useT(m);
  const kindLabel: Record<Kind, string> = { manga: t("kindManga"), webtoon: t("kindWebtoon"), comics: t("kindComics"), books: t("kindBooks") };
  const intervalLabel: Record<Interval, string> = {
    off: t("intervalOff"),
    hourly: t("intervalHourly"),
    daily: t("intervalDaily"),
    weekly: t("intervalWeekly"),
  };
  // right: the numeric and action columns are right-aligned.
  const heads = [
    { label: t("colName"), right: false },
    { label: t("colRoot"), right: false },
    { label: t("colSeries"), right: true },
    { label: t("colBooks"), right: true },
    { label: t("colSize"), right: true },
    { label: t("colLastScan"), right: false },
    { label: t("colActions"), right: true },
  ];
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
          if (form.importLocalArtwork !== original.importLocalArtwork) changes.importLocalArtwork = form.importLocalArtwork;
          if (form.watch !== original.watch) changes.watch = form.watch;
          if (!sameList(exclude, original.exclude)) changes.exclude = exclude;
        }
        if (Object.keys(changes).length > 0) {
          await api.updateLibrary(dlg.id, changes);
          toast.success(t("saved"));
        }
      } else {
        const body: LibraryCreate = {
          name: form.name.trim(),
          root: form.root.trim(),
          kind: form.kind,
          scanInterval: form.scanInterval,
          importComicInfo: form.importComicInfo,
          importLocalArtwork: form.importLocalArtwork,
          watch: form.watch,
          exclude,
        };
        await api.createLibrary(body);
        toast.success(t("added"));
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
      toast.success(t("scanStarted"));
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusyId(null);
    }
  };

  // Maintenance from the edit dialog: both run as background tasks.
  const runTask = async (id: string, run: (id: string) => Promise<unknown>, done: string) => {
    setBusyId(id);
    try {
      await run(id);
      toast.success(done);
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
      toast.success(t("deleted"));
      reload();
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusyId(null);
    }
  };

  const add = (
    <Button variant="primary" onClick={openAdd}>
      {t("addLibrary")}
    </Button>
  );

  return (
    <Page>
      <PageHeader title={t("title")} actions={libs && libs.length > 0 ? add : undefined} />

      {error && !libs && <LoadError message={error} onRetry={reload} />}

      {!error && !libs && <TableSkeleton heads={heads.map((h) => h.label)} />}

      {libs && libs.length === 0 && (
        <EmptyState title={t("noLibraries")} body={t("noLibrariesBody")} action={add} />
      )}

      {libs && libs.length > 0 && (
        <div className="overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                {heads.map((h) => (
                  <TableHead key={h.label} className={cn("whitespace-nowrap", h.right && "text-right")}>
                    {h.label}
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
                      {l.watchError && (
                        <p className={cn(typeClass("caption"), "mt-1 min-w-[200px] max-w-[260px] text-muted-foreground")}>
                          {l.watchError /* the server sends the whole sentence, in the request's language */}
                        </p>
                      )}
                    </TableCell>
                    <TableCell className={cn(typeClass("caption"), "max-w-[260px] truncate font-mono")}>{l.root}</TableCell>
                    <TableCell className="text-right tabular-nums">{l.seriesCount}</TableCell>
                    <TableCell className="text-right tabular-nums">{l.bookCount}</TableCell>
                    <TableCell className="text-right tabular-nums whitespace-nowrap">{formatBytes(l.sizeBytes)}</TableCell>
                    <TableCell className="whitespace-nowrap">{l.lastScan ? relTime(l.lastScan) : t("notScanned")}</TableCell>
                    <TableCell>
                      <div className="flex justify-end gap-1">
                        <IconAction label={t("scan")} icon={RefreshCw} onClick={() => !busy && scan(l)} />
                        <IconAction label={t("edit")} icon={SquarePen} onClick={() => !busy && openEdit(l)} />
                        <IconAction
                          label={t("delete")}
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
            <DialogTitle>{dlg.id ? t("editLibrary") : t("addLibrary")}</DialogTitle>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="lib-name">{t("name")}</FieldLabel>
              <Input id="lib-name" value={form.name} onChange={(e) => set({ name: e.target.value })} />
            </Field>
            <Field>
              <FieldLabel htmlFor="lib-root">{t("folder")}</FieldLabel>
              <Input
                id="lib-root"
                className="font-mono"
                value={form.root}
                disabled={dlg.id !== null}
                onChange={(e) => set({ root: e.target.value })}
              />
              <FieldDescription>
                {dlg.id ? t("rootLocked") : t("rootHint")}
              </FieldDescription>
            </Field>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor="lib-kind">{t("typeField")}</FieldLabel>
                <Select value={form.kind} onValueChange={(v) => set({ kind: v as Kind })}>
                  <SelectTrigger id="lib-kind" className="w-full" />
                  <SelectContent>
                    {(Object.keys(kindLabel) as Kind[]).map((k, i) => (
                      <SelectItem key={k} index={i} value={k}>
                        {kindLabel[k]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <Field>
                <FieldLabel htmlFor="lib-interval">{t("scanInterval")}</FieldLabel>
                <Select value={form.scanInterval} onValueChange={(v) => set({ scanInterval: v as Interval })}>
                  <SelectTrigger id="lib-interval" className="w-full" />
                  <SelectContent>
                    {(Object.keys(intervalLabel) as Interval[]).map((k, i) => (
                      <SelectItem key={k} index={i} value={k}>
                        {intervalLabel[k]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            </div>
            <div className="flex flex-col">
              <Switch
                label={t("readComicInfo")}
                checked={form.importComicInfo}
                onToggle={() => set({ importComicInfo: !form.importComicInfo })}
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Switch
                label={t("useLocalArtwork")}
                checked={form.importLocalArtwork}
                onToggle={() => set({ importLocalArtwork: !form.importLocalArtwork })}
              />
              <FieldDescription>{t("useLocalArtworkNote")}</FieldDescription>
            </div>
            <div className="flex flex-col gap-1.5">
              <Switch label={t("watchFiles")} checked={form.watch} onToggle={() => set({ watch: !form.watch })} />
              <FieldDescription>
                {t("watchNote")}
              </FieldDescription>
            </div>
            <Field>
              <FieldLabel htmlFor="lib-exclude">{t("exclude")}</FieldLabel>
              <Textarea
                id="lib-exclude"
                rows={3}
                className="font-mono"
                value={form.exclude}
                onChange={(e) => set({ exclude: e.target.value })}
              />
              <FieldDescription>{t("excludeHint")}</FieldDescription>
            </Field>
            {dlg.id && (
              <Field>
                <FieldLabel>{t("maintenance")}</FieldLabel>
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant="secondary"
                    disabled={busyId === dlg.id}
                    onClick={() => dlg.id && runTask(dlg.id, (id) => api.scanLibrary(id, true), t("deepScanStarted"))}
                  >
                    {t("deepScan")}
                  </Button>
                  <Button
                    variant="secondary"
                    disabled={busyId === dlg.id}
                    onClick={() => dlg.id && runTask(dlg.id, api.regenerateLibraryThumbnails, t("regenerateStarted"))}
                  >
                    {t("regenerate")}
                  </Button>
                </div>
                <FieldDescription>
                  {t("deepScanNote")}
                </FieldDescription>
              </Field>
            )}
          </FieldGroup>
          <DialogFooter>
            <DialogClose render={<Button variant="ghost">{t("cancel")}</Button>} />
            <Button
              variant="primary"
              disabled={saving || !form.name.trim() || !form.root.trim()}
              onClick={save}
            >
              {t("save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={removeOpen}
        onOpenChange={setRemoveOpen}
        title={t("deleteTitle")}
        body={t("deleteBody", { name: removing?.name ?? "" })}
        confirmLabel={t("delete")}
        onConfirm={() => {
          if (removing) void remove(removing);
        }}
      />
    </Page>
  );
}
