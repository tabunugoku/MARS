import { useState } from "react";
import { Copy, Eye, EyeOff } from "@keyline-icons/react";
import { api, useApi, type ServerSettings } from "@/api";
import { Block } from "@/components/media/skeletons";
import { Page, PageHeader, SectionTitle } from "@/components/media/layout";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { RadioGroup, RadioItem } from "@/components/ui/radio-group";
import { Select, SelectContent, SelectItem, SelectTrigger } from "@/components/ui/select";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { toast } from "@yusufceng1z/ding-react";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { ConfirmDialog, IconAction } from "./_parts";

const SIZE_LABEL: Record<ServerSettings["thumbnailSize"], string> = { small: "小", medium: "中", large: "大", xlarge: "特大" };
const FORMAT_LABEL: Record<ServerSettings["thumbnailFormat"], string> = { webp: "WebP", jpeg: "JPEG", avif: "AVIF" };
const KEY_MASK = "••••••••••••••••••••";
const BACKDROP_LABEL: Record<ServerSettings["loginBackdrop"], string> = { compass: "羅針盤", covers: "表紙の壁" };
const BACKDROP_NOTE: Record<ServerSettings["loginBackdrop"], string> = {
  compass: "ログイン前の画面には ComiNavi の羅針盤だけが表示されます",
  covers: "最近追加したシリーズの表紙を並べます。ログインしていない人にも表紙が見えます",
};

