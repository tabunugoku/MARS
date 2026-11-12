import { useState, type ReactNode } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { ChevronLeft, ChevronRight, Play } from "@keyline-icons/react";
import { toast } from "@yusufceng1z/ding-react";
import { api, bookFileUrl, errorMessage, formatBytes, formatDate, startDownload, useApi, type Author, type MediaStatus } from "@/api";
import { LoadError } from "@/components/shell/load-error";
import { Cover } from "@/components/media/cover";
import { BrandAction } from "@/components/media/brand-action";
import { DotTrack } from "@/components/media/dot-track";
import { EmptyState, Page, SectionTitle } from "@/components/media/layout";
import { Block } from "@/components/media/skeletons";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DropdownContent, DropdownMenu, DropdownTrigger } from "@/components/ui/dropdown";
import { MenuItem } from "@/components/ui/menu-item";
import { Tooltip } from "@/components/ui/tooltip";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";

const PREVIEW_PAGES = 24;

const roleLabel: Record<Author["role"], string> = {
  writer: "作",
  penciller: "画",
  colorist: "色",
  translator: "翻訳",
  editor: "編集",
  cover: "表紙",
};

const mediaLabel: Record<MediaStatus, string> = {
  ready: "正常",
  error: "エラー",
  unsupported: "非対応",
  outdated: "要再スキャン",
};

export default function Screen() {
  const { bookId = "" } = useParams();
  // Keyed by id so the read-state override resets when the route changes.
  return <BookPage key={bookId} id={bookId} />;
}

