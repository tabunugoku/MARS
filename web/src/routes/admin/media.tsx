import { useState } from "react";
import { Link } from "react-router";
import { Bin, RefreshCw } from "@keyline-icons/react";
import { api, errorMessage, useApi, type Book, type MediaIssue } from "@/api";
import { LoadError } from "@/components/shell/load-error";
import { Cover } from "@/components/media/cover";
import { EmptyState, Page, PageHeader } from "@/components/media/layout";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsItem, TabsPanel } from "@/components/ui/tabs";
import { toast } from "@yusufceng1z/ding-react";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { defineMessages, useT } from "@/i18n";
import { ConfirmDialog, IconAction, TableSkeleton, relTime } from "./_parts";

const m = defineMessages({
  ja: {
    title: "メディア管理",
    colCover: "表紙",
    colSeries: "シリーズ",
    colName: "名前",
    colPath: "パス",
    colDetails: "詳細",
    colMissingSince: "消えた日",
    colActions: "操作",
    reanalyzeStarted: "再解析を開始しました",
    tabErrors: "エラー",
    tabMissing: "見つからない",
    tabTrash: "ゴミ箱",
    tabCount: "{label}（{n}）",
    noProblems: "問題のあるファイルはありません",
    bookTitle: "{series} {number}巻",
    bookName: "{series} / {title}",
    seriesBooks: "{n} 冊",
    reanalyze: "再解析",
    trashNote: "ファイルが見つからなくなったシリーズと巻です。ファイルが同じ場所に戻ると、次のスキャンで自動的に元に戻ります。",
    trashEmpty: "ゴミ箱は空です",
    emptyTrash: "ゴミ箱を空にする",
    emptyTitle: "ゴミ箱を空にしますか",
    emptyBody: "ゴミ箱のシリーズと巻を一覧から消します。ファイルには触れません。元に戻せません。",
    emptyConfirm: "空にする",
    emptied: ({ series, books }) => `ゴミ箱を空にしました（シリーズ ${series} 件、本 ${books} 件）`,
    removeLabel: "「{name}」を消す",
    removeTitle: "「{name}」を消しますか",
    removeBody: "一覧から消します。ファイルには触れません。",
    removeConfirm: "消す",
    removed: "「{name}」を一覧から消しました",
  },
  en: {
    title: "Media management",
    colCover: "Cover",
    colSeries: "Series",
    colName: "Name",
    colPath: "Path",
    colDetails: "Details",
    colMissingSince: "Missing since",
    colActions: "Actions",
    reanalyzeStarted: "Re-analysis started",
    tabErrors: "Errors",
    tabMissing: "Missing",
    tabTrash: "Trash",
    tabCount: "{label} ({n})",
    noProblems: "No problem files",
    bookTitle: "{series} vol. {number}",
    bookName: "{series} / {title}",
    seriesBooks: ({ n }) => `${n} ${n === 1 ? "book" : "books"}`,
    reanalyze: "Re-analyze",
    trashNote:
      "Series and books whose files are gone. If a file comes back to the same place, the next scan restores it.",
    trashEmpty: "The trash is empty",
    emptyTrash: "Empty trash",
    emptyTitle: "Empty trash?",
    emptyBody: "Removes the series and books in the trash from the library. Files are not touched. This cannot be undone.",
    emptyConfirm: "Empty",
    emptied: ({ series, books }) =>
      `Emptied the trash: ${series} series, ${books} ${books === 1 ? "book" : "books"}`,
    removeLabel: "Remove “{name}”",
    removeTitle: "Remove “{name}”?",
    removeBody: "Removes it from the library. Files are not touched.",
    removeConfirm: "Remove",
    removed: "Removed “{name}” from the library",
  },
});

type Row = { issue: MediaIssue; book: Book; series: string };

/** One line of the trash: a trashed series, or a trashed book. */
type TrashItem = {
  kind: "series" | "book";
  id: string;
  name: string;
  /** Series only: how many books go with it. */
  bookCount?: number;
  path: string;
  deletedAt: string;
};

