import { useEffect, useRef, useState } from "react";
import { toast } from "@yusufceng1z/ding-react";
import { api, errorMessage, type Collection } from "@/api";
import { LoadError } from "@/components/shell/load-error";
import { Block } from "@/components/media/skeletons";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";

/** Adds this series to collections or takes it out. Each checkbox saves at once.
 *  Administrators only: the caller shows the menu item to them alone. */
export function AddToCollectionDialog({
  seriesId,
  open,
  onOpenChange,
}: {
  seriesId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="sm">
        <DialogHeader>
          <DialogTitle>コレクションに追加</DialogTitle>
        </DialogHeader>
        <CollectionChecks seriesId={seriesId} open={open} />
      </DialogContent>
    </Dialog>
  );
}

/** Replaces the item with the same id, or appends it when there is none. */
function mergeById<T extends { id: string }>(xs: T[], x: T): T[] {
  return xs.some((y) => y.id === x.id) ? xs.map((y) => (y.id === x.id ? x : y)) : [...xs, x];
}

function CollectionChecks({ seriesId, open }: { seriesId: string; open: boolean }) {
  const [lists, setLists] = useState<Collection[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [saving, setSaving] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [creating, setCreating] = useState(false);
  // Lists created here. A fetch that resolves after a create keeps them.
  const createdRef = useRef<Collection[]>([]);
  // Set synchronously, so Enter twice (or Enter and the button) cannot create two.
  const creatingRef = useRef(false);

  // Fetched on each open: the dialog content stays mounted while closed.
  useEffect(() => {
    if (!open) return;
    let live = true;
    api
      .collections()
      .then((cs) => {
        if (!live) return;
        setError(null);
        // Lists created before this fetch resolved are merged in, not overwritten.
        setLists(createdRef.current.reduce((xs, c) => (xs.some((y) => y.id === c.id) ? xs : [...xs, c]), cs));
      })
      .catch((err: unknown) => live && setError(errorMessage(err)));
    return () => {
      live = false;
    };
  }, [open, attempt]);

  const replaceRow = (row: Collection) => setLists((ls) => ls?.map((x) => (x.id === row.id ? row : x)) ?? null);

  const toggle = (c: Collection, on: boolean) => {
    const previous = c;
    const optimistic = on
      ? [...c.seriesIds, seriesId]
      : c.seriesIds.filter((id) => id !== seriesId);
    setSaving(c.id);
    replaceRow({ ...c, seriesIds: optimistic });
    const call = on ? api.addCollectionSeries(c.id, seriesId) : api.removeCollectionSeries(c.id, seriesId);
    call
      // The returned list is the server's view: it replaces the row.
      .then((updated) => replaceRow(updated))
      .catch((err: unknown) => {
        // Roll the checkbox back to what it was before the failed save.
        replaceRow(previous);
        toast.error(errorMessage(err));
      })
      .finally(() => setSaving(null));
  };

  const create = () => {
    const n = name.trim();
    if (!n || !lists || creatingRef.current) return;
    creatingRef.current = true;
    setCreating(true);
    api
      .createCollection({ name: n, ordered: false, seriesIds: [seriesId] })
      .then((created) => {
        createdRef.current = mergeById(createdRef.current, created);
        setLists((ls) => mergeById(ls ?? [], created));
        setName("");
        toast.success("コレクションを作成しました");
      })
      .catch((err: unknown) => toast.error(errorMessage(err)))
      .finally(() => {
        creatingRef.current = false;
        setCreating(false);
      });
  };

  if (error) return <LoadError message={error} onRetry={() => { setError(null); setAttempt((n) => n + 1); }} />;
  if (!lists) return <Block className="h-24 w-full" />;

  const busy = saving !== null || creating;

  return (
    <div className="flex flex-col gap-4">
      {lists.length === 0 ? (
        <p className={cn(typeClass("body"), "text-muted-foreground")}>コレクションはまだありません。</p>
      ) : (
        <ul className="flex flex-col">
          {lists.map((c) => (
            <li key={c.id}>
              <label
                className={cn(
                  typeClass("body"),
                  "flex min-h-9 cursor-pointer items-center gap-3 py-1 text-foreground pointer-coarse:min-h-11",
                  busy && "cursor-progress",
                )}
              >
                <Checkbox
                  checked={c.seriesIds.includes(seriesId)}
                  disabled={busy}
                  onCheckedChange={(on) => toggle(c, on)}
                />
                <span className="min-w-0 flex-1 truncate">{c.name}</span>
              </label>
            </li>
          ))}
        </ul>
      )}

      <Field>
        <FieldLabel htmlFor="add-to-collection-new">新しいコレクション</FieldLabel>
        <div className="flex items-center gap-2">
          <Input
            id="add-to-collection-new"
            className="min-w-0 flex-1"
            value={name}
            placeholder="名前"
            disabled={creating}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.nativeEvent.isComposing && e.keyCode !== 229) create();
            }}
          />
          <Button variant="secondary" disabled={!name.trim() || busy} onClick={create}>
            作成
          </Button>
        </div>
      </Field>
    </div>
  );
}
