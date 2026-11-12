import { useRef, useState } from "react";
import { toast } from "@yusufceng1z/ding-react";
import { api, errorMessage, statusLabel, type ReadingDirection, type SeriesBulkUpdate, type SeriesStatus } from "@/api";
import { inChunks, splitList } from "@/components/media/selection-state";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger } from "@/components/ui/select";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { defineMessages, useT } from "@/i18n";

const m = defineMessages({
  ja: {
    title: "一括編集",
    intro: "入力した項目だけを更新します。変更した項目はロックされ、スキャンで上書きされません。",
    keep: "変更しない",
    status: "ステータス",
    publisher: "出版社",
    direction: "読み方向",
    ageRating: "年齢制限",
    ageHint: "例: 18",
    ageInvalid: "年齢制限は数字で入力してください",
    clearAge: "年齢制限を外す",
    language: "言語",
    languageHint: "例: ja",
    addGenres: "ジャンルを追加",
    removeGenres: "ジャンルを外す",
    addTags: "タグを追加",
    removeTags: "タグを外す",
    listHint: "カンマで区切って入力",
    dirRtl: "右から左",
    dirLtr: "左から右",
    dirVertical: "縦",
    dirWebtoon: "縦スクロール",
    cancel: "キャンセル",
    submit: "更新",
    updated: ({ n }) => `${n} 件を更新しました`,
  },
  en: {
    title: "Edit series",
    intro: "Only the fields you fill in are updated. Changed fields get locked, so a scan will not overwrite them.",
    keep: "Keep",
    status: "Status",
    publisher: "Publisher",
    direction: "Reading direction",
    ageRating: "Age rating",
    ageHint: "e.g. 18",
    ageInvalid: "Enter the age rating as a number",
    clearAge: "Clear age rating",
    language: "Language",
    languageHint: "e.g. en",
    addGenres: "Add genres",
    removeGenres: "Remove genres",
    addTags: "Add tags",
    removeTags: "Remove tags",
    listHint: "Separate with commas",
    dirRtl: "Right to left",
    dirLtr: "Left to right",
    dirVertical: "Vertical",
    dirWebtoon: "Vertical scroll",
    cancel: "Cancel",
    submit: "Update",
    updated: ({ n }) => `Updated ${n}`,
  },
});

const STATUSES: SeriesStatus[] = ["ongoing", "ended", "hiatus", "abandoned"];
const DIRECTIONS: ReadingDirection[] = ["rtl", "ltr", "vertical", "webtoon"];

/** Bulk edit of series (admin). Only the fields the user filled in are sent;
 *  the server locks every field it changes. The parent mounts it fresh per open. */
