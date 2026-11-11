import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router";
import { AppMark } from "@/components/brand/app-mark";
import { Cover } from "@/components/media/cover";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import Input from "@/components/ui/input";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";

// Decoration only (aria-hidden): invented titles, so the wall reads as a
// shelf rather than numbered placeholders.
const WALL_TITLES = [
  "夜更けの測量士", "潮見坂ブックス", "白線の向こう側", "鉄と紙飛行機", "まどろみ天文台", "千年喫茶ほたる",
  "灯台守のレシピ", "雨音サーキット", "ひだまり探偵社", "砂時計の騎士", "月面ベーカリー", "青のアトリエ",
  "とおりゃんせ電車", "ことのは図書館", "星屑ガレージ", "風見鶏の約束", "遠雷マーチ", "硝子の庭師",
];
const WALL = Array.from({ length: 36 }, (_, i) => ({ seed: i, title: WALL_TITLES[i % WALL_TITLES.length] }));

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
      <div aria-hidden className="relative hidden md:block md:w-[55%] md:overflow-hidden">
        <div className="grid grid-cols-6 gap-3 p-6">
          {WALL.map((c) => (
            <Cover key={c.seed} seed={c.seed} title={c.title} />
          ))}
        </div>
        <div className="absolute inset-0 bg-background/70 tone" />
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
