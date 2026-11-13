import { useRef, useState } from "react";
import { Bin, Copy } from "@keyline-icons/react";
import { api, errorMessage, formatDate, useApi, type ApiKey, type ApiKeyCreated } from "@/api";
import { LoadError } from "@/components/shell/load-error";
import { SectionTitle } from "@/components/media/layout";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldGroup } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { toast } from "@yusufceng1z/ding-react";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { defineMessages, useT } from "@/i18n";
import { ConfirmDialog, IconAction, relTime } from "@/routes/admin/_parts";

const m = defineMessages({
  ja: {
    apiKeys: "API キー",
    apiKeysNote: "OPDS アプリや Mihon の Komga 拡張で、パスワードの代わりに使えます",
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
    copyFailed: "コピーできませんでした",
    keyCopied: "APIキーをコピーしました",
    keyDeleted: "APIキーを削除しました",
    deleteKeyTitle: "APIキーを削除しますか",
    deleteKeyBody: "「{name}」を使っているアプリは接続できなくなります。",
  },
  en: {
    apiKeys: "API keys",
    apiKeysNote: "Use a key instead of your password in OPDS apps and Mihon's Komga extension",
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
    copyFailed: "Could not copy",
    keyCopied: "API key copied",
    keyDeleted: "API key deleted",
    deleteKeyTitle: "Delete this API key?",
    deleteKeyBody: "Apps using \"{name}\" will no longer be able to connect.",
  },
});

// The signed-in user's own API keys, for OPDS apps and the Komga API. Any
// user can manage their own keys; the server scopes them to the caller.
export function ApiKeys() {
  const t = useT(m);
  const keys = useApi(() => api.apiKeys(), []);

  const [keyName, setKeyName] = useState("");
  const [creatingKey, setCreatingKey] = useState(false);
  // A second Enter before the first answer would create a second live key.
  const keyInFlight = useRef(false);
  const [createdKey, setCreatedKey] = useState<ApiKeyCreated | null>(null);
  const [removingKey, setRemovingKey] = useState<ApiKey | null>(null);
  const [removeKeyOpen, setRemoveKeyOpen] = useState(false);
  const [keyBusyId, setKeyBusyId] = useState<string | null>(null);

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
    <section>
      <SectionTitle>{t("apiKeys")}</SectionTitle>
      <FieldGroup>
        <FieldDescription>{t("apiKeysNote")}</FieldDescription>
        <Field>
          {keys.error && !keys.data && <LoadError message={keys.error} onRetry={keys.reload} />}
          {keys.data && keys.data.length === 0 && (
            <p className={cn(typeClass("caption"), "text-muted-foreground")}>{t("noApiKeys")}</p>
          )}
          {keys.data && keys.data.length > 0 && (
            <ul aria-label={t("apiKeys")} className="flex flex-col gap-1">
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
    </section>
  );
}
