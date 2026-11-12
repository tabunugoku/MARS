import { useRef, useState, type ChangeEvent } from "react";
import { useNavigate } from "react-router";
import { toast } from "@yusufceng1z/ding-react";
import { api, errorMessage, type CblImportResult } from "@/api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { fontWeights } from "@/lib/font-weight";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { defineMessages, useT } from "@/i18n";

const MAX_BYTES = 5 * 1024 * 1024;
const ACCEPT = ".cbl,application/xml,text/xml";

const m = defineMessages({
  ja: {
    trigger: "CBL を読み込む",
    title: "CBL を読み込む",
    pick: "CBL ファイル",
    hint: "ComicRack の .cbl ファイルから、リーディングリストを作成します。",
    tooLarge: "ファイルは 5 MB 以下にしてください",
    checking: "ファイルを確認しています",
    summary: ({ matched, unmatched }) => `${matched} 件一致 / ${unmatched} 件未一致`,
    unmatchedTitle: ({ n }) => `一致しなかった ${n} 件`,
    noneMatched: "一致する本がありません。ファイル内のシリーズ名と番号、閲覧できるライブラリに本があるかを確認してください。",
    entryNumber: ({ v }) => `番号 ${v}`,
    entryVolume: ({ v }) => `巻 ${v}`,
    entryYear: ({ v }) => `${v} 年`,
    cancel: "キャンセル",
    create: "作成",
    created: "リーディングリストを作成しました",
    noResult: "リーディングリストを作成できませんでした",
  },
  en: {
    trigger: "Import CBL",
    title: "Import CBL",
    pick: "CBL file",
    hint: "Create a read list from a ComicRack .cbl file.",
    tooLarge: "The file must be 5 MB or smaller",
    checking: "Checking the file",
    summary: ({ matched, unmatched }) => `${matched} matched / ${unmatched} unmatched`,
    unmatchedTitle: ({ n }) => (n === 1 ? "1 unmatched entry" : `${n} unmatched entries`),
    noneMatched:
      "No entries matched. Check the series names and numbers in the file, and that the books are in a library you can see.",
    entryNumber: ({ v }) => `Number ${v}`,
    entryVolume: ({ v }) => `Volume ${v}`,
    entryYear: ({ v }) => `Year ${v}`,
    cancel: "Cancel",
    create: "Create",
    created: "Read list created",
    noResult: "The read list could not be created",
  },
});

export function ImportCblDialog() {
  const t = useT(m);
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState<CblImportResult | null>(null);
  const [checking, setChecking] = useState(false);
  const [creating, setCreating] = useState(false);
  // Bumped on every pick or reset so a slow preview cannot overwrite a newer one.
  const seq = useRef(0);

  const reset = () => {
    seq.current++;
    setFile(null);
    setPreview(null);
    setChecking(false);
    setCreating(false);
  };

  const onOpenChange = (next: boolean) => {
    if (creating) return;
    if (!next) reset();
    setOpen(next);
  };

  const pick = (e: ChangeEvent<HTMLInputElement>) => {
    const picked = e.target.files?.[0] ?? null;
    e.target.value = "";
    if (!picked) return;
    seq.current++;
    const current = seq.current;
    setPreview(null);
    setFile(null);
    if (picked.size > MAX_BYTES) {
      toast.error(t("tooLarge"));
      return;
    }
    setFile(picked);
    setChecking(true);
    api
      .importReadList(picked, true)
      .then((res) => {
        if (seq.current === current) setPreview(res);
      })
      .catch((err: unknown) => {
        if (seq.current !== current) return;
        setFile(null);
        toast.error(errorMessage(err));
      })
      .finally(() => {
        if (seq.current === current) setChecking(false);
      });
  };

  const canCreate = file !== null && preview !== null && preview.matched.length > 0 && !checking && !creating;

  const create = () => {
    if (!file || !canCreate) return;
    setCreating(true);
    api
      .importReadList(file, false)
      .then((res) => {
        if (!res.readList) {
          toast.error(t("noResult"));
          return;
        }
        toast.success(t("created"));
        reset();
        setOpen(false);
        navigate(`/readlists/${res.readList.id}`);
      })
      .catch((err: unknown) => toast.error(errorMessage(err)))
      .finally(() => setCreating(false));
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogTrigger render={<Button variant="secondary">{t("trigger")}</Button>} />
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>{t("title")}</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <Field>
            <FieldLabel htmlFor="cbl-file">{t("pick")}</FieldLabel>
            <Input
              id="cbl-file"
              type="file"
              accept={ACCEPT}
              disabled={checking || creating}
              onChange={pick}
            />
            <FieldDescription>{t("hint")}</FieldDescription>
          </Field>

          {checking && <p className={cn(typeClass("caption"), "text-muted-foreground")}>{t("checking")}</p>}

          {preview && (
            <div className="flex flex-col gap-3">
              <div className="flex flex-col gap-0.5">
                <p className="text-foreground" style={{ fontVariationSettings: fontWeights.semibold }}>
                  {preview.name}
                </p>
                <p className={cn(typeClass("caption"), "tabular-nums text-muted-foreground")}>
                  {t("summary", { matched: preview.matched.length, unmatched: preview.unmatched.length })}
                </p>
              </div>

              {preview.matched.length === 0 && (
                <p className={cn(typeClass("caption"), "text-muted-foreground")}>{t("noneMatched")}</p>
              )}

              {preview.unmatched.length > 0 && (
                <div className="flex flex-col gap-2">
                  <p className={cn(typeClass("caption"), "text-muted-foreground")}>
                    {t("unmatchedTitle", { n: preview.unmatched.length })}
                  </p>
                  <ul className="max-h-60 divide-y divide-border overflow-y-auto rounded-md border border-border">
                    {preview.unmatched.map((e, i) => {
                      const parts: string[] = [];
                      if (e.number) parts.push(t("entryNumber", { v: e.number }));
                      if (e.volume) parts.push(t("entryVolume", { v: e.volume }));
                      if (e.year) parts.push(t("entryYear", { v: e.year }));
                      return (
                        <li key={i} className="px-3 py-2">
                          <p className="truncate text-foreground">{e.series}</p>
                          {parts.length > 0 && (
                            <p className={cn(typeClass("caption"), "text-muted-foreground")}>{parts.join(" · ")}</p>
                          )}
                        </li>
                      );
                    })}
                  </ul>
                </div>
              )}
            </div>
          )}
        </div>
        <DialogFooter>
          <DialogClose render={<Button variant="ghost">{t("cancel")}</Button>} />
          <Button variant="primary" disabled={!canCreate} onClick={create}>
            {t("create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
