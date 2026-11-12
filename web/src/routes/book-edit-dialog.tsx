import type { IconComponent } from "@/lib/icon-context";
import { useState, type ReactNode } from "react";
import { toast } from "@yusufceng1z/ding-react";
import { Lock, Plus, Unlock, X } from "@keyline-icons/react";
import { api, errorMessage, type Author, type Book, type BookUpdate } from "@/api";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Tabs, TabsItem, TabsPanel } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { Tooltip } from "@/components/ui/tooltip";
import { Select, SelectContent, SelectItem, SelectTrigger } from "@/components/ui/select";
import { roleLabel } from "@/routes/series/edit-dialog";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { defineMessages, translate, useT } from "@/i18n";

const m = defineMessages({
  ja: {
    title: "メタデータを編集",
    tabBasic: "基本",
    tabAuthors: "著者",
    tabTags: "タグ",
    fieldTitle: "タイトル",
    fieldNumber: "巻数",
    fieldNumberSort: "並び順の巻数",
    fieldReleaseDate: "発売日",
    fieldIsbn: "ISBN",
    fieldSummary: "あらすじ",
    fieldAuthors: "著者",
    fieldTags: "タグ",
    numberSortNote: "並び順は巻数のロックに従います",
    numberInvalid: "巻数は数字で入力してください",
    numberSortInvalid: "並び順の巻数は数字で入力してください",
    releaseDateInvalid: "発売日は 2026-11-12、2026-11、2026 の形で入力してください",
    saved: "メタデータを保存しました",
    authorName: "著者名",
    role: "役割",
    removeAuthor: "著者を削除",
    addAuthor: "著者を追加",
    separate: "カンマで区切って入力",
    cancel: "キャンセル",
    save: "保存",
    keptOnScan: "スキャンで上書きされません",
    overwrittenOnScan: "スキャンで上書きされます",
    lockField: "{name}をロック",
  },
  en: {
    title: "Edit metadata",
    tabBasic: "Basic",
    tabAuthors: "Authors",
    tabTags: "Tags",
    fieldTitle: "Title",
    fieldNumber: "Number",
    fieldNumberSort: "Sort number",
    fieldReleaseDate: "Release date",
    fieldIsbn: "ISBN",
    fieldSummary: "Summary",
    fieldAuthors: "Authors",
    fieldTags: "Tags",
    numberSortNote: "Sort number follows the number lock",
    numberInvalid: "Number must be a number",
    numberSortInvalid: "Sort number must be a number",
    releaseDateInvalid: "Release date must look like 2026-11-12, 2026-11 or 2026",
    saved: "Metadata saved",
    authorName: "Author name",
    role: "Role",
    removeAuthor: "Remove author",
    addAuthor: "Add author",
    separate: "Separate with commas",
    cancel: "Cancel",
    save: "Save",
    keptOnScan: "Scan won't overwrite this",
    overwrittenOnScan: "Scan can overwrite this",
    lockField: "Lock {name}",
  },
});

/** Fields the server can lock for a book (its bookLockNames). numberSort is
 *  locked under "number", so it has no lock of its own. */
const LOCKABLE = ["title", "number", "summary", "releaseDate", "isbn", "authors", "tags"];

function fieldLabel(): Record<string, string> {
  return {
    title: translate(m, "fieldTitle"),
    number: translate(m, "fieldNumber"),
    summary: translate(m, "fieldSummary"),
    releaseDate: translate(m, "fieldReleaseDate"),
    isbn: translate(m, "fieldIsbn"),
    authors: translate(m, "fieldAuthors"),
    tags: translate(m, "fieldTags"),
  };
}

type Form = {
  title: string;
  number: string;
  numberSort: string;
  summary: string;
  releaseDate: string;
  isbn: string;
  authors: Author[];
  tags: string;
};

function initialForm(b: Book): Form {
  return {
    title: b.title,
    number: String(b.number),
    numberSort: String(b.numberSort),
    summary: b.summary,
    releaseDate: b.releaseDate ?? "",
    isbn: b.isbn,
    authors: b.authors.map((a) => ({ ...a })),
    tags: b.tags.join(", "),
  };
}

const splitList = (raw: string) =>
  raw
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean);
const sameList = (a: string[], b: string[]) => a.length === b.length && a.every((x, i) => x === b[i]);

/** A finite number, or undefined when the text is not one. */
function parseNumber(raw: string): number | undefined {
  const t = raw.trim();
  if (t === "") return undefined;
  const n = Number(t);
  return Number.isFinite(n) ? n : undefined;
}

/** YYYY-MM-DD, YYYY-MM or YYYY. The server checks the calendar date itself. */
const RELEASE_DATE = /^\d{4}(-\d{2}(-\d{2})?)?$/;

