import { useState } from "react";
import { toast } from "@yusufceng1z/ding-react";
import { api, errorMessage, useApi, type Collection } from "@/api";
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

function CollectionChecks({ seriesId, open }: { seriesId: string; open: boolean }) {
  // Fetched on each open: the dialog content stays mounted while closed.
  const { data, error, reload } = useApi<Collection[] | null>(
    () => (open ? api.collections() : Promise.resolve(null)),
    [seriesId, open],
  );
  // Values set by this dialog's own saves, keyed by collection id. They
  // win over the loaded list until the dialog closes and reopens.
  const [checks, setChecks] = useState<Record<string, boolean>>({});
  const [saving, setSaving] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [creating, setCreating] = useState(false);

  const isOn = (c: Collection) => checks[c.id] ?? c.seriesIds.includes(seriesId);

  const toggle = (c: Collection, on: boolean) => {
    const previous = isOn(c);
    const seriesIds = on ? [...c.seriesIds, seriesId] : c.seriesIds.filter((id) => id !== seriesId);
    setChecks((m) => ({ ...m, [c.id]: on }));
    setSaving(c.id);
    api
      .updateCollection(c.id, { seriesIds })
      .then(() => reload())
      .catch((err: unknown) => {
        // Roll the checkbox back to what it was before the failed save.
        setChecks((m) => ({ ...m, [c.id]: previous }));
        toast.error(errorMessage(err));
      })
      .finally(() => setSaving(null));
  };

  const create = () => {
    const n = name.trim();
    if (!n) return;
    setCreating(true);
    api
      .createCollection({ name: n, ordered: false, seriesIds: [seriesId] })
      .then(() => {
        setName("");
        toast.success("コレクションを作成しました");
        reload();
      })
      .catch((err: unknown) => toast.error(errorMessage(err)))
      .finally(() => setCreating(false));
  };

  if (error) return <LoadError message={error} onRetry={reload} />;
  if (!data) return <Block className="h-24 w-full" />;

  const busy = saving !== null || creating;

  return (
    <div className="flex flex-col gap-4">
      {data.length === 0 ? (
        <p className={cn(typeClass("body"), "text-muted-foreground")}>コレクションはまだありません。</p>
      ) : (
        <ul className="flex flex-col">
          {data.map((c) => (
            <li key={c.id}>
              <label
                className={cn(
                  typeClass("body"),
                  "flex min-h-9 cursor-pointer items-center gap-3 py-1 text-foreground pointer-coarse:min-h-11",
                  busy && "cursor-progress",
                )}
              >
                <Checkbox
                  checked={isOn(c)}
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
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.nativeEvent.isComposing) create();
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
