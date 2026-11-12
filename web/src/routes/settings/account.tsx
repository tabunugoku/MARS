import { useState } from "react";
import { api, errorMessage, useApi, type User } from "@/api";
import { applyUserLocale, defineMessages, useT, type Locale } from "@/i18n";
import { Page, PageHeader, SectionTitle } from "@/components/media/layout";
import { Block } from "@/components/media/skeletons";
import { LoadError } from "@/components/shell/load-error";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { RadioGroup, RadioItem } from "@/components/ui/radio-group";
import { toast } from "@yusufceng1z/ding-react";

const m = defineMessages({
  ja: {
    title: "アカウント",
    roleAdmin: "管理者",
    roleUser: "一般ユーザー",
    profile: "プロフィール",
    displayName: "表示名",
    email: "メールアドレス",
    emailNote: "ログインに使います。変更は管理者に頼んでください。",
    save: "保存",
    nameSaved: "表示名を保存しました",
    password: "パスワード",
    currentPassword: "現在のパスワード",
    newPassword: "新しいパスワード",
    passwordHint: "8 文字以上",
    confirmPassword: "新しいパスワード（確認）",
    mismatch: "新しいパスワードと一致しません",
    changePassword: "パスワードを変更",
    passwordChanged: "パスワードを変更しました",
    language: "言語",
    displayLanguage: "表示言語",
    followBrowser: "ブラウザに合わせる",
    languageSaved: "言語を保存しました",
    nameJa: "日本語",
    nameEn: "English",
  },
  en: {
    title: "Account",
    roleAdmin: "Admin",
    roleUser: "Regular user",
    profile: "Profile",
    displayName: "Display name",
    email: "Email",
    emailNote: "Used to sign in. Ask an admin to change it.",
    save: "Save",
    nameSaved: "Display name saved",
    password: "Password",
    currentPassword: "Current password",
    newPassword: "New password",
    passwordHint: "At least 8 characters",
    confirmPassword: "New password (confirm)",
    mismatch: "Does not match the new password",
    changePassword: "Change password",
    passwordChanged: "Password changed",
    language: "Language",
    displayLanguage: "Display language",
    followBrowser: "Match browser",
    languageSaved: "Language saved",
    // Language names are shown in their own language.
    nameJa: "日本語",
    nameEn: "English",
  },
});


// The signed-in user's own account: display name, email (set by an admin)
// and password.
export default function Screen() {
  const t = useT(m);
  const { data: me, error, reload } = useApi(() => api.me(), []);
  // The user the server last returned from a save; it wins over the first load.
  const [saved, setSaved] = useState<User | null>(null);
  // The name being typed; null means "show the saved name".
  const [name, setName] = useState<string | null>(null);
  const [savingName, setSavingName] = useState(false);
  const [pw, setPw] = useState({ current: "", next: "", confirm: "" });
  const [savingPw, setSavingPw] = useState(false);
  // The language picked but not saved yet; null means "show the saved one".
  const [locale, setLocale] = useState<"" | Locale | null>(null);
  const [savingLocale, setSavingLocale] = useState(false);

  const current = saved ?? me;

  if (error) {
    return (
      <Page>
        <PageHeader title={t("title")} />
        <LoadError message={error} onRetry={reload} />
      </Page>
    );
  }

  if (!current) {
    return (
      <Page>
        <PageHeader title={t("title")} />
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
  const shownLocale = locale ?? current.locale;
  const localeDirty = locale !== null && locale !== current.locale;

  const saveName = async () => {
    setSavingName(true);
    try {
      const updated = await api.updateMe(trimmedName);
      setSaved(updated);
      setName(null);
      toast.success(t("nameSaved"));
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setSavingName(false);
    }
  };

  // The name goes as it is saved, not as typed: only the language changes here.
  const saveLocale = async () => {
    if (locale === null) return;
    setSavingLocale(true);
    try {
      const updated = await api.updateMe(current.name, locale);
      setSaved(updated);
      setLocale(null);
      applyUserLocale(updated.locale);
      toast.success(t("languageSaved"));
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setSavingLocale(false);
    }
  };

  const changePassword = async () => {
    setSavingPw(true);
    try {
      await api.changePassword(pw.current, pw.next);
      setPw({ current: "", next: "", confirm: "" });
      toast.success(t("passwordChanged"));
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setSavingPw(false);
    }
  };

  return (
    <Page>
      <PageHeader title={t("title")} meta={current.role === "admin" ? t("roleAdmin") : t("roleUser")} />

      <div className="max-w-[640px] space-y-10">
        <section>
          <SectionTitle>{t("profile")}</SectionTitle>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="acc-name">{t("displayName")}</FieldLabel>
              <Input id="acc-name" value={shownName} onChange={(e) => setName(e.target.value)} />
            </Field>
            <Field>
              <FieldLabel htmlFor="acc-email">{t("email")}</FieldLabel>
              <Input id="acc-email" value={current.email} readOnly disabled />
              <FieldDescription>{t("emailNote")}</FieldDescription>
            </Field>
            <div>
              <Button variant="secondary" disabled={!nameDirty || savingName} loading={savingName} onClick={saveName}>
                {t("save")}
              </Button>
            </div>
          </FieldGroup>
        </section>

        <section>
          <SectionTitle>{t("language")}</SectionTitle>
          <FieldGroup>
            <Field>
              <FieldLabel id="acc-locale">{t("displayLanguage")}</FieldLabel>
              <RadioGroup aria-labelledby="acc-locale" value={shownLocale} onValueChange={(v) => setLocale(v as "" | Locale)}>
                <RadioItem index={0} value="" label={t("followBrowser")} />
                <RadioItem index={1} value="ja" label={t("nameJa")} />
                <RadioItem index={2} value="en" label={t("nameEn")} />
              </RadioGroup>
            </Field>
            <div>
              <Button variant="secondary" disabled={!localeDirty || savingLocale} loading={savingLocale} onClick={saveLocale}>
                {t("save")}
              </Button>
            </div>
          </FieldGroup>
        </section>

        <section>
          <SectionTitle>{t("password")}</SectionTitle>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="acc-pw-current">{t("currentPassword")}</FieldLabel>
              <Input id="acc-pw-current" type="password" autoComplete="current-password" value={pw.current} onChange={(e) => setPw({ ...pw, current: e.target.value })} />
            </Field>
            <Field>
              <FieldLabel htmlFor="acc-pw-next">{t("newPassword")}</FieldLabel>
              <Input id="acc-pw-next" type="password" autoComplete="new-password" value={pw.next} onChange={(e) => setPw({ ...pw, next: e.target.value })} />
              <FieldDescription>{t("passwordHint")}</FieldDescription>
            </Field>
            <Field data-invalid={mismatch || undefined}>
              <FieldLabel htmlFor="acc-pw-confirm">{t("confirmPassword")}</FieldLabel>
              <Input
                id="acc-pw-confirm"
                type="password"
                autoComplete="new-password"
                aria-invalid={mismatch || undefined}
                value={pw.confirm}
                onChange={(e) => setPw({ ...pw, confirm: e.target.value })}
              />
              {mismatch && <FieldError>{t("mismatch")}</FieldError>}
            </Field>
            <div>
              <Button variant="secondary" disabled={!pwReady || savingPw} loading={savingPw} onClick={changePassword}>
                {t("changePassword")}
              </Button>
            </div>
          </FieldGroup>
        </section>
      </div>
    </Page>
  );
}
