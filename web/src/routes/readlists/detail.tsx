import { useRef, useState } from "react";
import { DotTrack } from "@/components/media/dot-track";
import { Link, useNavigate, useParams } from "react-router";
import { ArrowDown, ArrowUp, ListMinus, MoreHorizontal, Play, SquarePen } from "@keyline-icons/react";
import { toast } from "@yusufceng1z/ding-react";
import { api, errorMessage, useApi, type Book } from "@/api";
import { BrandAction } from "@/components/media/brand-action";
import { Cover } from "@/components/media/cover";
import { EmptyState, Page, PageHeader } from "@/components/media/layout";
import { Block } from "@/components/media/skeletons";
import { LoadError } from "@/components/shell/load-error";
import { Button } from "@/components/ui/button";
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownContent, DropdownMenu, DropdownTrigger } from "@/components/ui/dropdown";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { MenuItem } from "@/components/ui/menu-item";
import { Tooltip } from "@/components/ui/tooltip";
import { FluidHoverHighlight } from "@/components/fluid-hover-highlight";
import { useFluidHover, useRegisterFluidHoverItem } from "@/hooks/use-fluid-hover";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";

export default function Screen() {
  const { readListId = "" } = useParams();
  const navigate = useNavigate();
  const [dialog, setDialog] = useState<"rename" | "delete" | null>(null);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);

  const { data, error, reload } = useApi(async () => {
    const rl = await api.readList(readListId);
    if (!rl) return { rl: null, books: [] as Book[], titles: new Map<string, string>(), isAdmin: false };
    const [books, me] = await Promise.all([api.readListBooks(readListId), api.me()]);
    // Series titles only for the series these books belong to.
    const seriesIds = [...new Set(books.map((b) => b.seriesId))];
    const series = await api.seriesByIds(seriesIds);
    return {
      rl,
      books,
      titles: new Map(series.map((s) => [s.id, s.title])),
      isAdmin: me.role === "admin",
    };
  }, [readListId]);

  if (error) {
    return (
      <Page>
        <LoadError message={error} onRetry={reload} />
      </Page>
    );
  }

  if (!data) {
    return (
      <Page>
        <Block className="mb-6 mt-2 h-8 w-56" />
        <div className="flex flex-col gap-2">
          {Array.from({ length: 6 }, (_, i) => (
            <Block key={i} className="h-14 w-full rounded-[8px]" />
          ))}
        </div>
      </Page>
    );
  }

  const { rl, books, titles, isAdmin } = data;
  if (!rl) {
    return (
      <Page>
        <EmptyState title="リーディングリストが見つかりません" />
      </Page>
    );
  }

  const next = books.find((b) => !b.progress?.completed) ?? books[0];

  const openDialog = (kind: "rename" | "delete") => {
    setName(rl.name);
    setDialog(kind);
  };

  const save = () => {
    setBusy(true);
    api
      .updateReadList(rl.id, { name: name.trim() })
      .then(() => {
        toast.success("名前を変更しました");
        setDialog(null);
        reload();
      })
      .catch((err: unknown) => toast.error(errorMessage(err)))
      .finally(() => setBusy(false));
  };

  const remove = () => {
    setBusy(true);
    api
      .deleteReadList(rl.id)
      .then(() => {
        toast.success("削除しました");
        navigate("/readlists");
      })
      .catch((err: unknown) => toast.error(errorMessage(err)))
      .finally(() => setBusy(false));
  };

  // Saves a new member list (a reorder or a removal), then reloads the list.
  const saveBooks = (bookIds: string[]) => {
    setBusy(true);
    api
      .updateReadList(rl.id, { bookIds })
      .then(() => reload())
      .catch((err: unknown) => toast.error(errorMessage(err)))
      .finally(() => setBusy(false));
  };

  const moveBook = (index: number, direction: -1 | 1) => {
    const ids = books.map((b) => b.id);
    const to = index + direction;
    if (to < 0 || to >= ids.length) return;
    [ids[index], ids[to]] = [ids[to], ids[index]];
    saveBooks(ids);
  };

  const removeBook = (bookId: string) => {
    saveBooks(books.filter((b) => b.id !== bookId).map((b) => b.id));
  };

  return (
    <Page>
      <PageHeader
        back={{ to: "/readlists", label: "リーディングリスト" }}
        title={rl.name}
        meta={rl.summary || undefined}
        actions={
          <>
            {next && <BrandAction to={`/read/${next.id}`} icon={Play}>最初から読む</BrandAction>}
            {isAdmin && (
              <DropdownMenu>
                <Tooltip content="その他の操作">
                  <span className="inline-flex">
                    <DropdownTrigger render={<Button variant="ghost" size="icon" aria-label="その他の操作" />}>
                      <MoreHorizontal size={16} />
                    </DropdownTrigger>
                  </span>
                </Tooltip>
                <DropdownContent align="end">
                  <MenuItem index={0} label="名前を変更" icon={SquarePen} onSelect={() => openDialog("rename")} />
                  <MenuItem index={1} label="削除" onSelect={() => openDialog("delete")} />
                </DropdownContent>
              </DropdownMenu>
            )}
          </>
        }
      />

      <Dialog open={dialog !== null} onOpenChange={(o) => !o && setDialog(null)}>
        <DialogContent size="sm">
          {dialog === "rename" ? (
            <>
              <DialogHeader>
                <DialogTitle>名前を変更</DialogTitle>
              </DialogHeader>
              <Field>
                <FieldLabel htmlFor="readlist-name">名前</FieldLabel>
                <Input id="readlist-name" value={name} onChange={(e) => setName(e.target.value)} />
              </Field>
              <DialogFooter>
                <DialogClose render={<Button variant="ghost">キャンセル</Button>} />
                <Button variant="primary" disabled={!name.trim() || busy} onClick={save}>
                  保存
                </Button>
              </DialogFooter>
            </>
          ) : (
            <>
              <DialogHeader>
                <DialogTitle>リーディングリストを削除</DialogTitle>
              </DialogHeader>
              <p className={cn(typeClass("body"), "text-muted-foreground")}>
                「{rl.name}」を削除します。巻は削除されません。
              </p>
              <DialogFooter>
                <DialogClose render={<Button variant="ghost">キャンセル</Button>} />
                <Button variant="destructive" disabled={busy} onClick={remove}>
                  削除
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>

      {books.length === 0 ? (
        <EmptyState
          title="巻がありません"
          body={isAdmin ? "巻の「その他」メニューの「リーディングリストに追加」から追加できます。" : undefined}
        />
      ) : (
        <BookRows books={books} titles={titles} editable={isAdmin} busy={busy} onMove={moveBook} onRemove={removeBook} />
      )}
    </Page>
  );
}

function BookRows({
  books,
  titles,
  editable,
  busy,
  onMove,
  onRemove,
}: {
  books: Book[];
  titles: Map<string, string>;
  editable: boolean;
  busy: boolean;
  onMove: (index: number, direction: -1 | 1) => void;
  onRemove: (bookId: string) => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const hover = useFluidHover(ref);
  return (
    <div ref={ref} className="relative" {...hover.handlers}>
      <FluidHoverHighlight hover={hover} className="rounded-[8px]" />
      <ol>
        {books.map((b, i) => (
          <BookRow
            key={b.id}
            book={b}
            index={i}
            count={books.length}
            seriesTitle={titles.get(b.seriesId)}
            register={hover.registerItem}
            editable={editable}
            busy={busy}
            onMove={onMove}
            onRemove={onRemove}
          />
        ))}
      </ol>
    </div>
  );
}

function BookRow({
  book,
  index,
  count,
  seriesTitle,
  register,
  editable,
  busy,
  onMove,
  onRemove,
}: {
  book: Book;
  index: number;
  count: number;
  seriesTitle?: string;
  register: (index: number, element: HTMLElement | null) => void;
  editable: boolean;
  busy: boolean;
  onMove: (index: number, direction: -1 | 1) => void;
  onRemove: (bookId: string) => void;
}) {
  const ref = useRef<HTMLAnchorElement>(null);
  useRegisterFluidHoverItem(register, index, ref);
  const p = book.progress;
  const pct = p ? Math.min(100, Math.round((p.page / book.pageCount) * 100)) : 0;

  return (
    <li className="flex items-center gap-1">
      <Link
        ref={ref}
        to={`/books/${book.id}`}
        className="relative z-10 flex min-w-0 flex-1 items-center gap-3 rounded-[8px] px-2 py-2 outline-none focus-visible:ring-2 ring-[var(--focus-ring)]"
      >
        <span className={cn(typeClass("body"), "w-6 shrink-0 text-right text-muted-foreground tabular-nums")}>{index + 1}</span>
        <div className="w-10 shrink-0">
          <Cover src={book.thumbnailUrl} title={seriesTitle ?? book.title} label={String(book.number)} />
        </div>
        <div className="min-w-0 flex-1 space-y-0.5">
          <p className={cn(typeClass("caption"), "truncate text-muted-foreground")}>{seriesTitle}</p>
          <p className={cn(typeClass("body"), "truncate text-foreground")}>{book.title}</p>
        </div>
        <div className="w-24 shrink-0 text-right">
          {p?.completed ? (
            <span className={cn(typeClass("caption"), "text-muted-foreground")}>既読</span>
          ) : p ? (
            <div className="space-y-1">
              <DotTrack total={10} value={Math.round(pct / 10)} label="読書の進捗" size="sm" className="justify-end" />
              <p className={cn(typeClass("caption"), "font-mono text-muted-foreground tabular-nums")}>
                {p.page}/{book.pageCount}ページ
              </p>
            </div>
          ) : (
            <span className={cn(typeClass("caption"), "text-muted-foreground")}>未読</span>
          )}
        </div>
      </Link>
      {editable && (
        <div className="relative z-10 flex shrink-0 items-center">
          <Tooltip content="上へ">
            <Button
              variant="ghost"
              size="icon"
              className="touch-hit relative"
              aria-label="上へ"
              disabled={busy || index === 0}
              onClick={() => onMove(index, -1)}
            >
              <ArrowUp size={16} />
            </Button>
          </Tooltip>
          <Tooltip content="下へ">
            <Button
              variant="ghost"
              size="icon"
              className="touch-hit relative"
              aria-label="下へ"
              disabled={busy || index === count - 1}
              onClick={() => onMove(index, 1)}
            >
              <ArrowDown size={16} />
            </Button>
          </Tooltip>
          <Tooltip content="リストから外す">
            <Button
              variant="ghost"
              size="icon"
              className="touch-hit relative"
              aria-label="リストから外す"
              disabled={busy}
              onClick={() => onRemove(book.id)}
            >
              <ListMinus size={16} />
            </Button>
          </Tooltip>
        </div>
      )}
    </li>
  );
}
