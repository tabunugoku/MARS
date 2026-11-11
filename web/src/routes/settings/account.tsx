import { useState } from "react";
import { api, errorMessage, useApi, type User } from "@/api";
import { Page, PageHeader, SectionTitle } from "@/components/media/layout";
import { Block } from "@/components/media/skeletons";
import { LoadError } from "@/components/shell/load-error";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { toast } from "@yusufceng1z/ding-react";

// The signed-in user's own account: display name, email (set by an admin)
// and password.
export default function Screen() {
  const { data: me, error, reload } = useApi(() => api.me(), []);
  // The user the server last returned from a save; it wins over the first load.
  const [saved, setSaved] = useState<User | null>(null);
  // The name being typed; null means "show the saved name".
  const [name, setName] = useState<string | null>(null);
  const [savingName, setSavingName] = useState(false);
  const [pw, setPw] = useState({ current: "", next: "", confirm: "" });
  const [savingPw, setSavingPw] = useState(false);

  const current = saved ?? me;

  if (error) {
    return (
      <Page>
        <PageHeader title="アカウント" />
        <LoadError message={error} onRetry={reload} />
      </Page>
    );
  }

  if (!current) {
    return (
      <Page>
        <PageHeader title="アカウント" />
        <div className="max-w-[640px] space-y-3" aria-hidden>
          <Block className="h-5 w-24" />
          <Block className="h-9 w-full" />
          <Block className="h-9 w-full" />
        </div>
      </Page>
    );
  }

  const shownName = name ?? current.name;
  const trimmedName = shownName.trim();
  const nameDirty = name !== null && trimmedName !== "" && trimmedName !== current.name;
  const mismatch = pw.confirm !== "" && pw.next !== pw.confirm;
  const pwReady = pw.current !== "" && pw.next.length >= 8 && pw.next === pw.confirm;

  const saveName = async () => {
    setSavingName(true);
    try {
      const updated = await api.updateMe(trimmedName);
      setSaved(updated);
      setName(null);
      toast.success("表示名を保存しました");
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setSavingName(false);
    }
  };

  const changePassword = async () => {
    setSavingPw(true);
    try {
      await api.changePassword(pw.current, pw.next);
      setPw({ current: "", next: "", confirm: "" });
      toast.success("パスワードを変更しました");
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setSavingPw(false);
    }
  };

  return (
    <Page>
      <PageHeader title="アカウント" meta={current.role === "admin" ? "管理者" : "一般ユーザー"} />

      <div className="max-w-[640px] space-y-10">
        <section>
          <SectionTitle>プロフィール</SectionTitle>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="acc-name">表示名</FieldLabel>
              <Input id="acc-name" value={shownName} onChange={(e) => setName(e.target.value)} />
            </Field>
            <Field>
              <FieldLabel htmlFor="acc-email">メールアドレス</FieldLabel>
              <Input id="acc-email" value={current.email} readOnly disabled />
              <FieldDescription>ログインに使います。変更は管理者に頼んでください。</FieldDescription>
            </Field>
            <div>
              <Button variant="secondary" disabled={!nameDirty || savingName} loading={savingName} onClick={saveName}>
                保存
              </Button>
            </div>
          </FieldGroup>
        </section>

        <section>
          <SectionTitle>パスワード</SectionTitle>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="acc-pw-current">現在のパスワード</FieldLabel>
              <Input id="acc-pw-current" type="password" autoComplete="current-password" value={pw.current} onChange={(e) => setPw({ ...pw, current: e.target.value })} />
            </Field>
            <Field>
              <FieldLabel htmlFor="acc-pw-next">新しいパスワード</FieldLabel>
              <Input id="acc-pw-next" type="password" autoComplete="new-password" value={pw.next} onChange={(e) => setPw({ ...pw, next: e.target.value })} />
              <FieldDescription>8 文字以上</FieldDescription>
            </Field>
            <Field data-invalid={mismatch || undefined}>
              <FieldLabel htmlFor="acc-pw-confirm">新しいパスワード（確認）</FieldLabel>
              <Input
                id="acc-pw-confirm"
                type="password"
                autoComplete="new-password"
                aria-invalid={mismatch || undefined}
                value={pw.confirm}
                onChange={(e) => setPw({ ...pw, confirm: e.target.value })}
              />
              {mismatch && <FieldError>新しいパスワードと一致しません</FieldError>}
            </Field>
            <div>
              <Button variant="secondary" disabled={!pwReady || savingPw} loading={savingPw} onClick={changePassword}>
                パスワードを変更
              </Button>
            </div>
          </FieldGroup>
        </section>
      </div>
    </Page>
  );
}
