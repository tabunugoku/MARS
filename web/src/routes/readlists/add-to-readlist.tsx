import { useEffect, useRef, useState } from "react";
import { toast } from "@yusufceng1z/ding-react";
import { Plus } from "@keyline-icons/react";
import { api, errorMessage, type ReadList } from "@/api";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import type { IconComponent } from "@/lib/icon-context";
import { defineMessages, useT } from "@/i18n";

const m = defineMessages({
  ja: {
    created: "リーディングリストを作成しました",
    title: "リーディングリストに追加",
    loading: "読み込んでいます",
    empty: "リーディングリストがありません。下から作成できます。",
    newName: "新しいリーディングリスト",
    create: "作成",
    close: "閉じる",
  },
  en: {
    created: "Read list created",
    title: "Add to read list",
    loading: "Loading",
    empty: "No read lists yet. Create one below.",
    newName: "New read list",
    create: "Create",
    close: "Close",
  },
});

/** Adds one book to read lists. Admin only (the server refuses other users'
 *  list writes). Mounted fresh on each open (parent uses a key), so the lists
 *  are fetched once per open. Each checkbox saves at once; a failed save is
 *  rolled back and reported with a toast. */
export function AddToReadListDialog({
  bookId,
  open,
  onOpenChange,
}: {
  bookId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useT(m);
  const [lists, setLists] = useState<ReadList[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [savingId, setSavingId] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [creating, setCreating] = useState(false);
  // Lists created here. A fetch that resolves after a create keeps them.
  const createdRef = useRef<ReadList[]>([]);
  // Set synchronously, so Enter twice (or Enter and the button) cannot create two.
  const creatingRef = useRef(false);

  useEffect(() => {
    if (!open) return;
    let live = true;
    api
      .readLists()
      .then((ls) => {
        if (!live) return;
        setLists(createdRef.current.reduce((xs, c) => (xs.some((y) => y.id === c.id) ? xs : [...xs, c]), ls));
      })
      .catch((err: unknown) => live && setError(errorMessage(err)));
    return () => {
      live = false;
    };
  }, [open]);

  const replaceRow = (row: ReadList) => setLists((ls) => ls?.map((l) => (l.id === row.id ? row : l)) ?? null);

  const toggle = (list: ReadList, on: boolean) => {
    const previous = list;
    const optimistic = on ? [...list.bookIds, bookId] : list.bookIds.filter((id) => id !== bookId);
    setSavingId(list.id);
    replaceRow({ ...list, bookIds: optimistic });
    const call = on ? api.addReadListBook(list.id, bookId) : api.removeReadListBook(list.id, bookId);
    call
      // The returned list is the server's view: it replaces the row.
      .then((updated) => replaceRow(updated))
      .catch((err: unknown) => {
        replaceRow(previous);
        toast.error(errorMessage(err));
      })
      .finally(() => setSavingId(null));
  };

  const create = () => {
    const n = name.trim();
    if (!n || !lists || creatingRef.current) return;
    creatingRef.current = true;
    setCreating(true);
    api
      .createReadList({ name: n, bookIds: [bookId] })
      .then((created) => {
        toast.success(t("created"));
        setName("");
        createdRef.current = [...createdRef.current.filter((l) => l.id !== created.id), created];
        setLists((ls) => {
          const xs = ls ?? [];
          return xs.some((l) => l.id === created.id) ? xs.map((l) => (l.id === created.id ? created : l)) : [...xs, created];
        });
      })
      .catch((err: unknown) => toast.error(errorMessage(err)))
      .finally(() => {
        creatingRef.current = false;
        setCreating(false);
      });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="sm">
        <DialogHeader>
          <DialogTitle>{t("title")}</DialogTitle>
        </DialogHeader>

        {error ? (
          <p className={cn(typeClass("body"), "text-muted-foreground")}>{error}</p>
        ) : !lists ? (
          <p className={cn(typeClass("body"), "text-muted-foreground")}>{t("loading")}</p>
        ) : lists.length === 0 ? (
          <p className={cn(typeClass("body"), "text-muted-foreground")}>{t("empty")}</p>
        ) : (
          <ul className="max-h-[50dvh] divide-y divide-border overflow-y-auto">
            {lists.map((l) => {
              const checked = l.bookIds.includes(bookId);
              return (
                <li key={l.id}>
                  <label className="flex min-h-11 cursor-pointer items-center gap-3 py-2">
                    <Checkbox
                      checked={checked}
                      disabled={savingId !== null || creating}
                      onCheckedChange={(c) => toggle(l, c === true)}
                    />
                    <span className={cn(typeClass("body"), "min-w-0 flex-1 truncate text-foreground")}>{l.name}</span>
                    <span className={cn(typeClass("caption"), "font-mono text-muted-foreground tabular-nums")}>{l.bookIds.length}</span>
                  </label>
                </li>
              );
            })}
          </ul>
        )}

        <div className="space-y-2 border-t border-border pt-4">
          <Field>
            <FieldLabel htmlFor="new-readlist-name">{t("newName")}</FieldLabel>
            <div className="flex items-center gap-2">
              <Input
                id="new-readlist-name"
                className="flex-1"
                value={name}
                disabled={!lists || creating}
                onChange={(e) => setName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && !e.nativeEvent.isComposing && e.keyCode !== 229) create();
                }}
              />
              <Button
                variant="secondary"
                leadingIcon={Plus as IconComponent}
                disabled={!name.trim() || !lists || creating}
                onClick={create}
              >
                {t("create")}
              </Button>
            </div>
          </Field>
        </div>

        <DialogFooter>
          <DialogClose render={<Button variant="ghost">{t("close")}</Button>} />
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
