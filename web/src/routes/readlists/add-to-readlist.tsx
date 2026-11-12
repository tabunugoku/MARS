import { useEffect, useState } from "react";
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
  const [lists, setLists] = useState<ReadList[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [savingId, setSavingId] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    if (!open) return;
    let live = true;
    api
      .readLists()
      .then((ls) => live && setLists(ls))
      .catch((err: unknown) => live && setError(errorMessage(err)));
    return () => {
      live = false;
    };
  }, [open]);

  const toggle = (list: ReadList, on: boolean) => {
    const prev = list.bookIds;
    const next = on ? [...prev, bookId] : prev.filter((id) => id !== bookId);
    const patch = (bookIds: string[]) =>
      setLists((ls) => ls?.map((l) => (l.id === list.id ? { ...l, bookIds } : l)) ?? null);

    setSavingId(list.id);
    patch(next);
    api
      .updateReadList(list.id, { bookIds: next })
      .then((updated) => patch(updated.bookIds))
      .catch((err: unknown) => {
        patch(prev);
        toast.error(errorMessage(err));
      })
      .finally(() => setSavingId(null));
  };

  const create = () => {
    setCreating(true);
    api
      .createReadList({ name: name.trim(), bookIds: [bookId] })
      .then((created) => {
        toast.success("リーディングリストを作成しました");
        setName("");
        setLists((ls) => [...(ls ?? []), created]);
      })
      .catch((err: unknown) => toast.error(errorMessage(err)))
      .finally(() => setCreating(false));
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="sm">
        <DialogHeader>
          <DialogTitle>リーディングリストに追加</DialogTitle>
        </DialogHeader>

        {error ? (
          <p className={cn(typeClass("body"), "text-muted-foreground")}>{error}</p>
        ) : !lists ? (
          <p className={cn(typeClass("body"), "text-muted-foreground")}>読み込んでいます</p>
        ) : lists.length === 0 ? (
          <p className={cn(typeClass("body"), "text-muted-foreground")}>リーディングリストがありません。下から作成できます。</p>
        ) : (
          <ul className="max-h-[50dvh] divide-y divide-border overflow-y-auto">
            {lists.map((l) => {
              const checked = l.bookIds.includes(bookId);
              return (
                <li key={l.id}>
                  <label className="flex min-h-11 cursor-pointer items-center gap-3 py-2">
                    <Checkbox
                      checked={checked}
                      disabled={savingId !== null}
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
            <FieldLabel htmlFor="new-readlist-name">新しいリーディングリスト</FieldLabel>
            <div className="flex items-center gap-2">
              <Input
                id="new-readlist-name"
                className="flex-1"
                value={name}
                onChange={(e) => setName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && name.trim() && !creating) create();
                }}
              />
              <Button
                variant="secondary"
                leadingIcon={Plus as IconComponent}
                disabled={!name.trim() || creating}
                onClick={create}
              >
                作成
              </Button>
            </div>
          </Field>
        </div>

        <DialogFooter>
          <DialogClose render={<Button variant="ghost">閉じる</Button>} />
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
