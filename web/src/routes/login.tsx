import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router";
import { BookOpen } from "@keyline-icons/react";
import { Cover } from "@/components/media/cover";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import Input from "@/components/ui/input";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";

const WALL = Array.from({ length: 36 }, (_, i) => i);

export default function Screen() {
  const navigate = useNavigate();
  const [user, setUser] = useState("");
  const [pass, setPass] = useState("");
  const [keep, setKeep] = useState(false);
  const [errors, setErrors] = useState({ user: false, pass: false });

  const submit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const next = { user: !user.trim(), pass: !pass };
    setErrors(next);
    if (next.user || next.pass) return;
    navigate("/");
  };

  return (
    <div className="relative flex min-h-dvh overflow-hidden bg-background">
      <div aria-hidden className="hidden md:block md:w-[55%] md:overflow-hidden">
        <div className="grid grid-cols-6 gap-3 p-6 opacity-40">
          {WALL.map((i) => (
            <Cover key={i} seed={i} title={String(i)} />
          ))}
        </div>
      </div>

      <main className="flex flex-1 flex-col items-center justify-center p-6">
        <form onSubmit={submit} noValidate className="flex w-full max-w-[360px] flex-col gap-6">
          <div className="flex items-center gap-2">
            <span className="grid size-8 place-items-center rounded-[8px] bg-brand text-brand-foreground">
              <BookOpen size={16} aria-hidden />
            </span>
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
              "inline-flex h-9 w-full items-center justify-center rounded-lg bg-brand px-4 text-brand-foreground",
              "transition-[filter,box-shadow] duration-80 hover:brightness-110 active:shadow-[inset_0_0_0_1px_rgb(0_0_0/0.15)] active:brightness-95",
              "outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)] focus-visible:ring-offset-2 focus-visible:ring-offset-background",
              typeClass("body"),
            )}
            style={{ fontVariationSettings: fontWeights.semibold }}
          >
            ログイン
          </button>
        </form>

        <p className={cn(typeClass("caption"), "absolute bottom-6 text-muted-foreground")}>ComiNavi v0.1.0</p>
      </main>
    </div>
  );
}
