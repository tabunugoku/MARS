import { useState } from "react";
import { Bin, Download, Film, SquarePen } from "@keyline-icons/react";
import { api, errorMessage, useApi, type Library, type Role, type User, type UserCreate, type UserUpdate } from "@/api";
import { defineMessages, useT } from "@/i18n";
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

const m = defineMessages({
  ja: {
    title: "ユーザー",
    colName: "名前",
    colEmail: "メール",
    colRole: "役割",
    colAccess: "権限",
    colLibraries: "使えるライブラリ",
    colLastActive: "最終アクセス",
    colActions: "操作",
    addUser: "ユーザーを追加",
    emptyTitle: "ユーザーがいません",
    emptyBody: "家族や友人を追加すると、それぞれの権限で読めるようになります",
    roleAdmin: "管理者",
    roleUser: "一般",
    canDownload: "ダウンロード可",
    cannotDownload: "ダウンロード不可",
    canStream: "ストリーミング可",
    cannotStream: "ストリーミング不可",
    all: "すべて",
    listSep: "、",
    none: "なし",
    edit: "編集",
    delete: "削除",
    editUser: "ユーザーを編集",
    name: "名前",
    email: "メール",
    password: "パスワード",
    newPassword: "新しいパスワード（変更するときだけ）",
    role: "役割",
    allowDownload: "ファイルのダウンロードを許可",
    allowStream: "ページのストリーミングを許可",
    libraries: "ライブラリ",
    allLibraries: "すべてのライブラリ",
    cancel: "キャンセル",
    save: "保存",
    add: "追加",
    userSaved: "ユーザーを保存しました",
    userAdded: "ユーザーを追加しました",
    userDeleted: "ユーザーを削除しました",
    deleteTitle: "ユーザーを削除しますか",
    deleteBody: "「{name}」を削除します。このユーザーはサインインできなくなります。",
  },
  en: {
    title: "Users",
    colName: "Name",
    colEmail: "Email",
    colRole: "Role",
    colAccess: "Access",
    colLibraries: "Libraries",
    colLastActive: "Last active",
    colActions: "Actions",
    addUser: "Add user",
    emptyTitle: "No users",
    emptyBody: "Add family or friends. Each can read with their own access.",
    roleAdmin: "Admin",
    roleUser: "Regular",
    canDownload: "Downloads allowed",
    cannotDownload: "Downloads not allowed",
    canStream: "Streaming allowed",
    cannotStream: "Streaming not allowed",
    all: "All",
    listSep: ", ",
    none: "None",
    edit: "Edit",
    delete: "Delete",
    editUser: "Edit user",
    name: "Name",
    email: "Email",
    password: "Password",
    newPassword: "New password (only to change it)",
    role: "Role",
    allowDownload: "Allow file downloads",
    allowStream: "Allow page streaming",
    libraries: "Libraries",
    allLibraries: "All libraries",
    cancel: "Cancel",
    save: "Save",
    add: "Add",
    userSaved: "User saved",
    userAdded: "User added",
    userDeleted: "User deleted",
    deleteTitle: "Delete user?",
    deleteBody: "\"{name}\" will be deleted. This user can no longer sign in.",
  },
});

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
  const t = useT(m);
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
  const heads = [t("colName"), t("colEmail"), t("colRole"), t("colAccess"), t("colLibraries"), t("colLastActive"), t("colActions")];

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
          toast.success(t("userSaved"));
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
        toast.success(t("userAdded"));
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
      toast.success(t("userDeleted"));
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
      {t("addUser")}
    </Button>
  );

  return (
    <Page>
      <PageHeader title={t("title")} actions={users && users.length > 0 ? add : undefined} />

      {error && !users && <LoadError message={error} onRetry={reload} />}

      {!error && !users && <TableSkeleton heads={heads} rows={3} />}

      {users && users.length === 0 && (
        <EmptyState title={t("emptyTitle")} body={t("emptyBody")} action={add} />
      )}

      {users && users.length > 0 && (
        <div className="overflow-x-auto overflow-y-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                {heads.map((h) => (
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
                      <Badge variant="solid">{u.role === "admin" ? t("roleAdmin") : t("roleUser")}</Badge>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Download size={16} className={u.canDownload ? "text-foreground" : "text-muted-foreground"} aria-hidden />
                        <span className="sr-only">{u.canDownload ? t("canDownload") : t("cannotDownload")}</span>
                        <Film size={16} className={u.canStream ? "text-foreground" : "text-muted-foreground"} aria-hidden />
                        <span className="sr-only">{u.canStream ? t("canStream") : t("cannotStream")}</span>
                      </div>
                    </TableCell>
                    <TableCell className={cn(typeClass("caption"), "max-w-[260px] truncate")}>
                      {u.allLibraries ? t("all") : u.libraryIds.map((id) => names.get(id) ?? id).join(t("listSep"))}
                    </TableCell>
                    <TableCell className="whitespace-nowrap">{u.lastActive ? relTime(u.lastActive) : t("none")}</TableCell>
                    <TableCell>
                      <div className="flex gap-1">
                        <IconAction label={t("edit")} icon={SquarePen} onClick={() => !busy && openEdit(u)} />
                        <IconAction
                          label={t("delete")}
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
            <DialogTitle>{dlg.editing ? t("editUser") : t("addUser")}</DialogTitle>
          </DialogHeader>
          <FieldGroup>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor="user-name">{t("name")}</FieldLabel>
                <Input id="user-name" value={form.name} onChange={(e) => set({ name: e.target.value })} />
              </Field>
              <Field>
                <FieldLabel htmlFor="user-email">{t("email")}</FieldLabel>
                <Input id="user-email" type="email" value={form.email} onChange={(e) => set({ email: e.target.value })} />
              </Field>
            </div>
            <Field>
              <FieldLabel htmlFor="user-password">{dlg.editing ? t("newPassword") : t("password")}</FieldLabel>
              <Input
                id="user-password"
                type="password"
                autoComplete="new-password"
                value={form.password}
                onChange={(e) => set({ password: e.target.value })}
              />
            </Field>
            <div className="flex flex-col gap-1.5">
              <span className={cn(typeClass("body"), "text-muted-foreground")}>{t("role")}</span>
              <RadioGroup value={form.role} onValueChange={(v) => set({ role: v as Role })}>
                <RadioItem index={0} value="admin" label={t("roleAdmin")} />
                <RadioItem index={1} value="user" label={t("roleUser")} />
              </RadioGroup>
            </div>
            <div className="flex flex-col">
              <Switch
                label={t("allowDownload")}
                checked={form.canDownload}
                onToggle={() => set({ canDownload: !form.canDownload })}
              />
              <Switch
                label={t("allowStream")}
                checked={form.canStream}
                onToggle={() => set({ canStream: !form.canStream })}
              />
            </div>
            <div role="group" aria-label={t("libraries")} className="flex flex-col gap-1.5">
              <span className={cn(typeClass("body"), "text-muted-foreground")}>{t("libraries")}</span>
              <label className={cn(typeClass("body"), "flex items-center gap-2 text-foreground")}>
                <Checkbox checked={form.allLibs} onCheckedChange={(c) => set({ allLibs: c })} />
                {t("allLibraries")}
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
            <DialogClose render={<Button variant="ghost">{t("cancel")}</Button>} />
            <Button variant="primary" disabled={saving || !valid} onClick={save}>
              {dlg.editing ? t("save") : t("add")}
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