/** Only the fields the user changed, compared with the book as loaded.
 *  The server locks every field it receives, so unchanged fields must stay
 *  out. Throws a message when a number is not one or a date is malformed. */
function changesOf(form: Form, b: Book, locked: string[]): BookUpdate {
  const c: BookUpdate = {};
  if (form.title !== b.title) c.title = form.title;

  const num = parseNumber(form.number);
  if (num === undefined) throw new Error(translate(m, "numberInvalid"));
  if (num !== b.number) c.number = num;
  const sort = parseNumber(form.numberSort);
  if (sort === undefined) throw new Error(translate(m, "numberSortInvalid"));
  if (sort !== b.numberSort) c.numberSort = sort;

  if (form.summary !== b.summary) c.summary = form.summary;

  // Empty clears the date (null); otherwise it must have one of the three shapes.
  const date = form.releaseDate.trim();
  if (date !== "" && !RELEASE_DATE.test(date)) throw new Error(translate(m, "releaseDateInvalid"));
  const next = date === "" ? null : date;
  if (next !== (b.releaseDate || null)) c.releaseDate = next;

  if (form.isbn !== b.isbn) c.isbn = form.isbn;

  const authors = form.authors
    .map((a) => ({ name: a.name.trim(), role: a.role }))
    .filter((a) => a.name !== "");
  const sameAuthors =
    authors.length === b.authors.length && authors.every((a, i) => a.name === b.authors[i].name && a.role === b.authors[i].role);
  if (!sameAuthors) c.authors = authors;

  const tags = splitList(form.tags);
  if (!sameList(tags, b.tags)) c.tags = tags;

  // Lock list: sent only when the user toggled a lock. It replaces the whole
  // list, so fields the dialog does not show keep their locks, and the fields
  // being saved stay locked (saving a field locks it on the server).
  const lockChanged = !sameList([...locked].sort(), [...b.lockedFields].sort());
  if (lockChanged) {
    const hidden = b.lockedFields.filter((f) => !LOCKABLE.includes(f));
    const edited = Object.keys(c).map((k) => (k === "numberSort" ? "number" : k));
    c.lockedFields = [...new Set([...hidden, ...locked, ...edited])];
  }
  return c;
}

/** Metadata editor for one book (administrators). Mounted fresh on each open
 *  (parent uses a key), so the form starts from the book's current values.
 *  Save sends only the changed fields; onSaved lets the parent reload. */
