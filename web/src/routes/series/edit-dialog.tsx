import type { IconComponent } from "@/lib/icon-context";
import { useState, type ReactNode } from "react";
import { toast } from "@yusufceng1z/ding-react";
import { Lock, Plus, Unlock, X } from "@keyline-icons/react";
import {
  api,
  errorMessage,
  statusLabel,
  type Author,
  type ReadingDirection,
  type Series,
  type SeriesStatus,
  type SeriesUpdate,
} from "@/api";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger } from "@/components/ui/select";
import { Tabs, TabsItem, TabsPanel } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { Tooltip } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

export const directionLabel: Record<ReadingDirection, string> = {
  rtl: "右から左",
  ltr: "左から右",
  vertical: "縦",
  webtoon: "縦スクロール",
};

export const roleLabel: Record<Author["role"], string> = {
  writer: "作",
  penciller: "画",
  colorist: "色",
  translator: "翻訳",
  editor: "編集",
  cover: "表紙",
};

export const fieldLabel: Record<string, string> = {
  title: "タイトル",
  titleSort: "ソート用タイトル",
  summary: "あらすじ",
  status: "ステータス",
  readingDirection: "読み方向",
  publisher: "出版社",
  language: "言語",
  ageRating: "年齢区分",
  totalBookCount: "総巻数",
};

const statuses = Object.keys(statusLabel) as SeriesStatus[];
const directions = Object.keys(directionLabel) as ReadingDirection[];
const roles = Object.keys(roleLabel) as Author["role"][];

type Form = {
  title: string;
  titleSort: string;
  summary: string;
  status: SeriesStatus;
  readingDirection: ReadingDirection;
  publisher: string;
  language: string;
  ageRating: string;
  totalBookCount: string;
  authors: Author[];
  genres: string;
  tags: string;
};

function initialForm(s: Series): Form {
  return {
    title: s.title,
    titleSort: s.titleSort,
    summary: s.summary,
    status: s.status,
    readingDirection: s.readingDirection,
    publisher: s.publisher,
    language: s.language,
    ageRating: s.ageRating?.toString() ?? "",
    totalBookCount: s.totalBookCount?.toString() ?? "",
    authors: s.authors.map((a) => ({ ...a })),
    genres: s.genres.join(", "),
    tags: s.tags.join(", "),
  };
}

const splitList = (raw: string) =>
  raw
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean);
const sameList = (a: string[], b: string[]) => a.length === b.length && a.every((x, i) => x === b[i]);

/** "" clears the field (null); otherwise a whole number. undefined when the text is not one. */
function parseOptionalInt(raw: string): number | null | undefined {
  const t = raw.trim();
  if (t === "") return null;
  const n = Number(t);
  return Number.isInteger(n) ? n : undefined;
}

/** Only the fields the user changed, compared with the series as loaded.
 *  The server locks every field it receives, so unchanged fields must stay
 *  out. Throws a message when a number field is not a whole number. */
function changesOf(form: Form, s: Series, locked: string[]): SeriesUpdate {
  const c: SeriesUpdate = {};
  if (form.title !== s.title) c.title = form.title;
  if (form.titleSort !== s.titleSort) c.titleSort = form.titleSort;
  if (form.summary !== s.summary) c.summary = form.summary;
  if (form.status !== s.status) c.status = form.status;
  if (form.readingDirection !== s.readingDirection) c.readingDirection = form.readingDirection;
  if (form.publisher !== s.publisher) c.publisher = form.publisher;
  if (form.language !== s.language) c.language = form.language;

  const age = parseOptionalInt(form.ageRating);
  if (age === undefined) throw new Error("年齢区分は整数で入力してください");
  if (age !== s.ageRating) c.ageRating = age;
  const total = parseOptionalInt(form.totalBookCount);
  if (total === undefined) throw new Error("総巻数は整数で入力してください");
  if (total !== s.totalBookCount) c.totalBookCount = total;

  const authors = form.authors
    .map((a) => ({ name: a.name.trim(), role: a.role }))
    .filter((a) => a.name !== "");
  const sameAuthors =
    authors.length === s.authors.length && authors.every((a, i) => a.name === s.authors[i].name && a.role === s.authors[i].role);
  if (!sameAuthors) c.authors = authors;

  const genres = splitList(form.genres);
  if (!sameList(genres, s.genres)) c.genres = genres;
  const tags = splitList(form.tags);
  if (!sameList(tags, s.tags)) c.tags = tags;

  // Lock list: sent only when the user toggled a lock. It replaces the whole
  // list, so fields the dialog does not show keep their locks, and the fields
  // being saved stay locked (saving a field locks it on the server).
  const lockChanged = !sameList([...locked].sort(), [...s.lockedFields].sort());
  if (lockChanged) {
    const shown = Object.keys(fieldLabel);
    const hidden = s.lockedFields.filter((f) => !shown.includes(f));
    c.lockedFields = [...new Set([...hidden, ...locked, ...Object.keys(c)])];
  }
  return c;
}

