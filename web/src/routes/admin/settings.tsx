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
import { defineMessages, useT } from "@/i18n";
import { ConfirmDialog, IconAction, relTime } from "./_parts";

const m = defineMessages({
  ja: {
    title: "サーバー設定",
    general: "一般",
    port: "ポート",
    restartNote: "変更はサーバーを再起動すると反映されます",
    portNote: "変更はサーバーを再起動すると反映されます。環境変数 COMINAVI_PORT があるときは、そちらが使われます（Docker イメージでは 25600 に決まっています）",
    taskThreads: "タスクの同時実行数",
    thumbnails: "サムネイル",
    thumbSize: "サイズ",
    sizeSmall: "小",
    sizeMedium: "中",
    sizeLarge: "大",
    sizeXlarge: "特大",
    thumbFormat: "形式",
    webpLater: "WebP と AVIF は後で対応します",
    thumbNote: "新しく作るサムネイルから反映されます。今あるものは、シリーズやライブラリの「表紙を作り直す」で作り直せます。",
    auth: "認証",
    rememberDays: "ログイン保持の日数",
    loginScreen: "ログイン画面",
    backdropCompass: "羅針盤",
    backdropCovers: "表紙の壁",
    backdropCompassNote: "ログイン前の画面には ComiNavi の羅針盤だけが表示されます",
    backdropCoversNote: "最近追加したシリーズの表紙を並べます。ログインしていない人にも表紙が見えます",
    integrations: "連携",
    apiKeys: "API キー",
    noApiKeys: "APIキーはまだありません",
    keyCreated: "作成",
    keyLastUsed: "最終使用",
    keyNever: "なし",
    delete: "削除",
    keyNameLabel: "APIキーの名前",
    name: "名前",
    create: "作成",
    keyNote: "「{name}」のキー。この画面を閉じると、キーは二度と表示されません",
    close: "閉じる",
    copy: "コピー",
    keyDeleted: "APIキーを削除しました",
    keyCopied: "APIキーをコピーしました",
    copyFailed: "コピーできませんでした",
    settingsSaved: "設定を保存しました",
    collections: "コレクション",
    autoDeleteCollections: "空のコレクションを自動で削除する",
    deleteKeyTitle: "APIキーを削除しますか",
    deleteKeyBody: "「{name}」を使っているアプリは接続できなくなります。",
  },
  en: {
    title: "Server settings",
    general: "General",
    port: "Port",
    restartNote: "Changes take effect after the server restarts",
    portNote: "Changes take effect after the server restarts. If the COMINAVI_PORT environment variable is set, it is used instead (the Docker image uses 25600)",
    taskThreads: "Concurrent tasks",
    thumbnails: "Thumbnails",
    thumbSize: "Size",
    sizeSmall: "Small",
    sizeMedium: "Medium",
    sizeLarge: "Large",
    sizeXlarge: "Extra large",
    thumbFormat: "Format",
    webpLater: "WebP and AVIF will come later",
    thumbNote: "Applies to newly made thumbnails. To remake existing ones, use Regenerate covers on a series or library.",
    auth: "Authentication",
    rememberDays: "Days to stay signed in",
    loginScreen: "Sign-in screen",
    backdropCompass: "Compass",
    backdropCovers: "Cover wall",
    backdropCompassNote: "Only the ComiNavi compass is shown before sign-in",
    backdropCoversNote: "Shows covers of recently added series. People who are not signed in can see them too.",
    integrations: "Integrations",
    apiKeys: "API keys",
    noApiKeys: "No API keys yet",
    keyCreated: "Created",
    keyLastUsed: "Last used",
    keyNever: "Never",
    delete: "Delete",
    keyNameLabel: "API key name",
    name: "Name",
    create: "Create",
    keyNote: "Key for \"{name}\". It is not shown again after you close this screen",
    close: "Close",
    copy: "Copy",
    keyDeleted: "API key deleted",
    keyCopied: "API key copied",
    copyFailed: "Could not copy",
    settingsSaved: "Settings saved",
    collections: "Collections",
    autoDeleteCollections: "Delete empty collections automatically",
    deleteKeyTitle: "Delete this API key?",
    deleteKeyBody: "Apps using \"{name}\" will no longer be able to connect.",
  },
});

