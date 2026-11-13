import { useState, type ChangeEvent, type ReactNode } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { ChevronLeft, ChevronRight, CircleCheck, MoreHorizontal, Play, RotateCcw } from "@keyline-icons/react";
import { toast } from "@yusufceng1z/ding-react";
import { api, bookFileUrl, bookInlineUrl, errorMessage, formatBytes, formatDate, startDownload, useApi, type Author, type MediaStatus } from "@/api";
import { useLiveReload } from "@/api/live";
import { bumpCover } from "@/lib/cover-retry";
import { LoadError } from "@/components/shell/load-error";
import { Cover } from "@/components/media/cover";
import { BrandAction } from "@/components/media/brand-action";
import { DotTrack } from "@/components/media/dot-track";
import { EmptyState, Page, SectionTitle } from "@/components/media/layout";
import { Block } from "@/components/media/skeletons";
import { AddToReadListDialog } from "@/routes/readlists/add-to-readlist";
import { BookEditDialog } from "@/routes/book-edit-dialog";
import { ConfirmDialog } from "@/routes/admin/_parts";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DropdownContent, DropdownMenu, DropdownTrigger } from "@/components/ui/dropdown";
import { MenuItem } from "@/components/ui/menu-item";
import { Tooltip } from "@/components/ui/tooltip";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";
import { defineMessages, useT } from "@/i18n";

/** A secondary action that is a 36 px icon square on a phone. */
const phoneIcon = "max-sm:shrink-0 max-sm:px-2.5";

const m = defineMessages({
  ja: {
    notFound: "巻が見つかりません",
    notFoundBody: "シリーズからもう一度探してください",
    series: "シリーズ",
    listSep: "、",
    author: "{name}（{role}）",
    roleWriter: "作",
    rolePenciller: "画",
    roleColorist: "色",
    roleTranslator: "翻訳",
    roleEditor: "編集",
    roleCover: "表紙",
    mediaReady: "正常",
    mediaFailed: "エラー",
    mediaUnsupported: "非対応",
    mediaOutdated: "要再スキャン",
    coverRegenerated: "表紙を作り直しました",
    coverPicked: "{n}ページ目を表紙にしました",
    incognito: "シークレットモードで読む",
    download: "ファイルをダウンロード",
    addToReadList: "リーディングリストに追加",
    regenerate: "表紙を作り直す",
    choosePage: "表紙のページを選ぶ",
    markedUnread: "未読にしました",
    markedRead: "既読にしました",
    prev: "前の巻",
    next: "次の巻",
    volume: "第{n}巻",
    pages: "{n}ページ",
    badgeRead: "既読",
    pageOf: "{page} / {total} ページ",
    progress: "読書の進捗",
    openInBrowser: "ブラウザで開く",
    continueReading: "続きから読む",
    startReading: "読む",
    readFromStart: "最初から読む",
    markUnread: "未読にする",
    markRead: "既読にする",
    more: "その他",
    editMetadata: "メタデータを編集",
    uploadThumbnail: "表紙の画像をアップロード",
    thumbTooLarge: "画像は 20 MB 以下にしてください",
    thumbUploaded: "表紙の画像を差し替えました",
    regenerateTitle: "表紙を作り直しますか",
    regenerateUploadedBody: "アップロードした画像を、ページから作った表紙に置き換えます。",
    analysisPending: "解析が終わっていません",
    releaseDate: "発売日",
    notSet: "未設定",
    authors: "著者",
    pagesTitle: "ページ",
    pickPrompt: "表紙にするページを選んでください",
    cancel: "やめる",
    pageCover: "{n}ページ目を表紙にする",
    coverMark: "・表紙",
    showAll: "すべてのページを表示（{n}）",
    filesTitle: "ファイル",
    path: "パス",
    format: "形式",
    size: "サイズ",
    mediaStatus: "メディア状態",
    cantOpen: "アーカイブを開けません",
    unsupportedFormat: "この形式は読めません",
  },
  en: {
    notFound: "Book not found",
    notFoundBody: "Find it again from the series",
    series: "Series",
    listSep: ", ",
    author: "{name} ({role})",
    roleWriter: "Writer",
    rolePenciller: "Penciller",
    roleColorist: "Colorist",
    roleTranslator: "Translator",
    roleEditor: "Editor",
    roleCover: "Cover",
    mediaReady: "OK",
    mediaFailed: "Error",
    mediaUnsupported: "Unsupported",
    mediaOutdated: "Needs rescan",
    coverRegenerated: "Thumbnail regenerated",
    coverPicked: "Page {n} set as the cover",
    incognito: "Read in incognito mode",
    download: "Download file",
    addToReadList: "Add to read list",
    regenerate: "Regenerate thumbnail",
    choosePage: "Choose cover page",
    markedUnread: "Marked as unread",
    markedRead: "Marked as read",
    prev: "Previous book",
    next: "Next book",
    volume: "Book {n}",
    pages: "{n} pages",
    badgeRead: "Read",
    pageOf: "{page} / {total} pages",
    progress: "Reading progress",
    openInBrowser: "Open in browser",
    continueReading: "Resume",
    startReading: "Read",
    readFromStart: "Read from start",
    markUnread: "Mark as unread",
    markRead: "Mark as read",
    more: "More",
    editMetadata: "Edit metadata",
    uploadThumbnail: "Upload thumbnail",
    thumbTooLarge: "Images must be 20 MB or smaller",
    thumbUploaded: "Thumbnail replaced",
    regenerateTitle: "Regenerate the thumbnail?",
    regenerateUploadedBody: "This replaces the uploaded image with a thumbnail made from a page.",
    analysisPending: "Analysis is not finished yet",
    releaseDate: "Release date",
    notSet: "Not set",
    authors: "Authors",
    pagesTitle: "Pages",
    pickPrompt: "Choose the page to use as the cover",
    cancel: "Cancel",
    pageCover: "Use page {n} as the cover",
    coverMark: "· Cover",
    showAll: "Show all pages ({n})",
    filesTitle: "Files",
    path: "Path",
    format: "Format",
    size: "Size",
    mediaStatus: "Media status",
    cantOpen: "Can't open the archive",
    unsupportedFormat: "This format can't be read",
  },
});

