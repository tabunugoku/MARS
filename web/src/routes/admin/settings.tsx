import { useState } from "react";
import { Copy } from "@keyline-icons/react";
import { api, errorMessage, useApi, type ServerSettings } from "@/api";
import { LoadError } from "@/components/shell/load-error";
import { Block } from "@/components/media/skeletons";
import { Page, PageHeader, SectionTitle } from "@/components/media/layout";
import { SaveBar } from "@/components/shell/save-bar";
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
import { IconAction } from "./_parts";

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
    copy: "コピー",
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
    copy: "Copy",
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
  // The last copy the server returned; drafts are compared against it.
  const [saved, setSaved] = useState<ServerSettings | null>(null);
  const [draft, setDraft] = useState<ServerSettings | null>(null);
  const [saving, setSaving] = useState(false);

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
    </Page>
  );
}
