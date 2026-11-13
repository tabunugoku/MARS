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
    portNote: "変更はサーバーを再起動すると反映されます。環境変数 MARS_PORT があるときは、そちらが使われます（Docker イメージでは 25600 に決まっています）",
    taskThreads: "タスクの同時実行数",
    thumbnails: "サムネイル",
    thumbSize: "サイズ",
    sizeSmall: "小",
    sizeMedium: "中",
    sizeLarge: "大",
    sizeXlarge: "特大",
    thumbFormat: "形式",
    thumbNote: "新しく作るサムネイルから反映されます。今あるものは、シリーズやライブラリの「表紙を作り直す」で作り直せます。",
    thumbFormatNote: "新しく作るサムネイルから使われます。今あるものは、シリーズやライブラリの「表紙を作り直す」で切り替わります。OPDS と Komga 互換 API には、常に JPEG で返します。",
    auth: "認証",
    rememberDays: "ログイン保持の日数",
    loginScreen: "ログイン画面",
    backdropPlanet: "惑星",
    backdropCovers: "シリーズの表紙",
    backdropPlanetNote: "ログイン前の画面には MARS の惑星と軌道だけが表示されます",
    backdropCoversNote: "最近追加したシリーズの表紙を並べます。ログインしていない人にも表紙が見えます",
    backdropBookCovers: "巻の表紙",
    backdropBookCoversNote: "最近追加した巻の表紙を並べます。ログインしていない人にも表紙が見えます",
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
    opds: "OPDS",
    opdsNote: "Panels、Chunky、KyBook などの電子書籍アプリは、次のアドレスに接続します。サインインには MARS のユーザー名とパスワード、または API キーを使います",
    komga: "Komga 互換 API（Mihon）",
    komgaNote: "Mihon の Komga 拡張とトラッカーは、次のアドレスに接続します。サインインには MARS のユーザー名とパスワード、または API キーを使います",
    addressCopied: "アドレスをコピーしました",
    settingsSaved: "設定を保存しました",
    collections: "コレクション",
    autoDeleteCollections: "空のコレクションを自動で削除する",
    autoDeleteCollectionsNote: "ゴミ箱を空にしたときやライブラリを削除したときに、シリーズがひとつも残らなかったコレクションを削除します",
    deleteKeyTitle: "APIキーを削除しますか",
    deleteKeyBody: "「{name}」を使っているアプリは接続できなくなります。",
  },
  en: {
    title: "Server settings",
    general: "General",
    port: "Port",
    restartNote: "Changes take effect after the server restarts",
    portNote: "Changes take effect after the server restarts. If the MARS_PORT environment variable is set, it is used instead (the Docker image uses 25600)",
    taskThreads: "Concurrent tasks",
    thumbnails: "Thumbnails",
    thumbSize: "Size",
    sizeSmall: "Small",
    sizeMedium: "Medium",
    sizeLarge: "Large",
    sizeXlarge: "Extra large",
    thumbFormat: "Format",
    thumbNote: "Applies to newly made thumbnails. To remake existing ones, use Regenerate covers on a series or library.",
    thumbFormatNote: "Applies to newly made thumbnails. Existing ones change when you use Regenerate covers on a series or library. OPDS and the Komga API always get JPEG.",
    auth: "Authentication",
    rememberDays: "Days to stay signed in",
    loginScreen: "Sign-in screen",
    backdropPlanet: "Planet",
    backdropCovers: "Series covers",
    backdropPlanetNote: "Only the MARS planet and its orbit are shown before sign-in",
    backdropCoversNote: "Shows covers of recently added series. People who are not signed in can see them too.",
    backdropBookCovers: "Book covers",
    backdropBookCoversNote: "Shows covers of recently added books. People who are not signed in can see them too.",
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
    opds: "OPDS",
    opdsNote: "Reader apps such as Panels, Chunky and KyBook connect to this address. They sign in with a MARS user name and password, or an API key.",
    komga: "Komga-compatible API (Mihon)",
    komgaNote: "The Komga extension and tracker in Mihon connect to this address. They sign in with a MARS user name and password, or an API key.",
    addressCopied: "Address copied",
    settingsSaved: "Settings saved",
    collections: "Collections",
    autoDeleteCollections: "Delete empty collections automatically",
    autoDeleteCollectionsNote: "When emptying the trash or deleting a library leaves a collection with no series, the collection is deleted too",
    deleteKeyTitle: "Delete this API key?",
    deleteKeyBody: "Apps using \"{name}\" will no longer be able to connect.",
  },
});

