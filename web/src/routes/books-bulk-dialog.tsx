import { useState } from "react";
import { toast } from "@yusufceng1z/ding-react";
import { api, errorMessage } from "@/api";
import { inChunks, splitList } from "@/components/media/selection-state";
import { useBulkDialog } from "@/components/media/use-bulk-dialog";
import { Button } from "@/components/ui/button";
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { defineMessages, useT } from "@/i18n";

const m = defineMessages({
  ja: {
    title: "タグを編集",
    intro: "選んだ巻にタグを追加、または外します。変更したタグはロックされ、スキャンで上書きされません。",
    addTags: "タグを追加",
    removeTags: "タグを外す",
    listHint: "カンマで区切って入力",
    cancel: "キャンセル",
    submit: "更新",
    updated: ({ n }) => `${n} 件を更新しました`,
  },
  en: {
    title: "Edit tags",
    intro: "Add or remove tags on the books you picked. Changed tags get locked, so a scan will not overwrite them.",
    addTags: "Add tags",
    removeTags: "Remove tags",
    listHint: "Separate with commas",
    cancel: "Cancel",
    submit: "Update",
    updated: ({ n }) => `Updated ${n}`,
  },
});

/** Adds or removes tags on many books (admin). The parent mounts it fresh per open. */
export function BooksBulkDialog({
  bookIds,
  open,
  onOpenChange,
  onDone,
}: {
  bookIds: string[];
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
  const [addTags, setAddTags] = useState("");
  const [removeTags, setRemoveTags] = useState("");
  const [saving, setSaving] = useState(false);

  const add = splitList(addTags);
  const remove = splitList(removeTags);
  const changed = add.length > 0 || remove.length > 0;

  // A failed save may have saved earlier chunks; useBulkDialog reports that
  // through onDone (see the prop comment).
  const bulk = useBulkDialog(onOpenChange, onDone);

  const submit = async () => {
    if (!changed || saving || bookIds.length === 0) return;
    setSaving(true);
    try {
      const n = await inChunks(bookIds, (chunk) =>
        api.bulkUpdateBooks({ bookIds: chunk, addTags: add, removeTags: remove }).then((r) => r.updated),
      );
      toast.success(t("updated", { n }));
      bulk.saved();
    } catch (err) {
      toast.error(errorMessage(err));
      bulk.saveFailed();
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={bulk.close}>
      <DialogContent size="sm">
        <DialogHeader>
          <DialogTitle>{t("title")}</DialogTitle>
          <DialogDescription>{t("intro")}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          <Field>
            <FieldLabel htmlFor="bulk-book-add-tags">{t("addTags")}</FieldLabel>
            <Input id="bulk-book-add-tags" value={addTags} placeholder={t("listHint")} onChange={(e) => setAddTags(e.target.value)} />
          </Field>
          <Field>
            <FieldLabel htmlFor="bulk-book-remove-tags">{t("removeTags")}</FieldLabel>
            <Input
              id="bulk-book-remove-tags"
              value={removeTags}
              placeholder={t("listHint")}
              onChange={(e) => setRemoveTags(e.target.value)}
            />
          </Field>
        </div>

        <DialogFooter>
          <DialogClose render={<Button variant="ghost">{t("cancel")}</Button>} />
          <Button variant="primary" disabled={!changed || saving || bookIds.length === 0} onClick={submit}>
            {t("submit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