export function SeriesBulkDialog({
  seriesIds,
  open,
  onOpenChange,
  onDone,
}: {
  seriesIds: string[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** ok: every chunk worked and the dialog has closed; the caller reloads and
   *  clears the selection. A failure keeps the dialog, its draft and the
   *  selection for a retry. Earlier chunks may have been saved, so when the
   *  dialog is then closed without a successful retry, onDone(false) runs and
   *  the caller reloads (keeping the selection). Reloading while the dialog is
   *  open could swap the screen for its error view and lose the draft. */
  onDone: (ok: boolean) => void;
}) {
  const t = useT(m);
  const labels = statusLabel();
  const [status, setStatus] = useState("keep");
  const [publisher, setPublisher] = useState("");
  const [direction, setDirection] = useState("keep");
  const [age, setAge] = useState("");
  const [clearAge, setClearAge] = useState(false);
  const [language, setLanguage] = useState("");
  const [addGenres, setAddGenres] = useState("");
  const [removeGenres, setRemoveGenres] = useState("");
  const [addTags, setAddTags] = useState("");
  const [removeTags, setRemoveTags] = useState("");
  const [saving, setSaving] = useState(false);

  const directionName: Record<ReadingDirection, string> = {
    rtl: t("dirRtl"),
    ltr: t("dirLtr"),
    vertical: t("dirVertical"),
    webtoon: t("dirWebtoon"),
  };

  const ageText = age.trim();
  const ageValid = ageText === "" || /^\d{1,3}$/.test(ageText);

  // Only what the user touched goes out; an empty field means "leave it".
  const fields: Omit<SeriesBulkUpdate, "seriesIds"> = {};
  if (status !== "keep") fields.status = status as SeriesStatus;
  if (publisher.trim()) fields.publisher = publisher.trim();
  if (direction !== "keep") fields.readingDirection = direction as ReadingDirection;
  if (clearAge) fields.ageRating = null;
  else if (ageText && ageValid) fields.ageRating = Number(ageText);
  if (language.trim()) fields.language = language.trim();
  const addG = splitList(addGenres);
  const removeG = splitList(removeGenres);
  const addT = splitList(addTags);
  const removeT = splitList(removeTags);
  if (addG.length) fields.addGenres = addG;
  if (removeG.length) fields.removeGenres = removeG;
  if (addT.length) fields.addTags = addT;
  if (removeT.length) fields.removeTags = removeT;
  const changed = Object.keys(fields).length > 0;

  // A failed save may have saved earlier chunks: the caller reloads once the
  // dialog closes (see onDone).
  const failed = useRef(false);
  const close = (next: boolean) => {
    onOpenChange(next);
    if (!next && failed.current) {
      failed.current = false;
      onDone(false);
    }
  };

  const submit = async () => {
    if (!changed || !ageValid || saving || seriesIds.length === 0) return;
    setSaving(true);
    try {
      const n = await inChunks(seriesIds, (chunk) => api.bulkUpdateSeries({ ...fields, seriesIds: chunk }).then((r) => r.updated));
      toast.success(t("updated", { n }));
      onOpenChange(false);
      failed.current = false;
      onDone(true);
    } catch (err) {
      toast.error(errorMessage(err));
      failed.current = true;
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>{t("title")}</DialogTitle>
          <DialogDescription>{t("intro")}</DialogDescription>
        </DialogHeader>

        <div className="grid max-h-[60dvh] gap-4 overflow-y-auto md:grid-cols-2">
          <Field>
            <FieldLabel>{t("status")}</FieldLabel>
            <Select value={status} onValueChange={(v) => setStatus(v)}>
              <SelectTrigger aria-label={t("status")} />
              <SelectContent>
                <SelectItem index={0} value="keep">
                  {t("keep")}
                </SelectItem>
                {STATUSES.map((s, i) => (
                  <SelectItem key={s} index={i + 1} value={s}>
                    {labels[s]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>

          <Field>
            <FieldLabel>{t("direction")}</FieldLabel>
            <Select value={direction} onValueChange={(v) => setDirection(v)}>
              <SelectTrigger aria-label={t("direction")} />
              <SelectContent>
                <SelectItem index={0} value="keep">
                  {t("keep")}
                </SelectItem>
                {DIRECTIONS.map((d, i) => (
                  <SelectItem key={d} index={i + 1} value={d}>
                    {directionName[d]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>

          <Field>
            <FieldLabel htmlFor="bulk-publisher">{t("publisher")}</FieldLabel>
            <Input id="bulk-publisher" value={publisher} placeholder={t("keep")} onChange={(e) => setPublisher(e.target.value)} />
          </Field>

          <Field>
            <FieldLabel htmlFor="bulk-language">{t("language")}</FieldLabel>
            <Input
              id="bulk-language"
              value={language}
              placeholder={t("languageHint")}
              onChange={(e) => setLanguage(e.target.value)}
            />
          </Field>

          <Field>
            <FieldLabel htmlFor="bulk-age">{t("ageRating")}</FieldLabel>
            <Input
              id="bulk-age"
              inputMode="numeric"
              value={age}
              disabled={clearAge}
              placeholder={t("ageHint")}
              aria-invalid={!ageValid || undefined}
              onChange={(e) => setAge(e.target.value)}
            />
            <label className={cn(typeClass("body"), "flex min-h-9 cursor-pointer items-center gap-2 text-foreground pointer-coarse:min-h-11")}>
              <Checkbox checked={clearAge} onCheckedChange={(c) => setClearAge(c)} />
              {t("clearAge")}
            </label>
            {!ageValid && <p className={cn(typeClass("caption"), "text-muted-foreground")}>{t("ageInvalid")}</p>}
          </Field>

          <Field>
            <FieldLabel htmlFor="bulk-add-genres">{t("addGenres")}</FieldLabel>
            <Input id="bulk-add-genres" value={addGenres} placeholder={t("listHint")} onChange={(e) => setAddGenres(e.target.value)} />
          </Field>

          <Field>
            <FieldLabel htmlFor="bulk-remove-genres">{t("removeGenres")}</FieldLabel>
            <Input id="bulk-remove-genres" value={removeGenres} placeholder={t("listHint")} onChange={(e) => setRemoveGenres(e.target.value)} />
          </Field>

          <Field>
            <FieldLabel htmlFor="bulk-add-tags">{t("addTags")}</FieldLabel>
            <Input id="bulk-add-tags" value={addTags} placeholder={t("listHint")} onChange={(e) => setAddTags(e.target.value)} />
          </Field>

          <Field>
            <FieldLabel htmlFor="bulk-remove-tags">{t("removeTags")}</FieldLabel>
            <Input id="bulk-remove-tags" value={removeTags} placeholder={t("listHint")} onChange={(e) => setRemoveTags(e.target.value)} />
          </Field>
        </div>

        <DialogFooter>
          <DialogClose render={<Button variant="ghost">{t("cancel")}</Button>} />
          <Button variant="primary" disabled={!changed || !ageValid || saving || seriesIds.length === 0} onClick={submit}>
            {t("submit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
