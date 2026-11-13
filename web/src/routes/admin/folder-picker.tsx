import { useCallback, useEffect, useState } from "react";
import { ArrowUp, ChevronRight, Folder } from "@keyline-icons/react";
import { api, ApiError, errorMessage, type DirectoryListing } from "@/api";
import { Block } from "@/components/media/skeletons";
import { Button } from "@/components/ui/button";
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { toast } from "@yusufceng1z/ding-react";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { defineMessages, useT } from "@/i18n";
import { IconAction } from "./_parts";

const m = defineMessages({
  ja: {
    title: "フォルダを選ぶ",
    up: "1つ上のフォルダへ",
    current: "現在のフォルダ",
    unreadable: "読めません",
    empty: "フォルダはありません",
    notFound: "入力されたパスは見つかりませんでした。ルートを表示しています",
    use: "このフォルダを選ぶ",
    cancel: "キャンセル",
  },
  en: {
    title: "Choose a folder",
    up: "Up one folder",
    current: "Current folder",
    unreadable: "Not readable",
    empty: "No folders here",
    notFound: "The typed path was not found. Showing the root.",
    use: "Use this folder",
    cancel: "Cancel",
  },
});

/** Breadcrumb crumbs for a path: the root (the volume on Windows) first, then each folder. */
function crumbsOf(path: string): Crumb[] {
  if (path.startsWith("/")) {
    let acc = "";
    return [
      { label: "/", path: "/" },
      ...path.split("/").filter(Boolean).map((part) => {
        acc += "/" + part;
        return { label: part, path: acc };
      }),
    ];
  }
  // Windows: the first segment is the volume ("C:"), the root of its tree is "C:\".
  const [volume, ...parts] = path.split(/[\\/]+/).filter(Boolean);
  let acc = volume + "\\";
  return [
    { label: volume, path: acc },
    ...parts.map((part) => {
      acc = acc.endsWith("\\") ? acc + part : acc + "\\" + part;
      return { label: part, path: acc };
    }),
  ];
}

type Crumb = { label: string; path: string };

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The folder the picker starts in (the typed path; empty: the root). */
  initialPath: string;
  onPick: (path: string) => void;
};

export default function FolderPicker({ open, onOpenChange, initialPath, onPick }: Props) {
  const t = useT(m);
  const [listing, setListing] = useState<DirectoryListing | null>(null);
  const [loading, setLoading] = useState(false);
  const [notFound, setNotFound] = useState(false);

  // fallback: a typed path that the server cannot use falls back to the root.
  const load = useCallback(async (path: string, fallback: boolean) => {
    setLoading(true);
    try {
      setListing(await api.directories(path));
      setNotFound(false);
    } catch (err) {
      if (fallback && err instanceof ApiError && (err.status === 400 || err.status === 404)) {
        setNotFound(true);
        try {
          setListing(await api.directories(""));
        } catch (rootErr) {
          toast.error(errorMessage(rootErr));
        }
      } else {
        toast.error(errorMessage(err));
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!open) return;
    const start = initialPath.trim();
    void load(start, start !== "");
  }, [open, initialPath, load]);

  const crumbs = listing ? crumbsOf(listing.path) : [];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>{t("title")}</DialogTitle>
        </DialogHeader>

        <div className="flex min-w-0 items-center gap-1">
          {listing?.parent != null && (
            <IconAction label={t("up")} icon={ArrowUp} onClick={() => !loading && load(listing.parent!, false)} />
          )}
          <nav aria-label={t("current")} className="flex min-w-0 items-center gap-1 overflow-x-auto">
            {crumbs.map((c, i) => {
              const last = i === crumbs.length - 1;
              return (
                <span key={c.path} className="flex items-center gap-1 whitespace-nowrap">
                  {i > 0 && (
                    <span className="text-muted-foreground" aria-hidden>
                      <ChevronRight size={14} />
                    </span>
                  )}
                  {last ? (
                    <span
                      aria-current="page"
                      className={cn(typeClass("caption"), "font-mono text-foreground")}
                    >
                      {c.label}
                    </span>
                  ) : (
                    <button
                      type="button"
                      disabled={loading}
                      onClick={() => load(c.path, false)}
                      className={cn(
                        typeClass("caption"),
                        "touch-hit relative rounded px-1 font-mono text-muted-foreground hover:text-foreground focus-visible:ring-2 ring-[var(--focus-ring)] disabled:cursor-not-allowed",
                      )}
                    >
                      {c.label}
                    </button>
                  )}
                </span>
              );
            })}
          </nav>
        </div>

        {notFound && <p className={cn(typeClass("caption"), "text-muted-foreground")}>{t("notFound")}</p>}

        <div className="max-h-[320px] overflow-y-auto rounded-[12px] border border-border">
          {loading && (
            <div className="flex flex-col gap-1 p-2" aria-busy>
              {Array.from({ length: 4 }, (_, i) => (
                <div key={i} className="flex items-center gap-2 px-2 py-2">
                  <Block className="h-4 w-4 shrink-0" />
                  <Block className="h-4 w-2/3" />
                </div>
              ))}
            </div>
          )}

          {!loading && listing && listing.directories.length === 0 && (
            <p className={cn(typeClass("body"), "px-4 py-8 text-center text-muted-foreground")}>{t("empty")}</p>
          )}

          {!loading && listing && listing.directories.length > 0 && (
            <ul className="flex flex-col p-1">
              {listing.directories.map((d) => (
                <li key={d.path}>
                  <button
                    type="button"
                    disabled={!d.readable || loading}
                    aria-disabled={!d.readable}
                    onClick={() => load(d.path, false)}
                    className={cn(
                      "touch-hit relative flex min-h-9 w-full items-center gap-2 rounded-[8px] px-3 py-1.5 text-left",
                      "focus-visible:ring-2 ring-[var(--focus-ring)]",
                      d.readable
                        ? "text-foreground hover:bg-muted"
                        : "cursor-not-allowed text-muted-foreground",
                    )}
                  >
                    <span className="shrink-0 text-muted-foreground" aria-hidden>
                      <Folder size={16} />
                    </span>
                    <span className={cn(typeClass("body"), "min-w-0 flex-1 truncate")}>{d.name}</span>
                    {!d.readable && <span className={cn(typeClass("caption"), "shrink-0")}>{t("unreadable")}</span>}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <DialogFooter>
          <DialogClose render={<Button variant="ghost">{t("cancel")}</Button>} />
          <Button
            variant="primary"
            disabled={!listing || loading}
            onClick={() => {
              if (!listing) return;
              onPick(listing.path);
              onOpenChange(false);
            }}
          >
            {t("use")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