function BookPage({ id }: { id: string }) {
  const navigate = useNavigate();
  const { data, error, reload } = useApi(async () => {
    const book = await api.book(id);
    if (!book) return null;
    const [series, adjacent, pages] = await Promise.all([api.seriesById(book.seriesId), api.adjacentBooks(id), api.pages(id).catch(() => [])]);
    return { book, series, adjacent, pages };
  }, [id]);

  const [allPages, setAllPages] = useState(false);
  const [busy, setBusy] = useState(false);
  const { data: me } = useApi(() => api.me(), []);
  const isAdmin = me?.role === "admin";
  const canDownload = isAdmin || !!me?.canDownload;
  // Bumped after the thumbnail is made again, so the <img> fetches it anew.
  const [coverV, setCoverV] = useState(0);
  const [picking, setPicking] = useState(false);

  if (error) {
    return (
      <Page>
        <LoadError message={error} onRetry={reload} />
      </Page>
    );
  }

  if (data === undefined) {
    return (
      <Page>
        <div className="flex flex-col gap-6 md:flex-row md:gap-8" aria-hidden>
          <Block className="mx-auto aspect-[2/3] w-[160px] shrink-0 rounded-[6px] md:mx-0 md:w-[200px]" />
          <div className="flex min-w-0 flex-1 flex-col gap-3 pt-1">
            <Block className="h-4 w-24" />
            <Block className="h-8 w-2/3" />
            <Block className="h-4 w-1/2" />
            <div className="flex flex-wrap gap-1.5">{Array.from({ length: 24 }, (_, i) => <Block key={i} className="size-2 rounded-full" />)}</div>
            <Block className="mt-2 h-9 w-40" />
          </div>
        </div>
      </Page>
    );
  }

  if (!data) {
    return (
      <Page>
        <EmptyState title="巻が見つかりません" body="シリーズからもう一度探してください" />
      </Page>
    );
  }

  const { book, series, adjacent } = data;
  const completed = !!book.progress?.completed;
  const page = book.progress?.page ?? 0;
  const pct = book.pageCount ? Math.min(100, Math.round((page / book.pageCount) * 100)) : 0;
  const dots = Math.min(book.pageCount, 60);
  const shownPages = allPages ? data.pages.length : Math.min(PREVIEW_PAGES, data.pages.length);
  const authors = book.authors.map((a) => `${a.name}（${roleLabel[a.role]}）`).join("、");
  // Marking read needs the page count and a readable archive.
  const canMarkRead = book.pageCount > 0 && book.mediaStatus === "ready";

  const regenerate = async () => {
    try {
      await api.regenerateBookThumbnail(book.id);
      setCoverV((v) => v + 1);
      toast.success("表紙を作り直しました");
    } catch (err) {
      toast.error(errorMessage(err));
    }
  };

  const chooseCover = async (n: number) => {
    setBusy(true);
    try {
      await api.setBookCover(book.id, n);
      setPicking(false);
      setCoverV((v) => v + 1);
      toast.success(`${n}ページ目を表紙にしました`);
      reload();
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  // その他 menu, numbered in the order shown.
  const menu: { label: string; onSelect: () => void }[] = [
    { label: "シークレットモードで読む", onSelect: () => navigate(`/read/${book.id}?incognito=1`) },
  ];
  if (canDownload) menu.push({ label: "ファイルをダウンロード", onSelect: () => startDownload(bookFileUrl(book.id)) });
  if (isAdmin && book.mediaStatus === "ready") {
    menu.push({ label: "表紙を作り直す", onSelect: regenerate });
    menu.push({
      label: "表紙のページを選ぶ",
      onSelect: () => {
        setPicking(true);
        document.getElementById("pages")?.scrollIntoView({ behavior: "smooth", block: "start" });
      },
    });
  }

  const toggleRead = async () => {
    setBusy(true);
    try {
      if (completed) await api.clearProgress(book.id);
      else await api.setProgress(book.id, book.pageCount, true);
      toast.success(completed ? "未読にしました" : "既読にしました");
      reload();
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Page>
      <div className="mb-4 flex items-center justify-between gap-2">
        <Link
          to={`/series/${book.seriesId}`}
          className={cn(typeClass("caption"), "inline-flex min-w-0 items-center gap-1 text-muted-foreground hover:text-foreground pointer-coarse:-my-3 pointer-coarse:py-3")}
        >
          <ChevronLeft size={14} className="shrink-0" />
          <span className="truncate">{series?.title ?? "シリーズ"}</span>
        </Link>
        <div className="flex shrink-0 items-center gap-1">
          <Tooltip content="前の巻">
            <Button
              variant="ghost"
              size="icon"
              className="pointer-coarse:h-11 pointer-coarse:w-11"
              aria-label="前の巻"
              disabled={!adjacent.prev}
              onClick={() => adjacent.prev && navigate(`/books/${adjacent.prev.id}`)}
            >
              <ChevronLeft />
            </Button>
          </Tooltip>
          <Tooltip content="次の巻">
            <Button
              variant="ghost"
              size="icon"
              className="pointer-coarse:h-11 pointer-coarse:w-11"
              aria-label="次の巻"
              disabled={!adjacent.next}
              onClick={() => adjacent.next && navigate(`/books/${adjacent.next.id}`)}
            >
              <ChevronRight />
            </Button>
          </Tooltip>
        </div>
      </div>

      <div className="flex flex-col gap-6 md:flex-row md:gap-8">
        <div className="mx-auto w-[160px] shrink-0 md:mx-0 md:w-[200px]">
          <Cover src={coverV && book.thumbnailUrl.startsWith("/") ? `${book.thumbnailUrl}?v=${coverV}` : book.thumbnailUrl} title={series?.title ?? book.title} label={String(book.number)} priority />
        </div>

        <div className="flex min-w-0 flex-1 flex-col gap-3">
          <h1 className={cn(typeClass("display"), "break-words")} style={{ fontVariationSettings: fontWeights.bold }}>
            {book.title}
          </h1>
          <p className={cn(typeClass("body"), "font-mono text-muted-foreground")}>
            第{book.number}巻 · {book.pageCount}ページ · {book.format.toUpperCase()} · {formatBytes(book.sizeBytes)}
          </p>

          <div className="mt-1 space-y-1.5">
            {completed ? (
              <Badge variant="dot">既読</Badge>
            ) : book.progress ? (
              <>
                <p className={cn(typeClass("caption"), "font-mono text-muted-foreground tabular-nums")}>
                  {page} / {book.pageCount} ページ
                </p>
                <DotTrack total={dots} value={Math.round((pct / 100) * dots)} label="読書の進捗" />
              </>
            ) : null}
          </div>

          <div className="mt-2 flex flex-wrap items-center gap-2">
            <BrandAction to={`/read/${book.id}`} icon={Play}>
              {book.progress && !completed ? "続きから読む" : "読む"}
            </BrandAction>
            <Button variant="secondary" asChild>
              <Link to={`/read/${book.id}?page=1`}>最初から読む</Link>
            </Button>
            <Button variant="secondary" disabled={busy || !canMarkRead} onClick={toggleRead}>
              {completed ? "未読にする" : "既読にする"}
            </Button>
            <DropdownMenu>
              <DropdownTrigger render={<Button variant="secondary">その他</Button>} />
              <DropdownContent align="start" sideOffset={4}>
                {menu.map((m, i) => (
                  <MenuItem key={m.label} index={i} label={m.label} onSelect={m.onSelect} />
                ))}
              </DropdownContent>
            </DropdownMenu>
          </div>
          {!canMarkRead && <p className={cn(typeClass("caption"), "text-muted-foreground")}>解析が終わっていません</p>}

          <dl className="mt-4 max-w-2xl divide-y divide-border">
            <Fact label="発売日">{book.releaseDate ? formatDate(book.releaseDate) : "未設定"}</Fact>
            <Fact label="ISBN">{book.isbn || "未設定"}</Fact>
            <Fact label="著者">{authors || "未設定"}</Fact>
          </dl>
        </div>
      </div>

      <section id="pages" className="mt-10 scroll-mt-20">
        <SectionTitle>ページ</SectionTitle>
        {picking && (
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <p className={cn(typeClass("body"), "text-muted-foreground")}>表紙にするページを選んでください</p>
            <Button variant="ghost" size="compact" className="pointer-coarse:h-11" onClick={() => setPicking(false)}>
              やめる
            </Button>
          </div>
        )}
        <div className="grid grid-cols-[repeat(auto-fill,minmax(96px,1fr))] gap-3">
          {Array.from({ length: shownPages }, (_, i) => i + 1).map((n) => (
            <PageTile
              key={n}
              to={picking ? undefined : `/read/${book.id}?page=${n}`}
              onPick={picking && !busy ? () => chooseCover(n) : undefined}
              label={picking ? `${n}ページ目を表紙にする` : undefined}
            >
              <img
                src={data.pages[n - 1].url}
                alt=""
                loading="lazy"
                decoding="async"
                draggable={false}
                className="aspect-[2/3] w-full rounded-[6px] bg-muted object-cover transition-transform duration-160 ease-[cubic-bezier(0.16,1,0.3,1)] group-hover:-translate-y-0.5 motion-reduce:transform-none"
                style={{ boxShadow: "var(--cover-shadow)" }}
              />
              <span className={cn(typeClass("micro"), "mt-1 block text-center text-muted-foreground tabular-nums")}>
                {n}
                {n === book.coverPage && "・表紙"}
              </span>
            </PageTile>
          ))}
        </div>
        {!allPages && book.pageCount > PREVIEW_PAGES && (
          <Button variant="secondary" className="mt-4" onClick={() => setAllPages(true)}>
            すべてのページを表示（{book.pageCount}）
          </Button>
        )}
      </section>

      <section className="mt-10">
        <SectionTitle>ファイル</SectionTitle>
        <dl className="max-w-2xl divide-y divide-border">
          <Fact label="パス">
            <span className="break-all font-mono">{book.path}</span>
          </Fact>
          <Fact label="形式">{book.format.toUpperCase()}</Fact>
          <Fact label="サイズ">{formatBytes(book.sizeBytes)}</Fact>
          <Fact label="メディア状態">
            <div className="flex flex-wrap items-center gap-2">
              {book.mediaStatus === "error" ? (
                <Badge variant="solid" color="red">
                  {mediaLabel[book.mediaStatus]}
                </Badge>
              ) : (
                <Badge variant="dot">{mediaLabel[book.mediaStatus]}</Badge>
              )}
              {book.mediaStatus === "error" && <span className="text-muted-foreground">アーカイブを開けません</span>}
            </div>
          </Fact>
        </dl>
      </section>
    </Page>
  );
}

const tileClass =
  "group block w-full rounded-[6px] text-left outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)] focus-visible:ring-offset-2 focus-visible:ring-offset-background";

/** A page in the preview grid: a link into the reader, or, while choosing
 *  a cover, a button that makes it the cover. */
function PageTile({ to, onPick, label, children }: { to?: string; onPick?: () => void; label?: string; children: ReactNode }) {
  if (to) {
    return (
      <Link to={to} className={tileClass}>
        {children}
      </Link>
    );
  }
  return (
    <button type="button" className={cn(tileClass, "cursor-pointer disabled:cursor-default")} disabled={!onPick} aria-label={label} onClick={onPick}>
      {children}
    </button>
  );
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[7rem_1fr] gap-4 py-2.5">
      <dt className={cn(typeClass("caption"), "text-muted-foreground")}>{label}</dt>
      <dd className={typeClass("body")}>{children}</dd>
    </div>
  );
}
