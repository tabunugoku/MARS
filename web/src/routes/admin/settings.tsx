import { useRef, useState } from "react";
import { Bin, Copy } from "@keyline-icons/react";
import { api, errorMessage, formatDate, useApi, type ApiKey, type ApiKeyCreated, type ServerSettings } from "@/api";
import { LoadError } from "@/components/shell/load-error";
import { Block } from "@/components/media/skeletons";
import { Page, PageHeader, SectionTitle } from "@/components/media/layout";
import { SaveBar } from "@/components/shell/save-bar";
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
import { ConfirmDialog, IconAction, relTime } from "./_parts";

const SIZE_LABEL: Record<ServerSettings["thumbnailSize"], string> = { small: "小", medium: "中", large: "大", xlarge: "特大" };
const BACKDROP_LABEL: Record<ServerSettings["loginBackdrop"], string> = { compass: "羅針盤", covers: "表紙の壁" };
const BACKDROP_NOTE: Record<ServerSettings["loginBackdrop"], string> = {
  compass: "ログイン前の画面には ComiNavi の羅針盤だけが表示されます",
  covers: "最近追加したシリーズの表紙を並べます。ログインしていない人にも表紙が見えます",
};
const RESTART_NOTE = "変更はサーバーを再起動すると反映されます";

export default function Screen() {
  const { data, error, reload } = useApi(() => api.settings(), []);
  const keys = useApi(() => api.apiKeys(), []);
  // The last copy the server returned; drafts are compared against it.
  const [saved, setSaved] = useState<ServerSettings | null>(null);
  const [draft, setDraft] = useState<ServerSettings | null>(null);
  const [saving, setSaving] = useState(false);

  const [keyName, setKeyName] = useState("");
  const [creatingKey, setCreatingKey] = useState(false);
  // A second Enter before the first answer would create a second live key.
  const keyInFlight = useRef(false);
  const [createdKey, setCreatedKey] = useState<ApiKeyCreated | null>(null);
  const [removingKey, setRemovingKey] = useState<ApiKey | null>(null);
  const [removeKeyOpen, setRemoveKeyOpen] = useState(false);
  const [keyBusyId, setKeyBusyId] = useState<string | null>(null);

  const base = saved ?? data;
  const cur = draft ?? base;
  const dirty = draft !== null && base !== undefined && JSON.stringify(draft) !== JSON.stringify(base);

  if (!cur) {
    return (
      <Page>
        <PageHeader title="サーバー設定" />
        {error ? (
          <LoadError message={error} onRetry={reload} />
        ) : (
          <div className="max-w-[640px] space-y-3" aria-hidden>
            <Block className="h-5 w-24" />
            <Block className="h-9 w-full" />
            <Block className="h-9 w-full" />
          </div>
        )}
      </Page>
    );
  }

  const patch = (p: Partial<ServerSettings>) => setDraft({ ...cur, ...p });

  const save = async () => {
    if (!draft || !base || saving) return;
    const changes = Object.fromEntries(
      Object.entries(draft).filter(([k, v]) => base[k as keyof ServerSettings] !== v),
    ) as Partial<ServerSettings>;
    if (Object.keys(changes).length === 0) {
      setDraft(null);
      return;
    }
    setSaving(true);
    try {
      setSaved(await api.saveSettings(changes));
      setDraft(null);
      toast.success("設定を保存しました");
    } catch (err) {
      // Keep the draft so nothing typed is lost.
      toast.error(errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const createKey = async () => {
    const name = keyName.trim();
    if (!name || keyInFlight.current) return;
    keyInFlight.current = true;
    setCreatingKey(true);
    try {
      const created = await api.createApiKey(name);
      setCreatedKey(created);
      setKeyName("");
      keys.reload();
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      keyInFlight.current = false;
      setCreatingKey(false);
    }
  };

  const removeKey = async (k: ApiKey) => {
    setKeyBusyId(k.id);
    try {
      await api.deleteApiKey(k.id);
      toast.success("APIキーを削除しました");
      if (createdKey?.id === k.id) setCreatedKey(null);
      keys.reload();
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setKeyBusyId(null);
    }
  };

  const copyKey = async (key: string) => {
    try {
      await navigator.clipboard.writeText(key);
      toast.success("APIキーをコピーしました");
    } catch {
      toast.error("コピーできませんでした");
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
              <FieldDescription>{RESTART_NOTE}</FieldDescription>
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
            <FieldDescription>{RESTART_NOTE}</FieldDescription>
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
                <RadioItem index={0} value="jpeg" label="JPEG" />
              </RadioGroup>
              <FieldDescription>WebP と AVIF は後で対応します</FieldDescription>
            </Field>
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
              <FieldLabel id="set-api-keys">API キー</FieldLabel>
              {keys.error && !keys.data && <LoadError message={keys.error} onRetry={keys.reload} />}
              {keys.data && keys.data.length === 0 && (
                <p className={cn(typeClass("caption"), "text-muted-foreground")}>APIキーはまだありません</p>
              )}
              {keys.data && keys.data.length > 0 && (
                <ul aria-labelledby="set-api-keys" className="flex flex-col gap-1">
                  {keys.data.map((k) => (
                    <li key={k.id} className="flex items-center gap-2 rounded-lg bg-card py-2 pl-3 pr-1">
                      <div className="min-w-0 flex-1">
                        <p className={cn(typeClass("body"), "truncate text-foreground")}>{k.name}</p>
                        <p className={cn(typeClass("caption"), "text-muted-foreground")}>
                          作成 <span className="font-mono">{formatDate(k.createdAt)}</span> · 最終使用{" "}
                          {k.lastUsed ? relTime(k.lastUsed) : "なし"}
                        </p>
                      </div>
                      <IconAction
                        label="削除"
                        icon={Bin}
                        onClick={() => {
                          if (keyBusyId) return;
                          setRemovingKey(k);
                          setRemoveKeyOpen(true);
                        }}
                      />
                    </li>
                  ))}
                </ul>
              )}
              <div className="flex gap-2">
                <Input
                  aria-label="APIキーの名前"
                  placeholder="名前"
                  value={keyName}
                  disabled={creatingKey}
                  onChange={(e) => setKeyName(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") void createKey();
                  }}
                />
                <Button variant="secondary" disabled={creatingKey || !keyName.trim()} onClick={() => void createKey()}>
                  作成
                </Button>
              </div>
              {createdKey && (
                <div className="space-y-2 rounded-lg bg-card p-3">
                  <p className={cn(typeClass("caption"), "text-muted-foreground")}>
                    「{createdKey.name}」のキー。この画面を閉じると、キーは二度と表示されません
                  </p>
                  <div className="flex items-center gap-1">
                    <code className="min-w-0 flex-1 break-all font-mono text-foreground">{createdKey.key}</code>
                    <IconAction label="コピー" icon={Copy} onClick={() => void copyKey(createdKey.key)} />
                  </div>
                  <Button variant="ghost" onClick={() => setCreatedKey(null)}>
                    閉じる
                  </Button>
                </div>
              )}
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

      <SaveBar open={dirty && !saving} onDiscard={() => setDraft(null)} onSave={save} />

      <ConfirmDialog
        open={removeKeyOpen}
        onOpenChange={setRemoveKeyOpen}
        title="APIキーを削除しますか"
        body={`「${removingKey?.name ?? ""}」を使っているアプリは接続できなくなります。`}
        confirmLabel="削除"
        onConfirm={() => {
          if (removingKey) void removeKey(removingKey);
        }}
      />
    </Page>
  );
}
