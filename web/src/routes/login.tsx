import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router";
import { api, useApi, type LoginScreen } from "@/api";
import { AppMark } from "@/components/brand/app-mark";
import { CompassRose } from "@/components/brand/compass-rose";
import { Cover } from "@/components/media/cover";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import Input from "@/components/ui/input";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";

export default function Screen() {
  const navigate = useNavigate();
  const [user, setUser] = useState("");
  const [pass, setPass] = useState("");
  const [keep, setKeep] = useState(false);
  const [errors, setErrors] = useState({ user: false, pass: false });
  const { data: screen } = useApi(() => api.loginScreen(), []);

  const submit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const next = { user: !user.trim(), pass: !pass };
    setErrors(next);
    if (next.user || next.pass) return;
    navigate("/");
  };

  return (
    <div className="relative flex min-h-dvh overflow-hidden bg-background">
      <div aria-hidden className="relative hidden md:block md:w-[55%] md:overflow-hidden">
        <Backdrop screen={screen} />
        <div className="absolute inset-0 grid place-items-center">
          <AppMark size={96} />
        </div>
      </div>

      <main className="flex flex-1 flex-col items-center justify-center p-6">
        <form onSubmit={submit} noValidate className="flex w-full max-w-[360px] flex-col gap-6">
          <div className="flex items-center gap-2">
            <AppMark size={40} className="-ml-1" />
            <span className={typeClass("title")} style={{ fontVariationSettings: fontWeights.semibold }}>
              ComiNavi
            </span>
          </div>

          <h1 className={typeClass("display")} style={{ fontVariationSettings: fontWeights.bold }}>
            ログイン
          </h1>

          <div className="flex flex-col gap-4">
            <Field data-invalid={errors.user || undefined}>
              <FieldLabel htmlFor="login-user">ユーザー名</FieldLabel>
              <Input
                id="login-user"
                name="username"
                autoComplete="username"
                value={user}
                onChange={(e) => setUser(e.target.value)}
                aria-invalid={errors.user || undefined}
              />
              {errors.user && <FieldError>入力してください</FieldError>}
            </Field>

            <Field data-invalid={errors.pass || undefined}>
              <FieldLabel htmlFor="login-pass">パスワード</FieldLabel>
              <Input
                id="login-pass"
                name="password"
                type="password"
                autoComplete="current-password"
                value={pass}
                onChange={(e) => setPass(e.target.value)}
                aria-invalid={errors.pass || undefined}
              />
              {errors.pass && <FieldError>入力してください</FieldError>}
            </Field>

            <Field orientation="horizontal">
              <Checkbox id="login-keep" checked={keep} onCheckedChange={setKeep} />
              <FieldLabel htmlFor="login-keep">ログイン状態を保持する</FieldLabel>
            </Field>
          </div>

          <button
            type="submit"
            className={cn(
              "inline-flex h-9 pointer-coarse:h-11 w-full items-center justify-center rounded-lg bg-brand px-4 text-brand-foreground",
              "transition-[filter,transform] duration-200 ease-[cubic-bezier(0.16,1,0.3,1)] hover:brightness-110 active:scale-[0.96] active:brightness-95 motion-reduce:transform-none",
              "outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)] focus-visible:ring-offset-2 focus-visible:ring-offset-background",
              typeClass("body"),
            )}
            style={{ fontVariationSettings: fontWeights.semibold }}
          >
            <span className="[text-box:trim-both_cap_alphabetic]">ログイン</span>
          </button>
        </form>

        <p className={cn(typeClass("caption"), "absolute bottom-6 text-muted-foreground")}>ComiNavi v0.1.0</p>
      </main>
    </div>
  );
}

const WALL_SIZE = 42; // 7 rows of 6: enough to fill a tall panel

// Beside the form, chosen by an admin in サーバー設定 → ログイン画面.
// Until the choice arrives the panel stays plain, so neither flashes.
function Backdrop({ screen }: { screen: LoginScreen | undefined }) {
  if (!screen) return null;
  if (screen.backdrop === "covers") {
    // A small library repeats its covers so the wall still fills the panel.
    const wall = screen.covers.length ? Array.from({ length: WALL_SIZE }, (_, i) => screen.covers[i % screen.covers.length]) : [];
    return (
      <>
        <div className="grid grid-cols-6 gap-3 p-6">
          {wall.map((c, i) => (
            <Cover key={i} seed={c.seed} title={c.title} />
          ))}
        </div>
        <div className="absolute inset-0 bg-background/70 tone" />
      </>
    );
  }
  // The night chart: the same rose as behind every screen, centred under the
  // mark. Its needle swings in once to the mark's own 35° heading.
  return (
    <>
      <div className="tone absolute inset-0 opacity-70 [mask-image:radial-gradient(closest-side,#000,transparent)]" />
      <CompassRose
        deg={35}
        from={-90}
        className="absolute left-1/2 top-1/2 size-[min(88%,680px)] -translate-x-1/2 -translate-y-1/2 opacity-[0.16] dark:opacity-[0.2]"
      />
    </>
  );
}
