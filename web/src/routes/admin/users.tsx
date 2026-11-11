import { useState } from "react";
import { Bin, Download, Film, SquarePen } from "@keyline-icons/react";
import { api, errorMessage, useApi, type Library, type Role, type User, type UserCreate, type UserUpdate } from "@/api";
import { LoadError } from "@/components/shell/load-error";
import { EmptyState, Page, PageHeader } from "@/components/media/layout";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { RadioGroup, RadioItem } from "@/components/ui/radio-group";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { toast } from "@yusufceng1z/ding-react";
import { fontWeights } from "@/lib/font-weight";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { ConfirmDialog, IconAction, TableSkeleton, relTime } from "./_parts";

const HEADS = ["名前", "メール", "役割", "権限", "使えるライブラリ", "最終アクセス", "操作"];

type UserForm = {
  name: string;
  email: string;
  password: string;
  role: Role;
  canDownload: boolean;
  canStream: boolean;
  allLibs: boolean;
  picked: string[];
};

const BLANK: UserForm = {
  name: "",
  email: "",
  password: "",
  role: "user",
  canDownload: true,
  canStream: true,
  allLibs: false,
  picked: [],
};

const sameSet = (a: string[], b: string[]) => {
  if (a.length !== b.length) return false;
  const sb = new Set(b);
  return a.every((x) => sb.has(x));
};

const toForm = (u: User): UserForm => ({
  name: u.name,
  email: u.email,
  password: "",
  role: u.role,
  canDownload: u.canDownload,
  canStream: u.canStream,
  allLibs: u.allLibraries,
  picked: u.allLibraries ? [] : u.libraryIds,
});

