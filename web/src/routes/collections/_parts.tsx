import { useState, type ReactNode } from "react";
import { Link, useNavigate } from "react-router";
import { toast } from "@yusufceng1z/ding-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsItem } from "@/components/ui/tabs";
import { Cover } from "@/components/media/cover";
import { Block } from "@/components/media/skeletons";
import { api, errorMessage } from "@/api";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";
import { defineMessages, useT } from "@/i18n";

const m = defineMessages({
  ja: {
    collections: "コレクション",
    readLists: "リーディングリスト",
    created: "作成しました",
    newButton: "新規作成",
    createCollection: "コレクションを作成",
    createReadList: "リーディングリストを作成",
    name: "名前",
    manualOrder: "手動で並べる",
    cancel: "キャンセル",
    create: "作成",
  },
  en: {
    collections: "Collections",
    readLists: "Read lists",
    created: "Created",
    newButton: "New",
    createCollection: "Create collection",
    createReadList: "Create read list",
    name: "Name",
    manualOrder: "Order manually",
    cancel: "Cancel",
    create: "Create",
  },
});

/** Segment between collections and reading lists; each side is its own route. */
export function ViewSwitch({ current }: { current: "collections" | "readlists" }) {
  const t = useT(m);
  const navigate = useNavigate();
  return (
    <Tabs
      idPrefix="list-view"
      className="mb-6"
      selectedIndex={current === "collections" ? 0 : 1}
      onSelect={(i) => navigate(i === 0 ? "/collections" : "/readlists")}
    >
      <TabsItem index={0} label={t("collections")} />
      <TabsItem index={1} label={t("readLists")} />
    </Tabs>
  );
}

/** Creates a collection or reading list, then opens it. Administrators only (the caller hides it). */
export function NewListDialog({ kind }: { kind: "collection" | "readlist" }) {
  const t = useT(m);
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [manual, setManual] = useState(false);
  const [busy, setBusy] = useState(false);
  const isCollection = kind === "collection";

  const submit = () => {
    setBusy(true);
    const created: Promise<{ id: string }> = isCollection
      ? api.createCollection({ name: name.trim(), ordered: manual })
      : api.createReadList({ name: name.trim() });
    created
      .then((item) => {
        toast.success(t("created"));
        setName("");
        setManual(false);
        setOpen(false);
        navigate(isCollection ? `/collections/${item.id}` : `/readlists/${item.id}`);
      })
      .catch((err: unknown) => toast.error(errorMessage(err)))
      .finally(() => setBusy(false));
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button variant="secondary">{t("newButton")}</Button>} />
      <DialogContent size="sm">
        <DialogHeader>
          <DialogTitle>{isCollection ? t("createCollection") : t("createReadList")}</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <Field>
            <FieldLabel htmlFor={`new-${kind}-name`}>{t("name")}</FieldLabel>
            <Input id={`new-${kind}-name`} value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
          {isCollection && <Switch label={t("manualOrder")} checked={manual} onToggle={() => setManual((m) => !m)} />}
        </div>
        <DialogFooter>
          <DialogClose render={<Button variant="ghost">{t("cancel")}</Button>} />
          <Button variant="primary" disabled={!name.trim() || busy} onClick={submit}>
            {t("create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export interface StackCover {
  src: string;
  title: string;
  label?: string;
}

/**
 * Up to four covers stacked down and to the right. The front cover sits at
 * the bottom-right slot; each cover behind it steps up-left and is darker.
 */
export function CoverStack({ covers }: { covers: StackCover[] }) {
  const shown = covers.slice(0, 4);
  if (shown.length === 0) {
    return (
      <div className="relative aspect-[5/6]">
        <div className="absolute left-0 top-0 w-[60%]">
          <Block className="aspect-[2/3] w-full rounded-[6px]" />
        </div>
      </div>
    );
  }
  const n = shown.length;
  return (
    <div className="relative aspect-[5/6]">
      {shown.map((c, i) => {
        const slot = 4 - n + (n - 1 - i); // i=0 is the front cover -> slot 3
        const depth = 3 - slot;
        return (
          <div
            key={i}
            className="absolute w-[60%]"
            style={{ left: `${(slot / 3) * 40}%`, top: `${(slot / 3) * 25}%`, zIndex: slot }}
          >
            <Cover src={c.src} title={c.title} label={c.label} />
            {depth > 0 && (
              <span
                aria-hidden
                className="pointer-events-none absolute inset-0 rounded-[6px]"
                style={{ backgroundColor: `rgb(0 0 0 / ${0.2 * depth})` }}
              />
            )}
          </div>
        );
      })}
    </div>
  );
}

/** Whole card is one link: stacked covers, then name and count. */
export function ListCard({ to, name, count, covers }: { to: string; name: string; count: string; covers: StackCover[] }) {
  return (
    <Link
      to={to}
      className="group block rounded-[8px] outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)] focus-visible:ring-offset-2 focus-visible:ring-offset-background"
    >
      <div className="transition-transform duration-160 ease-[cubic-bezier(0.16,1,0.3,1)] group-hover:-translate-y-0.5 motion-reduce:transform-none">
        <CoverStack covers={covers} />
      </div>
      <div className="mt-2 space-y-0.5 px-0.5">
        <p className={cn(typeClass("subtitle"), "line-clamp-2")} style={{ fontVariationSettings: fontWeights.semibold }}>
          {name}
        </p>
        <p className={cn(typeClass("caption"), "text-muted-foreground")}>{count}</p>
      </div>
    </Link>
  );
}

export function ListGrid({ children }: { children: ReactNode }) {
  return <div className="grid grid-cols-[repeat(auto-fill,minmax(200px,1fr))] gap-x-4 gap-y-8">{children}</div>;
}

export function ListGridSkeleton() {
  return (
    <ListGrid>
      {Array.from({ length: 8 }, (_, i) => (
        <div key={i} aria-hidden>
          <Block className="aspect-[5/6] w-full rounded-[8px]" />
          <Block className="mt-2 h-4 w-4/5" />
          <Block className="mt-1 h-3 w-1/2" />
        </div>
      ))}
    </ListGrid>
  );
}
