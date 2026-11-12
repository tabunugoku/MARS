import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router";
import { api, errorMessage } from "@/api";
import { AppMark } from "@/components/brand/app-mark";
import { Field, FieldDescription, FieldError, FieldLabel } from "@/components/ui/field";
import Input from "@/components/ui/input";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";
import { defineMessages, useT } from "@/i18n";

const m = defineMessages({
  ja: {
    setupTitle: "初期設定",
    setupBody: "最初の管理者アカウントを作ります。",
    token: "セットアップトークン",
    tokenHint: "サーバーの起動時のログと DATA/setup-token に書かれています",
    name: "名前",
    email: "メールアドレス",
    password: "パスワード",
    passwordHint: "8 文字以上",
    passwordTooShort: "8 文字以上にしてください",
    confirmPassword: "パスワード（確認）",
    mismatch: "パスワードが一致しません",
    required: "入力してください",
    start: "はじめる",
  },
  en: {
    setupTitle: "First-time setup",
    setupBody: "This creates the first admin account.",
    token: "Setup token",
    tokenHint: "Shown in the server start-up log and in DATA/setup-token",
    name: "Name",
    email: "Email",
    password: "Password",
    passwordHint: "At least 8 characters",
    passwordTooShort: "Use at least 8 characters",
    confirmPassword: "Confirm password",
    mismatch: "Passwords do not match",
    required: "Required",
    start: "Get started",
  },
});

// The first-run form: no user exists yet. The server signs the new admin in
// on success, so the app goes straight to its start page.
export function SetupForm() {
  const t = useT(m);
  const navigate = useNavigate();
  const [token, setToken] = useState("");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [pass, setPass] = useState("");
  const [confirm, setConfirm] = useState("");
  const [errors, setErrors] = useState({ token: false, name: false, email: false, pass: false, confirm: false });
  const [serverError, setServerError] = useState<string | null>(null);
  const [sending, setSending] = useState(false);

  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const next = {
      token: !token.trim(),
      name: !name.trim(),
      email: !email.trim(),
      pass: pass.length < 8,
      confirm: pass !== confirm,
    };
    setErrors(next);
    setServerError(null);
    if (Object.values(next).some(Boolean)) return;
    setSending(true);
    try {
      await api.setup({ token: token.trim(), name: name.trim(), email: email.trim(), password: pass });
      navigate("/", { replace: true });
    } catch (err) {
      setServerError(errorMessage(err));
      setSending(false);
    }
  };

  return (
    <form onSubmit={submit} noValidate className="flex w-full max-w-[360px] flex-col gap-6">
      <div className="flex items-center gap-2">
        <AppMark size={40} className="-ml-1" />
        <span className={typeClass("title")} style={{ fontVariationSettings: fontWeights.semibold }}>
          ComiNavi
        </span>
      </div>

      <div className="flex flex-col gap-2">
        <h1 className={typeClass("display")} style={{ fontVariationSettings: fontWeights.bold }}>
          {t("setupTitle")}
        </h1>
        <p className={cn(typeClass("body"), "text-muted-foreground")}>{t("setupBody")}</p>
      </div>

      <div className="flex flex-col gap-4">
        <Field data-invalid={errors.token || undefined}>
          <FieldLabel htmlFor="setup-token">{t("token")}</FieldLabel>
          <Input
            className="pointer-coarse:h-11"
            id="setup-token"
            name="setupToken"
            autoComplete="off"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            aria-invalid={errors.token || undefined}
          />
          <FieldDescription>{t("tokenHint")}</FieldDescription>
          {errors.token && <FieldError>{t("required")}</FieldError>}
        </Field>

        <Field data-invalid={errors.name || undefined}>
          <FieldLabel htmlFor="setup-name">{t("name")}</FieldLabel>
          <Input
            className="pointer-coarse:h-11"
            id="setup-name"
            name="name"
            autoComplete="name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            aria-invalid={errors.name || undefined}
          />
          {errors.name && <FieldError>{t("required")}</FieldError>}
        </Field>

        <Field data-invalid={errors.email || undefined}>
          <FieldLabel htmlFor="setup-email">{t("email")}</FieldLabel>
          <Input
            className="pointer-coarse:h-11"
            id="setup-email"
            name="email"
            type="email"
            autoComplete="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            aria-invalid={errors.email || undefined}
          />
          {errors.email && <FieldError>{t("required")}</FieldError>}
        </Field>

        <Field data-invalid={errors.pass || undefined}>
          <FieldLabel htmlFor="setup-pass">{t("password")}</FieldLabel>
          <Input
            className="pointer-coarse:h-11"
            id="setup-pass"
            name="password"
            type="password"
            autoComplete="new-password"
            value={pass}
            onChange={(e) => setPass(e.target.value)}
            aria-invalid={errors.pass || undefined}
          />
          <FieldDescription>{t("passwordHint")}</FieldDescription>
          {errors.pass && <FieldError>{t("passwordTooShort")}</FieldError>}
        </Field>

        <Field data-invalid={errors.confirm || undefined}>
          <FieldLabel htmlFor="setup-confirm">{t("confirmPassword")}</FieldLabel>
          <Input
            className="pointer-coarse:h-11"
            id="setup-confirm"
            name="passwordConfirm"
            type="password"
            autoComplete="new-password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            aria-invalid={errors.confirm || undefined}
          />
          {errors.confirm && <FieldError>{t("mismatch")}</FieldError>}
        </Field>
      </div>

      {serverError && (
        <p role="alert" className={cn(typeClass("body"), "text-destructive")}>
          {serverError}
        </p>
      )}

      <button
        type="submit"
        disabled={sending}
        aria-busy={sending || undefined}
        className={cn(
          "inline-flex h-9 pointer-coarse:h-11 w-full items-center justify-center rounded-lg bg-brand px-4 text-brand-foreground",
          "transition-[filter,transform,opacity] duration-200 ease-[cubic-bezier(0.16,1,0.3,1)] hover:brightness-110 active:scale-[0.96] active:brightness-95 motion-reduce:transform-none",
          "outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)] focus-visible:ring-offset-2 focus-visible:ring-offset-background",
          "disabled:pointer-events-none disabled:opacity-60",
          typeClass("body"),
        )}
        style={{ fontVariationSettings: fontWeights.semibold }}
      >
        <span className="[text-box:trim-both_cap_alphabetic]">{t("start")}</span>
      </button>
    </form>
  );
}
