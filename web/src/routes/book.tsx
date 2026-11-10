import { useState, type ReactNode } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { ChevronLeft, ChevronRight, Play } from "@keyline-icons/react";
import { toast } from "@yusufceng1z/ding-react";
import { api, formatBytes, formatDate, useApi, type Author, type MediaStatus } from "@/api";
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
import { pageArt } from "@/lib/art";
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
  const { data } = useApi(async () => {
    const book = await api.book(id);
    if (!book) return null;
    const [series, adjacent] = await Promise.all([api.seriesById(book.seriesId), api.adjacentBooks(id)]);
    return { book, series, adjacent };
  }, [id]);

  const [readOverride, setReadOverride] = useState<boolean | null>(null);
  const [allPages, setAllPages] = useState(false);

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
  const completed = readOverride ?? !!book.progress?.completed;
  const page = book.progress?.page ?? 0;
  const pct = book.pageCount ? Math.min(100, Math.round((page / book.pageCount) * 100)) : 0;
  const dots = Math.min(book.pageCount, 60);
  const shownPages = allPages ? book.pageCount : Math.min(PREVIEW_PAGES, book.pageCount);
  const authors = book.authors.map((a) => `${a.name}（${roleLabel[a.role]}）`).join("、");

  const toggleRead = () => {
    const next = !completed;
    setReadOverride(next);
    toast.success(next ? "既読にしました" : "未読にしました");
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
          <Cover seed={book.coverSeed} title={series?.title ?? book.title} label={String(book.number)} priority />
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
            <DropdownMenu>
              <DropdownTrigger render={<Button variant="ghost">その他</Button>} />
              <DropdownContent align="start" sideOffset={4}>
                <MenuItem index={0} label="シークレットモードで読む" onSelect={() => navigate(`/read/${book.id}?incognito=1`)} />
                <MenuItem index={1} label={completed ? "未読にする" : "既読にする"} onSelect={toggleRead} />
                <MenuItem index={2} label="ダウンロード" />
              </DropdownContent>
            </DropdownMenu>
          </div>

          <dl className="mt-4 max-w-2xl divide-y divide-border">
            <Fact label="発売日">{book.releaseDate ? formatDate(book.releaseDate) : "未設定"}</Fact>
            <Fact label="ISBN">{book.isbn || "未設定"}</Fact>
            <Fact label="著者">{authors || "未設定"}</Fact>
          </dl>
        </div>
      </div>

      <section className="mt-10">
        <SectionTitle>ページ</SectionTitle>
        <div className="grid grid-cols-[repeat(auto-fill,minmax(96px,1fr))] gap-3">
          {Array.from({ length: shownPages }, (_, i) => i + 1).map((n) => (
            <Link
              key={n}
              to={`/read/${book.id}?page=${n}`}
              className="group block rounded-[6px] outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)] focus-visible:ring-offset-2 focus-visible:ring-offset-background"
            >
              <img
                src={pageArt(book.coverSeed, n)}
                alt=""
                loading="lazy"
                decoding="async"
                draggable={false}
                className="aspect-[2/3] w-full rounded-[6px] bg-muted object-cover transition-transform duration-160 ease-[cubic-bezier(0.16,1,0.3,1)] group-hover:-translate-y-0.5 motion-reduce:transform-none"
                style={{ boxShadow: "var(--cover-shadow)" }}
              />
              <span className={cn(typeClass("micro"), "mt-1 block text-center text-muted-foreground tabular-nums")}>{n}</span>
            </Link>
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

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[7rem_1fr] gap-4 py-2.5">
      <dt className={cn(typeClass("caption"), "text-muted-foreground")}>{label}</dt>
      <dd className={typeClass("body")}>{children}</dd>
    </div>
  );
}