// AddressRow shows an address to enter in a reader app, in monospace, with a
// copy button.
function AddressRow({ url, copyLabel, onCopy }: { url: string; copyLabel: string; onCopy: (url: string) => void }) {
  return (
    <div className="flex items-center gap-1 rounded-lg bg-card py-1 pl-3 pr-1">
      <code className={cn(typeClass("body"), "min-w-0 flex-1 select-all break-all font-mono text-foreground")}>{url}</code>
      <IconAction label={copyLabel} icon={Copy} onClick={() => onCopy(url)} />
    </div>
  );
}

export default function Screen() {
  const t = useT(m);
  const sizeLabel: Record<ServerSettings["thumbnailSize"], string> = {
    small: t("sizeSmall"),
    medium: t("sizeMedium"),
    large: t("sizeLarge"),
    xlarge: t("sizeXlarge"),
  };
  const formatLabel: Record<ServerSettings["thumbnailFormat"], string> = {
    jpeg: "JPEG",
    webp: "WebP",
    avif: "AVIF",
  };
  const backdropLabel: Record<ServerSettings["loginBackdrop"], string> = {
    planet: t("backdropPlanet"),
    covers: t("backdropCovers"),
    bookCovers: t("backdropBookCovers"),
  };
  const backdropNote: Record<ServerSettings["loginBackdrop"], string> = {
    planet: t("backdropPlanetNote"),
    covers: t("backdropCoversNote"),
    bookCovers: t("backdropBookCoversNote"),
  };
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

  const copyText = async (text: string, done: string) => {
    try {
      await navigator.clipboard.writeText(text);
      toast.success(done);
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
              <FieldLabel htmlFor="set-thumb-format">{t("thumbFormat")}</FieldLabel>
              <Select value={cur.thumbnailFormat} onValueChange={(v) => patch({ thumbnailFormat: v as ServerSettings["thumbnailFormat"] })}>
                <SelectTrigger id="set-thumb-format" className="w-full" />
                <SelectContent>
                  {(Object.keys(formatLabel) as ServerSettings["thumbnailFormat"][]).map((k, i) => (
                    <SelectItem key={k} index={i} value={k}>
                      {formatLabel[k]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FieldDescription>{t("thumbFormatNote")}</FieldDescription>
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
            <Field>
              <Switch
                label={t("opds")}
                checked={cur.opdsEnabled}
                onToggle={() => patch({ opdsEnabled: !cur.opdsEnabled })}
              />
              <FieldDescription>{t("opdsNote")}</FieldDescription>
              {cur.opdsEnabled && (
                <AddressRow
                  url={`${window.location.origin}/opds/v1.2/catalog`}
                  copyLabel={t("copy")}
                  onCopy={(url) => void copyText(url, t("addressCopied"))}
                />
              )}
            </Field>
            <Field>
              <Switch
                label={t("komga")}
                checked={cur.komgaApiEnabled}
                onToggle={() => patch({ komgaApiEnabled: !cur.komgaApiEnabled })}
              />
              <FieldDescription>{t("komgaNote")}</FieldDescription>
              {cur.komgaApiEnabled && (
                <AddressRow
                  url={`${window.location.origin}/komga`}
                  copyLabel={t("copy")}
                  onCopy={(url) => void copyText(url, t("addressCopied"))}
                />
              )}
            </Field>
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
                    <code className={cn(typeClass("body"), "min-w-0 flex-1 break-all font-mono text-foreground")}>{createdKey.key}</code>
                    <IconAction label={t("copy")} icon={Copy} onClick={() => void copyText(createdKey.key, t("keyCopied"))} />
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
          <Field>
            <Switch
              label={t("autoDeleteCollections")}
              checked={cur.deleteEmptyCollections}
              onToggle={() => patch({ deleteEmptyCollections: !cur.deleteEmptyCollections })}
            />
            <FieldDescription>{t("autoDeleteCollectionsNote")}</FieldDescription>
          </Field>
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