export function BookEditDialog({
  book,
  open,
  onOpenChange,
  onSaved,
}: {
  book: Book;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}) {
  const t = useT(m);
  const [form, setForm] = useState<Form>(() => initialForm(book));
  const [locked, setLocked] = useState<string[]>(book.lockedFields);
  const [tab, setTab] = useState(0);
  const [saving, setSaving] = useState(false);
  const roleNames = roleLabel();
  const roles = Object.keys(roleNames) as Author["role"][];
  const names = fieldLabel();

  const set = <K extends keyof Form>(key: K, value: Form[K]) => setForm((f) => ({ ...f, [key]: value }));
  const toggleLock = (field: string) =>
    setLocked((l) => (l.includes(field) ? l.filter((x) => x !== field) : [...l, field]));
  const setAuthor = (i: number, patch: Partial<Author>) =>
    set(
      "authors",
      form.authors.map((a, j) => (j === i ? { ...a, ...patch } : a)),
    );

  const save = async () => {
    let changes: BookUpdate;
    try {
      changes = changesOf(form, book, locked);
    } catch (err) {
      toast.error(errorMessage(err));
      return;
    }
    if (Object.keys(changes).length === 0) {
      onOpenChange(false);
      return;
    }
    setSaving(true);
    try {
      await api.updateBook(book.id, changes);
      toast.success(t("saved"));
      onSaved();
      onOpenChange(false);
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const lockProps = { locked, onToggle: toggleLock };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>{t("title")}</DialogTitle>
        </DialogHeader>

        <Tabs selectedIndex={tab} onSelect={setTab} idPrefix="bmeta">
          <TabsItem index={0} label={t("tabBasic")} />
          <TabsItem index={1} label={t("tabAuthors")} />
          <TabsItem index={2} label={t("tabTags")} />
        </Tabs>

        <div className="max-h-[60dvh] overflow-y-auto">
          <TabsPanel index={0} selectedIndex={tab} idPrefix="bmeta" className="grid gap-4 md:grid-cols-2">
            <LockedField id="bmeta-title" field="title" name={names.title} className="md:col-span-2" {...lockProps}>
              <Input id="bmeta-title" value={form.title} onChange={(e) => set("title", e.target.value)} />
            </LockedField>
            <LockedField id="bmeta-number" field="number" name={names.number} {...lockProps}>
              <Input
                id="bmeta-number"
                type="number"
                inputMode="decimal"
                step="any"
                value={form.number}
                onChange={(e) => set("number", e.target.value)}
              />
            </LockedField>
            <Field>
              <FieldLabel htmlFor="bmeta-number-sort">{t("fieldNumberSort")}</FieldLabel>
              <Input
                id="bmeta-number-sort"
                type="number"
                inputMode="decimal"
                step="any"
                value={form.numberSort}
                onChange={(e) => set("numberSort", e.target.value)}
              />
              <p className={cn(typeClass("caption"), "text-muted-foreground")}>{t("numberSortNote")}</p>
            </Field>
            <LockedField id="bmeta-release" field="releaseDate" name={names.releaseDate} {...lockProps}>
              <Input
                id="bmeta-release"
                placeholder="2026-11-12"
                value={form.releaseDate}
                onChange={(e) => set("releaseDate", e.target.value)}
              />
            </LockedField>
            <LockedField id="bmeta-isbn" field="isbn" name={names.isbn} {...lockProps}>
              <Input id="bmeta-isbn" value={form.isbn} onChange={(e) => set("isbn", e.target.value)} />
            </LockedField>
            <LockedField id="bmeta-summary" field="summary" name={names.summary} className="md:col-span-2" {...lockProps}>
              <Textarea id="bmeta-summary" rows={4} value={form.summary} onChange={(e) => set("summary", e.target.value)} />
            </LockedField>
          </TabsPanel>

          <TabsPanel index={1} selectedIndex={tab} idPrefix="bmeta" className="space-y-3">
            <div className="flex items-center justify-between gap-2">
              <span className={cn(typeClass("caption"), "text-muted-foreground")}>{names.authors}</span>
              <LockToggle field="authors" name={names.authors} {...lockProps} />
            </div>
            {form.authors.map((a, i) => (
              <div key={i} className="flex items-center gap-2">
                <Input
                  aria-label={t("authorName")}
                  className="flex-1"
                  value={a.name}
                  onChange={(e) => setAuthor(i, { name: e.target.value })}
                />
                <Select value={a.role} onValueChange={(v) => setAuthor(i, { role: v as Author["role"] })}>
                  <SelectTrigger aria-label={t("role")} className="w-28" />
                  <SelectContent>
                    {roles.map((r, j) => (
                      <SelectItem key={r} index={j} value={r}>
                        {roleNames[r]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Tooltip content={t("removeAuthor")}>
                  <Button
                    variant="ghost"
                    size="icon-compact"
                    aria-label={t("removeAuthor")}
                    onClick={() => set("authors", form.authors.filter((_, j) => j !== i))}
                  >
                    <X />
                  </Button>
                </Tooltip>
              </div>
            ))}
            <Button
              variant="secondary"
              leadingIcon={Plus as IconComponent}
              onClick={() => set("authors", [...form.authors, { name: "", role: "writer" }])}
            >
              {t("addAuthor")}
            </Button>
          </TabsPanel>

          <TabsPanel index={2} selectedIndex={tab} idPrefix="bmeta" className="grid gap-4">
            <LockedField id="bmeta-tags" field="tags" name={names.tags} {...lockProps}>
              <Input id="bmeta-tags" placeholder={t("separate")} value={form.tags} onChange={(e) => set("tags", e.target.value)} />
            </LockedField>
          </TabsPanel>
        </div>

        <DialogFooter>
          <Button variant="secondary" disabled={saving} onClick={() => onOpenChange(false)}>
            {t("cancel")}
          </Button>
          <Button disabled={saving} onClick={save}>
            {t("save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** The lock toggle. Locked fields keep their value on scan. */
function LockToggle({
  field,
  name,
  locked,
  onToggle,
}: {
  field: string;
  name: string;
  locked: string[];
  onToggle: (field: string) => void;
}) {
  const t = useT(m);
  const isLocked = locked.includes(field);
  return (
    <Tooltip content={isLocked ? t("keptOnScan") : t("overwrittenOnScan")}>
      <Button
        variant="ghost"
        size="icon-compact"
        aria-label={t("lockField", { name })}
        aria-pressed={isLocked}
        className={cn(isLocked && "text-foreground")}
        onClick={() => onToggle(field)}
      >
        {isLocked ? <Lock /> : <Unlock />}
      </Button>
    </Tooltip>
  );
}

/** A labelled control with a lock toggle. */
function LockedField({
  id,
  field,
  name,
  locked,
  onToggle,
  className,
  children,
}: {
  id: string;
  field: string;
  name: string;
  locked: string[];
  onToggle: (field: string) => void;
  className?: string;
  children: ReactNode;
}) {
  return (
    <Field className={className}>
      <div className="flex items-center justify-between gap-2">
        <FieldLabel htmlFor={id}>{name}</FieldLabel>
        <LockToggle field={field} name={name} locked={locked} onToggle={onToggle} />
      </div>
      {children}
    </Field>
  );
}
