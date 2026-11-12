import { useState } from "react";
import { Link } from "react-router";
import { RefreshCw } from "@keyline-icons/react";
import { api, errorMessage, useApi, type Book, type MediaIssue } from "@/api";
import { LoadError } from "@/components/shell/load-error";
import { Cover } from "@/components/media/cover";
import { EmptyState, Page, PageHeader } from "@/components/media/layout";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsItem, TabsPanel } from "@/components/ui/tabs";
import { toast } from "@yusufceng1z/ding-react";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { IconAction, TableSkeleton } from "./_parts";

const HEADS = ["表紙", "シリーズ", "パス", "詳細", "操作"];

type Row = { issue: MediaIssue; book: Book; series: string };

export default function Screen() {
  const { data, error, reload } = useApi(async () => {
    const issues = await api.mediaIssues();
    const books = await api.booksByIds(issues.map((i) => i.bookId));
    return { issues, books };
  }, []);

  const [tab, setTab] = useState(0);
  const [busyIds, setBusyIds] = useState<Set<string>>(new Set());

  const reanalyze = async (bookId: string) => {
    setBusyIds((s) => new Set(s).add(bookId));
    try {
      await api.analyzeBook(bookId);
      toast.success("再解析を開始しました");
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
    { label: "エラー", rows: rowsFor(["error", "unsupported"]) },
    { label: "見つからない", rows: rowsFor(["missing"]) },
  ];

  return (
    <Page>
      <PageHeader title="メディア管理" />

      {error && !data && <LoadError message={error} onRetry={reload} />}

      {!error && !data && <TableSkeleton heads={HEADS} rows={3} />}

      {data && (
        <>
          <Tabs selectedIndex={tab} onSelect={setTab} idPrefix="media" className="mb-4">
            {tabs.map((t, i) => (
              <TabsItem key={t.label} index={i} label={`${t.label}（${t.rows.length}）`} />
            ))}
          </Tabs>
          {tabs.map((t, i) => (
            <TabsPanel key={t.label} index={i} selectedIndex={tab} idPrefix="media">
              {t.rows.length === 0 ? (
                <EmptyState title="問題のあるファイルはありません" />
              ) : (
                <div className="overflow-x-auto">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        {HEADS.map((h) => (
                          <TableHead key={h}>{h}</TableHead>
                        ))}
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {t.rows.map(({ issue, book, series }, r) => (
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
                              {series} {book.number}巻
                            </Link>
                          </TableCell>
                          <TableCell className={cn(typeClass("caption"), "max-w-[280px] truncate font-mono")}>{book.path}</TableCell>
                          <TableCell className="min-w-[200px]">{issue.detail}</TableCell>
                          <TableCell>
                            <div className="flex gap-1">
                              <IconAction
                                label="再解析"
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
        </>
      )}
    </Page>
  );
}