export default function Screen() {
  const t = useT(m);
  const sizeLabel: Record<ServerSettings["thumbnailSize"], string> = {
    small: t("sizeSmall"),
    medium: t("sizeMedium"),
    large: t("sizeLarge"),
    xlarge: t("sizeXlarge"),
  };
  const backdropLabel: Record<ServerSettings["loginBackdrop"], string> = { compass: t("backdropCompass"), covers: t("backdropCovers") };
  const backdropNote: Record<ServerSettings["loginBackdrop"], string> = { compass: t("backdropCompassNote"), covers: t("backdropCoversNote") };
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
        <PageHeader title={t("title")} />
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
      toast.success(t("settingsSaved"));
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
      toast.success(t("keyDeleted"));
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
      toast.success(t("keyCopied"));
    } catch {
      toast.error(t("copyFailed"));
    }
  };

  return (
    <Page>
      <PageHeader title={t("title")} />

      <div className="max-w-[640px] space-y-10">
        <section>
          <SectionTitle>{t("general")}</SectionTitle>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="set-port">{t("port")}</FieldLabel>
              <Input
                id="set-port"
                type="number"
                min={1}
                max={65535}
                className="tabular-nums"
                value={cur.port}
                onChange={(e) => patch({ port: Number(e.target.value) })}
              />
              <FieldDescription>
                {t("portNote")}
              </FieldDescription>
            </Field>
            <Slider
              value={cur.taskThreads}
              onChange={(v) => patch({ taskThreads: v as number })}
              min={1}
              max={16}
              step={1}
              label={t("taskThreads")}
              formatValue={(v) => `${v}`}
            />
            <FieldDescription>{t("restartNote")}</FieldDescription>
          </FieldGroup>
        </section>

        <section>
          <SectionTitle>{t("thumbnails")}</SectionTitle>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="set-thumb-size">{t("thumbSize")}</FieldLabel>
              <Select value={cur.thumbnailSize} onValueChange={(v) => patch({ thumbnailSize: v as ServerSettings["thumbnailSize"] })}>
                <SelectTrigger id="set-thumb-size" className="w-full" />
                <SelectContent>
                  {(Object.keys(sizeLabel) as ServerSettings["thumbnailSize"][]).map((k, i) => (
                    <SelectItem key={k} index={i} value={k}>
                      {sizeLabel[k]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FieldDescription>{t("thumbNote")}</FieldDescription>
            </Field>
            <Field>
              <FieldLabel id="set-thumb-format">{t("thumbFormat")}</FieldLabel>
              <RadioGroup
                aria-labelledby="set-thumb-format"
                value={cur.thumbnailFormat}
                onValueChange={(v) => patch({ thumbnailFormat: v as ServerSettings["thumbnailFormat"] })}
              >
                <RadioItem index={0} value="jpeg" label="JPEG" />
              </RadioGroup>
              <FieldDescription>{t("webpLater")}</FieldDescription>
            </Field>
          </FieldGroup>
        </section>

        <section>
          <SectionTitle>{t("auth")}</SectionTitle>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="set-remember">{t("rememberDays")}</FieldLabel>
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
              <FieldLabel id="set-login-backdrop">{t("loginScreen")}</FieldLabel>
              <RadioGroup
                aria-labelledby="set-login-backdrop"
                aria-describedby="set-login-backdrop-note"
                value={cur.loginBackdrop}
                onValueChange={(v) => patch({ loginBackdrop: v as ServerSettings["loginBackdrop"] })}
              >
                {(Object.keys(backdropLabel) as ServerSettings["loginBackdrop"][]).map((k, i) => (
                  <RadioItem key={k} index={i} value={k} label={backdropLabel[k]} />
                ))}
              </RadioGroup>
              <FieldDescription id="set-login-backdrop-note">{backdropNote[cur.loginBackdrop]}</FieldDescription>
            </Field>
          </FieldGroup>
        </section>

        <section>
          <SectionTitle>{t("integrations")}</SectionTitle>
          <FieldGroup>
            {/* OPDS returns with the OPDS server (a later phase); Kobo sync is hidden until then too. */}
            <Field>
              <FieldLabel id="set-api-keys">{t("apiKeys")}</FieldLabel>
              {keys.error && !keys.data && <LoadError message={keys.error} onRetry={keys.reload} />}
              {keys.data && keys.data.length === 0 && (
                <p className={cn(typeClass("caption"), "text-muted-foreground")}>{t("noApiKeys")}</p>
              )}
              {keys.data && keys.data.length > 0 && (
                <ul aria-labelledby="set-api-keys" className="flex flex-col gap-1">
                  {keys.data.map((k) => (
                    <li key={k.id} className="flex items-center gap-2 rounded-lg bg-card py-2 pl-3 pr-1">
                      <div className="min-w-0 flex-1">
                        <p className={cn(typeClass("body"), "truncate text-foreground")}>{k.name}</p>
                        <p className={cn(typeClass("caption"), "text-muted-foreground")}>
                          {t("keyCreated")} <span className="font-mono">{formatDate(k.createdAt)}</span> · {t("keyLastUsed")}{" "}
                          {k.lastUsed ? relTime(k.lastUsed) : t("keyNever")}
                        </p>
                      </div>
                      <IconAction
                        label={t("delete")}
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
                  aria-label={t("keyNameLabel")}
                  placeholder={t("name")}
                  value={keyName}
                  disabled={creatingKey}
                  onChange={(e) => setKeyName(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") void createKey();
                  }}
                />
                <Button variant="secondary" disabled={creatingKey || !keyName.trim()} onClick={() => void createKey()}>
                  {t("create")}
                </Button>
              </div>
              {createdKey && (
                <div className="space-y-2 rounded-lg bg-card p-3">
                  <p className={cn(typeClass("caption"), "text-muted-foreground")}>
                    {t("keyNote", { name: createdKey.name })}
                  </p>
                  <div className="flex items-center gap-1">
                    <code className="min-w-0 flex-1 break-all font-mono text-foreground">{createdKey.key}</code>
                    <IconAction label={t("copy")} icon={Copy} onClick={() => void copyKey(createdKey.key)} />
                  </div>
                  <Button variant="ghost" onClick={() => setCreatedKey(null)}>
                    {t("close")}
                  </Button>
                </div>
              )}
            </Field>
          </FieldGroup>
        </section>

        <section>
          <SectionTitle>{t("collections")}</SectionTitle>
          <Switch
            label={t("autoDeleteCollections")}
            checked={cur.deleteEmptyCollections}
            onToggle={() => patch({ deleteEmptyCollections: !cur.deleteEmptyCollections })}
          />
        </section>
      </div>

      <SaveBar open={dirty && !saving} onDiscard={() => setDraft(null)} onSave={save} />

      <ConfirmDialog
        open={removeKeyOpen}
        onOpenChange={setRemoveKeyOpen}
        title={t("deleteKeyTitle")}
        body={t("deleteKeyBody", { name: removingKey?.name ?? "" })}
        confirmLabel={t("delete")}
        onConfirm={() => {
          if (removingKey) void removeKey(removingKey);
        }}
      />
    </Page>
  );
}