export default function Screen() {
  const { data } = useApi(() => api.settings(), []);
  // Mock layer has no save endpoint: the saved copy lives here until the API exists.
  const [saved, setSaved] = useState<ServerSettings | null>(null);
  const [draft, setDraft] = useState<ServerSettings | null>(null);
  const [apiKey, setApiKey] = useState("cmk_7f3a9c1e2b84d05a6f19");
  const [revealKey, setRevealKey] = useState(false);
  const [rotating, setRotating] = useState(false);

  const base = saved ?? data;
  const cur = draft ?? base;
  const dirty = draft !== null && base !== undefined && JSON.stringify(draft) !== JSON.stringify(base);

  if (!cur) {
    return (
      <Page>
        <PageHeader title="サーバー設定" />
        <div className="max-w-[640px] space-y-3" aria-hidden>
          <Block className="h-5 w-24" />
          <Block className="h-9 w-full" />
          <Block className="h-9 w-full" />
        </div>
      </Page>
    );
  }

  const patch = (p: Partial<ServerSettings>) => setDraft({ ...cur, ...p });
  const save = async () => {
    if (!draft || !base) return;
    const changes = Object.fromEntries(
      Object.entries(draft).filter(([k, v]) => base[k as keyof ServerSettings] !== v),
    ) as Partial<ServerSettings>;
    try {
      setSaved(await api.saveSettings(changes));
      setDraft(null);
      toast.success("設定を保存しました");
    } catch {
      // Keep the draft so nothing typed is lost.
      toast.error("設定を保存できませんでした");
    }
  };

  return (
    <Page>
      <PageHeader title="サーバー設定" />

      <div className="max-w-[640px] space-y-10">
        <section>
          <SectionTitle>一般</SectionTitle>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="set-port">ポート</FieldLabel>
              <Input
                id="set-port"
                type="number"
                min={1}
                max={65535}
                className="tabular-nums"
                value={cur.port}
                onChange={(e) => patch({ port: Number(e.target.value) })}
              />
            </Field>
            <Slider
              value={cur.taskThreads}
              onChange={(v) => patch({ taskThreads: v as number })}
              min={1}
              max={8}
              step={1}
              label="タスクの同時実行数"
              formatValue={(v) => `${v}`}
            />
          </FieldGroup>
        </section>

        <section>
          <SectionTitle>サムネイル</SectionTitle>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="set-thumb-size">サイズ</FieldLabel>
              <Select value={cur.thumbnailSize} onValueChange={(v) => patch({ thumbnailSize: v as ServerSettings["thumbnailSize"] })}>
                <SelectTrigger id="set-thumb-size" className="w-full" />
                <SelectContent>
                  {(Object.keys(SIZE_LABEL) as ServerSettings["thumbnailSize"][]).map((k, i) => (
                    <SelectItem key={k} index={i} value={k}>
                      {SIZE_LABEL[k]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field>
              <FieldLabel id="set-thumb-format">形式</FieldLabel>
              <RadioGroup
                aria-labelledby="set-thumb-format"
                value={cur.thumbnailFormat}
                onValueChange={(v) => patch({ thumbnailFormat: v as ServerSettings["thumbnailFormat"] })}
              >
                {(Object.keys(FORMAT_LABEL) as ServerSettings["thumbnailFormat"][]).map((k, i) => (
                  <RadioItem key={k} index={i} value={k} label={FORMAT_LABEL[k]} />
                ))}
              </RadioGroup>
            </Field>
            <div>
              <Button variant="tertiary" onClick={() => toast.success("サムネイルの再生成を開始しました")}>
                サムネイルを再生成
              </Button>
            </div>
          </FieldGroup>
        </section>

        <section>
          <SectionTitle>認証</SectionTitle>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="set-remember">ログイン保持の日数</FieldLabel>
              <Input
                id="set-remember"
                type="number"
                min={1}
                className="tabular-nums"
                value={cur.rememberMeDays}
                onChange={(e) => patch({ rememberMeDays: Number(e.target.value) })}
              />
            </Field>
            <Field>
              <FieldLabel id="set-login-backdrop">ログイン画面</FieldLabel>
              <RadioGroup
                aria-labelledby="set-login-backdrop"
                aria-describedby="set-login-backdrop-note"
                value={cur.loginBackdrop}
                onValueChange={(v) => patch({ loginBackdrop: v as ServerSettings["loginBackdrop"] })}
              >
                {(Object.keys(BACKDROP_LABEL) as ServerSettings["loginBackdrop"][]).map((k, i) => (
                  <RadioItem key={k} index={i} value={k} label={BACKDROP_LABEL[k]} />
                ))}
              </RadioGroup>
              <FieldDescription id="set-login-backdrop-note">{BACKDROP_NOTE[cur.loginBackdrop]}</FieldDescription>
            </Field>
          </FieldGroup>
        </section>

        <section>
          <SectionTitle>連携</SectionTitle>
          <FieldGroup>
            <Switch label="OPDS を有効にする" checked={cur.opdsEnabled} onToggle={() => patch({ opdsEnabled: !cur.opdsEnabled })} />
            {cur.opdsEnabled && (
              <div className="rounded-lg bg-card p-3">
                <p className={cn(typeClass("caption"), "text-muted-foreground")}>OPDS カタログの URL</p>
                <p className={cn(typeClass("body"), "break-all font-mono text-foreground")}>
                  {`http://<host>:${cur.port}/opds/v1.2/catalog`}
                </p>
              </div>
            )}
            <Switch label="Kobo 同期を有効にする" checked={cur.koboSyncEnabled} onToggle={() => patch({ koboSyncEnabled: !cur.koboSyncEnabled })} />
            <Field>
              <FieldLabel htmlFor="set-api-key">API キー</FieldLabel>
              <div className="flex items-center gap-1">
                <Input
                  id="set-api-key"
                  readOnly
                  className="font-mono"
                  value={revealKey ? apiKey : KEY_MASK}
                />
                <IconAction
                  label={revealKey ? "隠す" : "表示"}
                  icon={revealKey ? EyeOff : Eye}
                  onClick={() => setRevealKey((r) => !r)}
                />
                <IconAction
                  label="コピー"
                  icon={Copy}
                  onClick={() => {
                    void navigator.clipboard?.writeText(apiKey);
                    toast.success("APIキーをコピーしました");
                  }}
                />
              </div>
              <div>
                <Button variant="secondary" onClick={() => setRotating(true)}>
                  再発行
                </Button>
              </div>
            </Field>
          </FieldGroup>
        </section>

        <section>
          <SectionTitle>コレクション</SectionTitle>
          <Switch
            label="空のコレクションを自動で削除する"
            checked={cur.deleteEmptyCollections}
            onToggle={() => patch({ deleteEmptyCollections: !cur.deleteEmptyCollections })}
          />
        </section>
      </div>

      {dirty && (
        <div className="sticky bottom-20 z-10 -mx-4 mt-8 flex items-center justify-end gap-2 border-t border-border bg-background px-4 py-3 md:bottom-0 md:-mx-8 md:px-8">
          <Button variant="ghost" onClick={() => setDraft(null)}>
            変更を破棄
          </Button>
          <Button variant="primary" onClick={save}>
            保存
          </Button>
        </div>
      )}

      <ConfirmDialog
        open={rotating}
        onOpenChange={setRotating}
        title="APIキーを再発行しますか"
        body="今のキーは使えなくなります。キーを使っているアプリは、新しいキーに入れ替えてください。"
        confirmLabel="再発行"
        onConfirm={() => {
          setApiKey(`cmk_${Math.random().toString(36).slice(2, 12)}${Math.random().toString(36).slice(2, 12)}`);
          toast.success("APIキーを再発行しました");
        }}
      />
    </Page>
  );
}