const PREVIEW_PAGES = 24;
// The API's limit for an uploaded thumbnail.
const MAX_THUMBNAIL_BYTES = 20 * 1024 * 1024;

export default function Screen() {
  const { bookId = "" } = useParams();
  // Keyed by id so the read-state override resets when the route changes.
  return <BookPage key={bookId} id={bookId} />;
}

function BookPage({ id }: { id: string }) {
  const t = useT(m);
  const navigate = useNavigate();
  const { data, error, reload } = useApi(async () => {
    const book = await api.book(id);
    if (!book) return null;
    const [series, adjacent, pages] = await Promise.all([api.seriesById(book.seriesId), api.adjacentBooks(id), api.pages(id).catch(() => [])]);
    return { book, series, adjacent, pages };
  }, [id]);
  // Analysis or a new cover of this book, and reading on another tab or device.
  const shown = data?.book;
  useLiveReload(reload, (e) => {
    if (e.type === "book") return e.data.bookId === id;
    if (e.type !== "progress" || !shown) return false;
    if (e.data.bookId) return e.data.bookId === id;
    if (e.data.seriesId) return e.data.seriesId === shown.seriesId;
    return e.data.libraryId === shown.libraryId;
  });

  const [allPages, setAllPages] = useState(false);
  const [busy, setBusy] = useState(false);
  const { data: me } = useApi(() => api.me(), []);
  const isAdmin = me?.role === "admin";
  const canDownload = isAdmin || !!me?.canDownload;
  const canStream = isAdmin || !!me?.canStream;
  const [picking, setPicking] = useState(false);
  // The read-list dialog is remounted per open so its lists are fetched fresh.
  const [addOpen, setAddOpen] = useState(false);
  const [addKey, setAddKey] = useState(0);
  // The metadata dialog is remounted per open so its form starts fresh.
  const [editOpen, setEditOpen] = useState(false);
  const [editKey, setEditKey] = useState(0);
  const [confirmRegen, setConfirmRegen] = useState(false);

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
        <EmptyState title={t("notFound")} body={t("notFoundBody")} />
      </Page>
    );
  }

  const { book, series, adjacent } = data;
  const roles: Record<Author["role"], string> = {
    writer: t("roleWriter"),
    penciller: t("rolePenciller"),
    colorist: t("roleColorist"),
    translator: t("roleTranslator"),
    editor: t("roleEditor"),
    cover: t("roleCover"),
  };
  const mediaLabel: Record<MediaStatus, string> = {
    ready: t("mediaReady"),
    error: t("mediaFailed"),
    unsupported: t("mediaUnsupported"),
    outdated: t("mediaOutdated"),
  };
  const completed = !!book.progress?.completed;
  const page = book.progress?.page ?? 0;
  const pct = book.pageCount ? Math.min(100, Math.round((page / book.pageCount) * 100)) : 0;
  const dots = Math.min(book.pageCount, 60);
  const shownPages = allPages ? data.pages.length : Math.min(PREVIEW_PAGES, data.pages.length);
  const authors = book.authors.map((a) => t("author", { name: a.name, role: roles[a.role] })).join(t("listSep"));
  // Marking read needs the page count and a readable archive.
  const canMarkRead = book.pageCount > 0 && book.mediaStatus === "ready";
  // A PDF whose pages are not single images cannot go to the reader: offer the
  // browser's own PDF viewer instead (streaming permission only).
  const pdfInBrowser = book.format === "pdf" && book.mediaStatus === "unsupported";

  const regenerate = async () => {
    try {
      await api.regenerateBookThumbnail(book.id);
      bumpCover(book.id, book.seriesId); // the <img> fetches it anew
      toast.success(t("coverRegenerated"));
    } catch (err) {
      toast.error(errorMessage(err));
    }
  };

  const chooseCover = async (n: number) => {
    setBusy(true);
    try {
      await api.setBookCover(book.id, n);
      setPicking(false);
      bumpCover(book.id, book.seriesId);
      toast.success(t("coverPicked", { n }));
      reload();
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  // その他 menu, numbered in the order shown.
  const menu: { label: string; onSelect: () => void }[] = [
    { label: t("incognito"), onSelect: () => navigate(`/read/${book.id}?incognito=1`) },
  ];
  if (canDownload) menu.push({ label: t("download"), onSelect: () => startDownload(bookFileUrl(book.id)) });
  // Same rule as editing read lists (server: UpdateReadList is admin only).
  if (isAdmin) {
    menu.push({
      label: t("addToReadList"),
      onSelect: () => {
        setAddKey((k) => k + 1);
        setAddOpen(true);
      },
    });
  }
  if (isAdmin) {
    menu.push({
      label: t("editMetadata"),
      onSelect: () => {
        setEditKey((k) => k + 1);
        setEditOpen(true);
      },
    });
  }
  if (isAdmin && book.mediaStatus === "ready") {
    // An uploaded thumbnail is replaced by regenerating, so ask first.
    menu.push({ label: t("regenerate"), onSelect: book.thumbnailUploaded ? () => setConfirmRegen(true) : regenerate });
    menu.push({
      label: t("choosePage"),
      onSelect: () => {
        setPicking(true);
        document.getElementById("pages")?.scrollIntoView({ behavior: "smooth", block: "start" });
      },
    });
  }
  if (isAdmin) {
    menu.push({ label: t("uploadThumbnail"), onSelect: () => document.getElementById("book-thumb-input")?.click() });
  }

  const uploadThumbnail = async (file: File) => {
    if (file.size > MAX_THUMBNAIL_BYTES) {
      toast.error(t("thumbTooLarge"));
      return;
    }
    setBusy(true);
    try {
      await api.uploadBookThumbnail(book.id, file);
      bumpCover(book.id, book.seriesId); // the <img> fetches the new image
      toast.success(t("thumbUploaded"));
      reload();
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const onThumbPicked = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = ""; // so picking the same file again still fires
    if (file) void uploadThumbnail(file);
  };

  const toggleRead = async () => {
    setBusy(true);
    try {
      if (completed) await api.clearProgress(book.id);
      else await api.setProgress(book.id, book.pageCount, true);
      toast.success(completed ? t("markedUnread") : t("markedRead"));
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
          className={cn(typeClass("caption"), "inline-flex min-w-0 items-center gap-1 text-muted-foreground hover:text-foreground pointer-coarse:-my-3.5 pointer-coarse:py-3.5")}
        >
          <ChevronLeft size={14} className="shrink-0" />
          <span className="truncate">{series?.title ?? t("series")}</span>
        </Link>
        <div className="flex shrink-0 items-center gap-1">
          <Tooltip content={t("prev")}>
            <Button
              variant="ghost"
              size="icon"
              className="pointer-coarse:h-11 pointer-coarse:w-11"
              aria-label={t("prev")}
              disabled={!adjacent.prev}
              onClick={() => adjacent.prev && navigate(`/books/${adjacent.prev.id}`)}
            >
              <ChevronLeft />
            </Button>
          </Tooltip>
          <Tooltip content={t("next")}>
            <Button
              variant="ghost"
              size="icon"
              className="pointer-coarse:h-11 pointer-coarse:w-11"
              aria-label={t("next")}
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
          <Cover src={book.thumbnailUrl} title={series?.title ?? book.title} label={String(book.number)} priority />
        </div>

        <div className="flex min-w-0 flex-1 flex-col gap-3">
          <h1 className={cn(typeClass("display"), "break-words")} style={{ fontVariationSettings: fontWeights.bold }}>
            {book.title}
          </h1>
          <p className={cn(typeClass("body"), "font-mono text-muted-foreground")}>
            {t("volume", { n: book.number })} · {t("pages", { n: book.pageCount })} · {book.format.toUpperCase()} · {formatBytes(book.sizeBytes)}
          </p>

          <div className="mt-1 space-y-1.5">
            {completed ? (
              <Badge variant="dot">{t("badgeRead")}</Badge>
            ) : book.progress ? (
              <>
                <p className={cn(typeClass("caption"), "font-mono text-muted-foreground tabular-nums")}>
                  {t("pageOf", { page, total: book.pageCount })}
                </p>
                <DotTrack total={dots} value={Math.round((pct / 100) * dots)} label={t("progress")} />
              </>
            ) : null}
          </div>

          {/* One row on a phone: there the secondary actions show their icon
              only (the label stays for screen readers) and the main action
              takes the rest of the row. */}
          <div className="mt-2 flex flex-wrap items-center gap-2 max-sm:flex-nowrap">
            {pdfInBrowser ? (
              canStream && (
                <Button variant="primary" asChild>
                  <a href={bookInlineUrl(book.id)} target="_blank" rel="noopener">
                    {t("openInBrowser")}
                  </a>
                </Button>
              )
            ) : (
              <>
                <BrandAction to={`/read/${book.id}`} icon={Play} className="max-sm:flex-1">
                  {book.progress && !completed ? t("continueReading") : t("startReading")}
                </BrandAction>
                <Button variant="secondary" asChild className={phoneIcon}>
                  <Link to={`/read/${book.id}?page=1`}>
                    <RotateCcw size={16} className="sm:hidden" />
                    <span className="max-sm:sr-only">{t("readFromStart")}</span>
                  </Link>
                </Button>
              </>
            )}
            <Button variant="secondary" disabled={busy || !canMarkRead} onClick={toggleRead} aria-pressed={completed} className={phoneIcon}>
              <CircleCheck size={16} className={cn("sm:hidden", completed && "text-brand")} />
              <span className="max-sm:sr-only">{completed ? t("markUnread") : t("markRead")}</span>
            </Button>
            <DropdownMenu>
              <DropdownTrigger
                render={
                  <Button variant="secondary" className={phoneIcon}>
                    <MoreHorizontal size={16} className="sm:hidden" />
                    <span className="max-sm:sr-only">{t("more")}</span>
                  </Button>
                }
              />
              <DropdownContent align="start" sideOffset={4} className="w-56">
                {menu.map((item, i) => (
                  <MenuItem key={item.label} index={i} label={item.label} onSelect={item.onSelect} />
                ))}
              </DropdownContent>
            </DropdownMenu>
          </div>
          {isAdmin && <AddToReadListDialog key={addKey} bookId={book.id} open={addOpen} onOpenChange={setAddOpen} />}
          {isAdmin && (
            <>
              <input
                id="book-thumb-input"
                type="file"
                accept="image/jpeg,image/png,image/webp"
                tabIndex={-1}
                aria-hidden
                hidden
                onChange={onThumbPicked}
              />
              <BookEditDialog key={editKey} book={book} open={editOpen} onOpenChange={setEditOpen} onSaved={reload} />
              <ConfirmDialog
                open={confirmRegen}
                onOpenChange={setConfirmRegen}
                title={t("regenerateTitle")}
                body={t("regenerateUploadedBody")}
                confirmLabel={t("regenerate")}
                onConfirm={regenerate}
              />
            </>
          )}
          {!canMarkRead && book.mediaStatus === "outdated" && (
            <p className={cn(typeClass("caption"), "text-muted-foreground")}>{t("analysisPending")}</p>
          )}

          <dl className="mt-4 max-w-2xl divide-y divide-border">
            <Fact label={t("releaseDate")}>{book.releaseDate ? formatDate(book.releaseDate) : t("notSet")}</Fact>
            <Fact label="ISBN">{book.isbn || t("notSet")}</Fact>
            <Fact label={t("authors")}>{authors || t("notSet")}</Fact>
          </dl>
        </div>
      </div>

      <section id="pages" className="mt-10 scroll-mt-20">
        <SectionTitle>{t("pagesTitle")}</SectionTitle>
        {picking && (
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <p className={cn(typeClass("body"), "text-muted-foreground")}>{t("pickPrompt")}</p>
            <Button variant="ghost" size="compact" className="pointer-coarse:h-11" onClick={() => setPicking(false)}>
              {t("cancel")}
            </Button>
          </div>
        )}
        <div className="grid grid-cols-[repeat(auto-fill,minmax(96px,1fr))] gap-3">
          {Array.from({ length: shownPages }, (_, i) => i + 1).map((n) => (
            <PageTile
              key={n}
              to={picking ? undefined : `/read/${book.id}?page=${n}`}
              onPick={picking && !busy ? () => chooseCover(n) : undefined}
              label={picking ? t("pageCover", { n }) : undefined}
            >
              <img
                src={data.pages[n - 1].thumbnailUrl}
                alt=""
                loading="lazy"
                decoding="async"
                draggable={false}
                className="aspect-[2/3] w-full rounded-[6px] bg-muted object-cover transition-transform duration-160 ease-[cubic-bezier(0.16,1,0.3,1)] group-hover:-translate-y-0.5 motion-reduce:transform-none"
                style={{ boxShadow: "var(--cover-shadow)" }}
              />
              <span className={cn(typeClass("micro"), "mt-1 block text-center text-muted-foreground tabular-nums")}>
                {n}
                {n === book.coverPage && t("coverMark")}
              </span>
            </PageTile>
          ))}
        </div>
        {!allPages && book.pageCount > PREVIEW_PAGES && (
          <Button variant="secondary" className="mt-4" onClick={() => setAllPages(true)}>
            {t("showAll", { n: book.pageCount })}
          </Button>
        )}
      </section>

      <section className="mt-10">
        <SectionTitle>{t("filesTitle")}</SectionTitle>
        <dl className="max-w-2xl divide-y divide-border">
          <Fact label={t("path")}>
            <span className="break-all font-mono">{book.path}</span>
          </Fact>
          <Fact label={t("format")}>{book.format.toUpperCase()}</Fact>
          <Fact label={t("size")}>{formatBytes(book.sizeBytes)}</Fact>
          <Fact label={t("mediaStatus")}>
            <div className="flex flex-col gap-1">
              <div className="flex flex-wrap items-center gap-2">
                {book.mediaStatus === "error" ? (
                  <Badge variant="solid" color="red">
                    {mediaLabel[book.mediaStatus]}
                  </Badge>
                ) : (
                  <Badge variant="dot">{mediaLabel[book.mediaStatus]}</Badge>
                )}
                {book.mediaStatus === "error" && <span className="text-muted-foreground">{t("cantOpen")}</span>}
                {book.mediaStatus === "unsupported" && <span className="text-muted-foreground">{t("unsupportedFormat")}</span>}
              </div>
              {(book.mediaStatus === "error" || book.mediaStatus === "unsupported") && book.mediaError && (
                <p className={cn(typeClass("caption"), "text-muted-foreground break-words")}>{book.mediaError}</p>
              )}
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
