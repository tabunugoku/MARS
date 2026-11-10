import { useState } from "react";
import { api, useApi } from "@/api";
import { Page, PageHeader, SectionTitle } from "@/components/media/layout";
import { Block } from "@/components/media/skeletons";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { toast } from "@yusufceng1z/ding-react";

// The signed-in user's own account: display name, email (set by an admin)
// and password. The mock layer has no save endpoint, so saving only confirms.
export default function Screen() {
  const { data: me } = useApi(() => api.me(), []);
  const [name, setName] = useState<string | null>(null);
  const [pw, setPw] = useState({ current: "", next: "", confirm: "" });

  if (!me) {
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

  const shownName = name ?? me.name;
  const nameDirty = name !== null && name.trim() !== "" && name !== me.name;
  const mismatch = pw.confirm !== "" && pw.next !== pw.confirm;
  const pwReady = pw.current !== "" && pw.next.length >= 8 && pw.next === pw.confirm;

  return (
    <Page>
      <PageHeader title="アカウント" meta={me.role === "admin" ? "管理者" : "一般ユーザー"} />

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
              <Input id="acc-email" value={me.email} readOnly disabled />
              <FieldDescription>ログインに使います。変更は管理者に頼んでください。</FieldDescription>
            </Field>
            <div>
              <Button
                variant="secondary"
                disabled={!nameDirty}
                onClick={() => {
                  setName(null);
                  toast.success("表示名を保存しました");
                }}
              >
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
              <Button
                variant="secondary"
                disabled={!pwReady}
                onClick={() => {
                  setPw({ current: "", next: "", confirm: "" });
                  toast.success("パスワードを変更しました");
                }}
              >
                パスワードを変更
              </Button>
            </div>
          </FieldGroup>
        </section>
      </div>
    </Page>
  );
}
