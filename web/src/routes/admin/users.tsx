import { useState } from "react";
import { Download, Film } from "@keyline-icons/react";
import { api, useApi, type Library, type Role, type User } from "@/api";
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
import { TableSkeleton, relTime } from "./_parts";

const HEADS = ["名前", "メール", "役割", "権限", "使えるライブラリ", "最終アクセス"];

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

export default function Screen() {
  const { data } = useApi(() => Promise.all([api.users(), api.libraries()]), []);
  // Mock layer has no mutations: added users live here until the API exists.
  const [edited, setEdited] = useState<User[] | null>(null);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState<UserForm>(BLANK);

  const users = edited ?? data?.[0];
  const libs: Library[] = data?.[1] ?? [];
  const names = new Map(libs.map((l) => [l.id, l.name]));
  const set = (patch: Partial<UserForm>) => setForm((f) => ({ ...f, ...patch }));

  const togglePick = (id: string) =>
    set({ picked: form.picked.includes(id) ? form.picked.filter((x) => x !== id) : [...form.picked, id] });

  const save = () => {
    const user: User = {
      id: `u-${Date.now()}`,
      name: form.name.trim(),
      email: form.email.trim(),
      role: form.role,
      canDownload: form.canDownload,
      canStream: form.canStream,
      libraryIds: form.allLibs ? "all" : form.picked,
      lastActive: new Date().toISOString(),
    };
    setEdited([...(users ?? []), user]);
    setOpen(false);
    toast.success("ユーザーを追加しました");
  };

  const add = (
    <Button
      variant="primary"
      onClick={() => {
        setForm(BLANK);
        setOpen(true);
      }}
    >
      ユーザーを追加
    </Button>
  );

  return (
    <Page>
      <PageHeader title="ユーザー" actions={users && users.length > 0 ? add : undefined} />

      {!users && <TableSkeleton heads={HEADS} />}

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
              {users.map((u, i) => (
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
                    {u.libraryIds === "all" ? "すべて" : u.libraryIds.map((id) => names.get(id) ?? id).join("、")}
                  </TableCell>
                  <TableCell className="whitespace-nowrap">{relTime(u.lastActive)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent size="lg">
          <DialogHeader>
            <DialogTitle>ユーザーを追加</DialogTitle>
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
              <FieldLabel htmlFor="user-password">パスワード</FieldLabel>
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
            <Button
              variant="primary"
              disabled={!form.name.trim() || !form.email.trim() || !form.password || (!form.allLibs && form.picked.length === 0)}
              onClick={save}
            >
              追加
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Page>
  );
}