export default function Screen() {
  const t = useT(m);
  const heads = [t("colCover"), t("colSeries"), t("colPath"), t("colDetails"), t("colActions")];
  const trashHeads = [t("colName"), t("colPath"), t("colMissingSince"), t("colActions")];
  const { data, error, reload } = useApi(async () => {
    const issues = await api.mediaIssues();
    const books = await api.booksByIds(issues.map((i) => i.bookId));
    const trash = await api.trash();
    return { issues, books, trash };
  }, []);

  const [tab, setTab] = useState(0);
  const [busyIds, setBusyIds] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);
  // "all" empties the trash; an item removes just that series or book.
  const [dialog, setDialog] = useState<{ open: boolean; target: TrashItem | "all" }>({ open: false, target: "all" });

  const reanalyze = async (bookId: string) => {
    setBusyIds((s) => new Set(s).add(bookId));
    try {
      await api.analyzeBook(bookId);
      toast.success(t("reanalyzeStarted"));
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusyIds((s) => {
        const next = new Set(s);
        next.delete(bookId);
        return next;
      });
    }
  };

  const removeTarget = async (target: TrashItem | "all") => {
    setBusy(true);
    try {
      if (target === "all") {
        const r = await api.emptyTrash();
        toast.success(t("emptied", { series: r.series, books: r.books }));
      } else {
        if (target.kind === "series") await api.deleteTrashedSeries(target.id);
        else await api.deleteTrashedBook(target.id);
        toast.success(t("removed", { name: target.name }));
      }
      reload();
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  // Rows per tab: エラー also gathers unsupported files, since the spec has no tab for them.
  const rowsFor = (kinds: MediaIssue["kind"][]): Row[] => {
    if (!data) return [];
    const out: Row[] = [];
    for (const issue of data.issues) {
      const book = data.books.find((b) => b.id === issue.bookId);
      if (!book || !kinds.includes(issue.kind)) continue;
      out.push({ issue, book, series: book.seriesTitle || book.title });
    }
    return out;
  };
  const tabs = [
    { label: t("tabErrors"), rows: rowsFor(["error", "unsupported"]) },
    { label: t("tabMissing"), rows: rowsFor(["missing"]) },
  ];
  const trashItems: TrashItem[] = data
    ? [
        ...data.trash.series.map(
          (s): TrashItem => ({ kind: "series", id: s.id, name: s.title, bookCount: s.bookCount, path: s.path, deletedAt: s.deletedAt }),
        ),
        ...data.trash.books.map(
          (b): TrashItem => ({
            kind: "book",
            id: b.id,
            name: t("bookName", { series: b.seriesTitle, title: b.title }),
            path: b.path,
            deletedAt: b.deletedAt,
          }),
        ),
      ]
    : [];
  const trashCount = trashItems.length;

  return (
    <Page>
      <PageHeader title={t("title")} />

      {error && !data && <LoadError message={error} onRetry={reload} />}

      {!error && !data && <TableSkeleton heads={heads} rows={3} />}

      {data && (
        <>
          <Tabs selectedIndex={tab} onSelect={setTab} idPrefix="media" className="mb-4">
            {tabs.map((entry, i) => (
              <TabsItem key={entry.label} index={i} label={t("tabCount", { label: entry.label, n: entry.rows.length })} />
            ))}
            <TabsItem index={2} label={t("tabCount", { label: t("tabTrash"), n: trashCount })} />
          </Tabs>
          {tabs.map((entry, i) => (
            <TabsPanel key={entry.label} index={i} selectedIndex={tab} idPrefix="media">
              {entry.rows.length === 0 ? (
                <EmptyState title={t("noProblems")} />
              ) : (
                <div className="overflow-x-auto overflow-y-hidden">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        {heads.map((h) => (
                          <TableHead key={h}>{h}</TableHead>
                        ))}
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {entry.rows.map(({ issue, book, series }, r) => (
                        <TableRow key={`${issue.kind}-${book.id}`} index={r}>
                          <TableCell>
                            <div className="w-8">
                              <Cover src={book.thumbnailUrl} title={series} label={String(book.number)} />
                            </div>
                          </TableCell>
                          <TableCell>
                            <Link
                              to={`/books/${book.id}`}
                              className="whitespace-nowrap text-foreground underline-offset-2 hover:underline"
                            >
                              {t("bookTitle", { series, number: book.number })}
                            </Link>
                          </TableCell>
                          <TableCell className={cn(typeClass("caption"), "max-w-[280px] truncate font-mono")}>{book.path}</TableCell>
                          <TableCell className="min-w-[200px]">{issue.detail}</TableCell>
                          <TableCell>
                            <div className="flex gap-1">
                              <IconAction
                                label={t("reanalyze")}
                                icon={RefreshCw}
                                onClick={() => !busyIds.has(book.id) && reanalyze(book.id)}
                              />
                            </div>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
            </TabsPanel>
          ))}
          <TabsPanel index={2} selectedIndex={tab} idPrefix="media">
            <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
              <p className={cn(typeClass("body"), "max-w-[640px] text-muted-foreground")}>{t("trashNote")}</p>
              <Button
                variant="secondary"
                disabled={busy || trashCount === 0}
                onClick={() => setDialog({ open: true, target: "all" })}
              >
                {t("emptyTrash")}
              </Button>
            </div>
            {trashCount === 0 ? (
              <EmptyState title={t("trashEmpty")} />
            ) : (
              <div className="overflow-x-auto overflow-y-hidden">
                <Table>
                  <TableHeader>
                    <TableRow>
                      {trashHeads.map((h) => (
                        <TableHead key={h}>{h}</TableHead>
                      ))}
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {trashItems.map((item, r) => (
                      <TableRow key={`${item.kind}-${item.id}`} index={r}>
                        <TableCell>
                          <span className="text-foreground">{item.name}</span>
                          {item.bookCount !== undefined && (
                            <span className={cn(typeClass("caption"), "ml-2 text-muted-foreground")}>
                              {t("seriesBooks", { n: item.bookCount })}
                            </span>
                          )}
                        </TableCell>
                        <TableCell className={cn(typeClass("caption"), "max-w-[280px] truncate font-mono")}>{item.path}</TableCell>
                        <TableCell className="whitespace-nowrap">{relTime(item.deletedAt)}</TableCell>
                        <TableCell>
                          <div className="flex gap-1">
                            <IconAction
                              label={t("removeLabel", { name: item.name })}
                              icon={Bin}
                              onClick={() => setDialog({ open: true, target: item })}
                            />
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </TabsPanel>
        </>
      )}

      <ConfirmDialog
        open={dialog.open}
        onOpenChange={(open) => setDialog((d) => ({ ...d, open }))}
        title={dialog.target === "all" ? t("emptyTitle") : t("removeTitle", { name: dialog.target.name })}
        body={dialog.target === "all" ? t("emptyBody") : t("removeBody")}
        confirmLabel={dialog.target === "all" ? t("emptyConfirm") : t("removeConfirm")}
        onConfirm={() => void removeTarget(dialog.target)}
      />
    </Page>
  );
}