export default function Screen() {
  const { data, error, reload } = useApi(() => Promise.all([api.users(), api.libraries()]), []);
  // editing: the user being edited, or null when the dialog adds a new one.
  const [dlg, setDlg] = useState<{ open: boolean; editing: User | null }>({ open: false, editing: null });
  const [form, setForm] = useState<UserForm>(BLANK);
  const [saving, setSaving] = useState(false);
  const [removing, setRemoving] = useState<User | null>(null);
  const [removeOpen, setRemoveOpen] = useState(false);
  const [busyId, setBusyId] = useState<string | null>(null);

  const users = data?.[0];
  const libs: Library[] = data?.[1] ?? [];
  const names = new Map(libs.map((l) => [l.id, l.name]));
  const set = (patch: Partial<UserForm>) => setForm((f) => ({ ...f, ...patch }));

  const togglePick = (id: string) =>
    set({ picked: form.picked.includes(id) ? form.picked.filter((x) => x !== id) : [...form.picked, id] });

  const openAdd = () => {
    setForm(BLANK);
    setDlg({ open: true, editing: null });
  };
  const openEdit = (u: User) => {
    setForm(toForm(u));
    setDlg({ open: true, editing: u });
  };

  const save = async () => {
    const editing = dlg.editing;
    setSaving(true);
    try {
      if (editing) {
        const changes: UserUpdate = {};
        if (form.name.trim() !== editing.name) changes.name = form.name.trim();
        if (form.email.trim() !== editing.email) changes.email = form.email.trim();
        if (form.password) changes.password = form.password;
        if (form.role !== editing.role) changes.role = form.role;
        if (form.canDownload !== editing.canDownload) changes.canDownload = form.canDownload;
        if (form.canStream !== editing.canStream) changes.canStream = form.canStream;
        if (form.allLibs !== editing.allLibraries) changes.allLibraries = form.allLibs;
        if (!form.allLibs && !sameSet(form.picked, editing.libraryIds)) changes.libraryIds = form.picked;
        if (Object.keys(changes).length > 0) {
          await api.updateUser(editing.id, changes);
          toast.success("ユーザーを保存しました");
        }
      } else {
        const body: UserCreate = {
          name: form.name.trim(),
          email: form.email.trim(),
          password: form.password,
          role: form.role,
          canDownload: form.canDownload,
          canStream: form.canStream,
          allLibraries: form.allLibs,
          libraryIds: form.allLibs ? [] : form.picked,
        };
        await api.createUser(body);
        toast.success("ユーザーを追加しました");
      }
      setDlg({ open: false, editing: null });
      reload();
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const remove = async (u: User) => {
    setBusyId(u.id);
    try {
      await api.deleteUser(u.id);
      toast.success("ユーザーを削除しました");
      reload();
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusyId(null);
    }
  };

  const valid =
    form.name.trim() !== "" &&
    form.email.trim() !== "" &&
    (dlg.editing !== null || form.password !== "") &&
    (form.allLibs || form.picked.length > 0);

  const add = (
    <Button variant="primary" onClick={openAdd}>
      ユーザーを追加
    </Button>
  );

  return (
    <Page>
      <PageHeader title="ユーザー" actions={users && users.length > 0 ? add : undefined} />

      {error && !users && <LoadError message={error} onRetry={reload} />}

      {!error && !users && <TableSkeleton heads={HEADS} rows={3} />}

      {users && users.length === 0 && (
        <EmptyState title="ユーザーがいません" body="家族や友人を追加すると、それぞれの権限で読めるようになります" action={add} />
      )}

      {users && users.length > 0 && (
        <div className="overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                {HEADS.map((h) => (
                  <TableHead key={h}>{h}</TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {users.map((u, i) => {
                const busy = busyId === u.id;
                return (
                  <TableRow key={u.id} index={i}>
                    <TableCell>
                      <span className="whitespace-nowrap text-foreground" style={{ fontVariationSettings: fontWeights.semibold }}>
                        {u.name}
                      </span>
                    </TableCell>
                    <TableCell className="font-mono">{u.email}</TableCell>
                    <TableCell>
                      <Badge variant="solid">{u.role === "admin" ? "管理者" : "一般"}</Badge>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Download size={16} className={u.canDownload ? "text-foreground" : "text-muted-foreground"} aria-hidden />
                        <span className="sr-only">{u.canDownload ? "ダウンロード可" : "ダウンロード不可"}</span>
                        <Film size={16} className={u.canStream ? "text-foreground" : "text-muted-foreground"} aria-hidden />
                        <span className="sr-only">{u.canStream ? "ストリーミング可" : "ストリーミング不可"}</span>
                      </div>
                    </TableCell>
                    <TableCell className={cn(typeClass("caption"), "max-w-[260px] truncate")}>
                      {u.allLibraries ? "すべて" : u.libraryIds.map((id) => names.get(id) ?? id).join("、")}
                    </TableCell>
                    <TableCell className="whitespace-nowrap">{u.lastActive ? relTime(u.lastActive) : "なし"}</TableCell>
                    <TableCell>
                      <div className="flex justify-end gap-1">
                        <IconAction label="編集" icon={SquarePen} onClick={() => !busy && openEdit(u)} />
                        <IconAction
                          label="削除"
                          icon={Bin}
                          onClick={() => {
                            if (busy) return;
                            setRemoving(u);
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

      <Dialog open={dlg.open} onOpenChange={(o) => !saving && setDlg((d) => ({ ...d, open: o }))}>
        <DialogContent size="lg">
          <DialogHeader>
            <DialogTitle>{dlg.editing ? "ユーザーを編集" : "ユーザーを追加"}</DialogTitle>
          </DialogHeader>
          <FieldGroup>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor="user-name">名前</FieldLabel>
                <Input id="user-name" value={form.name} onChange={(e) => set({ name: e.target.value })} />
              </Field>
              <Field>
                <FieldLabel htmlFor="user-email">メール</FieldLabel>
                <Input id="user-email" type="email" value={form.email} onChange={(e) => set({ email: e.target.value })} />
              </Field>
            </div>
            <Field>
              <FieldLabel htmlFor="user-password">{dlg.editing ? "新しいパスワード（変更するときだけ）" : "パスワード"}</FieldLabel>
              <Input
                id="user-password"
                type="password"
                autoComplete="new-password"
                value={form.password}
                onChange={(e) => set({ password: e.target.value })}
              />
            </Field>
            <div className="flex flex-col gap-1.5">
              <span className={cn(typeClass("body"), "text-muted-foreground")}>役割</span>
              <RadioGroup value={form.role} onValueChange={(v) => set({ role: v as Role })}>
                <RadioItem index={0} value="admin" label="管理者" />
                <RadioItem index={1} value="user" label="一般" />
              </RadioGroup>
            </div>
            <div className="flex flex-col">
              <Switch
                label="ファイルのダウンロードを許可"
                checked={form.canDownload}
                onToggle={() => set({ canDownload: !form.canDownload })}
              />
              <Switch
                label="ページのストリーミングを許可"
                checked={form.canStream}
                onToggle={() => set({ canStream: !form.canStream })}
              />
            </div>
            <div role="group" aria-label="ライブラリ" className="flex flex-col gap-1.5">
              <span className={cn(typeClass("body"), "text-muted-foreground")}>ライブラリ</span>
              <label className={cn(typeClass("body"), "flex items-center gap-2 text-foreground")}>
                <Checkbox checked={form.allLibs} onCheckedChange={(c) => set({ allLibs: c })} />
                すべてのライブラリ
              </label>
              {libs.map((l) => (
                <label
                  key={l.id}
                  className={cn(
                    typeClass("body"),
                    "flex items-center gap-2",
                    form.allLibs ? "text-muted-foreground" : "text-foreground",
                  )}
                >
                  <Checkbox
                    disabled={form.allLibs}
                    checked={form.allLibs || form.picked.includes(l.id)}
                    onCheckedChange={() => togglePick(l.id)}
                  />
                  {l.name}
                </label>
              ))}
            </div>
          </FieldGroup>
          <DialogFooter>
            <DialogClose render={<Button variant="ghost">キャンセル</Button>} />
            <Button variant="primary" disabled={saving || !valid} onClick={save}>
              {dlg.editing ? "保存" : "追加"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={removeOpen}
        onOpenChange={setRemoveOpen}
        title="ユーザーを削除しますか"
        body={`「${removing?.name ?? ""}」を削除します。このユーザーはサインインできなくなります。`}
        confirmLabel="削除"
        onConfirm={() => {
          if (removing) void remove(removing);
        }}
      />
    </Page>
  );
}