/** Metadata editor. Mounted fresh on each open (parent uses a key), so the
 *  form always starts from the series' current values. Save sends only the
 *  changed fields; the saved series goes back to the parent through onSaved. */
export function EditDialog({
  series,
  open,
  onOpenChange,
  onSaved,
}: {
  series: Series;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSaved: (series: Series) => void;
}) {
  const [form, setForm] = useState<Form>(() => initialForm(series));
  const [locked, setLocked] = useState<string[]>(series.lockedFields);
  const [tab, setTab] = useState(0);
  const [saving, setSaving] = useState(false);

  const set = <K extends keyof Form>(key: K, value: Form[K]) => setForm((f) => ({ ...f, [key]: value }));
  const toggleLock = (field: string) =>
    setLocked((l) => (l.includes(field) ? l.filter((x) => x !== field) : [...l, field]));
  const setAuthor = (i: number, patch: Partial<Author>) =>
    set(
      "authors",
      form.authors.map((a, j) => (j === i ? { ...a, ...patch } : a)),
    );

  const save = async () => {
    let changes: SeriesUpdate;
    try {
      changes = changesOf(form, series, locked);
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
      const updated = await api.updateSeries(series.id, changes);
      toast.success("メタデータを保存しました");
      onSaved(updated);
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
          <DialogTitle>メタデータを編集</DialogTitle>
        </DialogHeader>

        <Tabs selectedIndex={tab} onSelect={setTab} idPrefix="meta">
          <TabsItem index={0} label="基本" />
          <TabsItem index={1} label="著者" />
          <TabsItem index={2} label="タグとジャンル" />
        </Tabs>

        <div className="max-h-[60dvh] overflow-y-auto">
          <TabsPanel index={0} selectedIndex={tab} idPrefix="meta" className="grid gap-4 md:grid-cols-2">
            <LockedField id="meta-title" field="title" {...lockProps}>
              <Input id="meta-title" value={form.title} onChange={(e) => set("title", e.target.value)} />
            </LockedField>
            <LockedField id="meta-title-sort" field="titleSort" {...lockProps}>
              <Input id="meta-title-sort" value={form.titleSort} onChange={(e) => set("titleSort", e.target.value)} />
            </LockedField>
            <LockedField id="meta-summary" field="summary" className="md:col-span-2" {...lockProps}>
              <Textarea id="meta-summary" rows={4} value={form.summary} onChange={(e) => set("summary", e.target.value)} />
            </LockedField>
            <LockedField id="meta-status" field="status" {...lockProps}>
              <Select value={form.status} onValueChange={(v) => set("status", v as SeriesStatus)}>
                <SelectTrigger id="meta-status" />
                <SelectContent>
                  {statuses.map((s, i) => (
                    <SelectItem key={s} index={i} value={s}>
                      {statusLabel[s]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </LockedField>
            <LockedField id="meta-direction" field="readingDirection" {...lockProps}>
              <Select value={form.readingDirection} onValueChange={(v) => set("readingDirection", v as ReadingDirection)}>
                <SelectTrigger id="meta-direction" />
                <SelectContent>
                  {directions.map((d, i) => (
                    <SelectItem key={d} index={i} value={d}>
                      {directionLabel[d]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </LockedField>
            <LockedField id="meta-publisher" field="publisher" {...lockProps}>
              <Input id="meta-publisher" value={form.publisher} onChange={(e) => set("publisher", e.target.value)} />
            </LockedField>
            <LockedField id="meta-language" field="language" {...lockProps}>
              <Input id="meta-language" value={form.language} onChange={(e) => set("language", e.target.value)} />
            </LockedField>
            <LockedField id="meta-age" field="ageRating" {...lockProps}>
              <Input
                id="meta-age"
                type="number"
                inputMode="numeric"
                min={0}
                value={form.ageRating}
                onChange={(e) => set("ageRating", e.target.value)}
              />
            </LockedField>
            <LockedField id="meta-total" field="totalBookCount" {...lockProps}>
              <Input
                id="meta-total"
                type="number"
                inputMode="numeric"
                min={0}
                value={form.totalBookCount}
                onChange={(e) => set("totalBookCount", e.target.value)}
              />
            </LockedField>
          </TabsPanel>

          <TabsPanel index={1} selectedIndex={tab} idPrefix="meta" className="space-y-3">
            {form.authors.map((a, i) => (
              <div key={i} className="flex items-center gap-2">
                <Input
                  aria-label="著者名"
                  className="flex-1"
                  value={a.name}
                  onChange={(e) => setAuthor(i, { name: e.target.value })}
                />
                <Select value={a.role} onValueChange={(v) => setAuthor(i, { role: v as Author["role"] })}>
                  <SelectTrigger aria-label="役割" className="w-28" />
                  <SelectContent>
                    {roles.map((r, j) => (
                      <SelectItem key={r} index={j} value={r}>
                        {roleLabel[r]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Tooltip content="著者を削除">
                  <Button
                    variant="ghost"
                    size="icon-compact"
                    aria-label="著者を削除"
                    onClick={() => set("authors", form.authors.filter((_, j) => j !== i))}
                  >
                    <X />
                  </Button>
                </Tooltip>
              </div>
            ))}
            <Button variant="secondary" leadingIcon={Plus as IconComponent} onClick={() => set("authors", [...form.authors, { name: "", role: "writer" }])}>
              著者を追加
            </Button>
          </TabsPanel>

          <TabsPanel index={2} selectedIndex={tab} idPrefix="meta" className="grid gap-4">
            <Field>
              <FieldLabel htmlFor="meta-genres">ジャンル</FieldLabel>
              <Input id="meta-genres" placeholder="カンマで区切って入力" value={form.genres} onChange={(e) => set("genres", e.target.value)} />
            </Field>
            <Field>
              <FieldLabel htmlFor="meta-tags">タグ</FieldLabel>
              <Input id="meta-tags" placeholder="カンマで区切って入力" value={form.tags} onChange={(e) => set("tags", e.target.value)} />
            </Field>
          </TabsPanel>
        </div>

        <DialogFooter>
          <Button variant="secondary" disabled={saving} onClick={() => onOpenChange(false)}>
            キャンセル
          </Button>
          <Button disabled={saving} onClick={save}>
            保存
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** A labelled control with a lock toggle. Locked fields keep their value on scan. */
function LockedField({
  id,
  field,
  locked,
  onToggle,
  className,
  children,
}: {
  id: string;
  field: string;
  locked: string[];
  onToggle: (field: string) => void;
  className?: string;
  children: ReactNode;
}) {
  const isLocked = locked.includes(field);
  return (
    <Field className={className}>
      <div className="flex items-center justify-between gap-2">
        <FieldLabel htmlFor={id}>{fieldLabel[field]}</FieldLabel>
        <Tooltip content={isLocked ? "スキャンで上書きされません" : "スキャンで上書きされます"}>
          <Button
            variant="ghost"
            size="icon-compact"
            aria-label={`${fieldLabel[field]}をロック`}
            aria-pressed={isLocked}
            className={cn(isLocked && "text-foreground")}
            onClick={() => onToggle(field)}
          >
            {isLocked ? <Lock /> : <Unlock />}
          </Button>
        </Tooltip>
      </div>
      {children}
    </Field>
  );
}
