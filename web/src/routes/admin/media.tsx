import { useState } from "react";
import { Link } from "react-router";
import { RefreshCw, Bin } from "@keyline-icons/react";
import { api, useApi, type Book, type MediaIssue } from "@/api";
import { Cover } from "@/components/media/cover";
import { EmptyState, Page, PageHeader } from "@/components/media/layout";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsItem, TabsPanel } from "@/components/ui/tabs";
import { toast } from "@yusufceng1z/ding-react";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { ConfirmDialog, IconAction, TableSkeleton } from "./_parts";

const HEADS = ["表紙", "シリーズ", "パス", "詳細", "操作"];

type Row = { issue: MediaIssue; book: Book; series: string };

export default function Screen() {
  const { data } = useApi(async () => {
    const [issues, series] = await Promise.all([api.mediaIssues(), api.series()]);
    const books = await api.booksByIds(issues.map((i) => i.bookId));
    return { issues, books, titles: new Map(series.map((s) => [s.id, s.title])) };
  }, []);

  const [tab, setTab] = useState(0);
  const [trashed, setTrashed] = useState<Set<string>>(new Set());
  const [confirm, setConfirm] = useState<Book | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);

  // Rows per tab: エラー also gathers unsupported files, since the spec has no tab for them.
  const rowsFor = (kinds: MediaIssue["kind"][]): Row[] => {
    if (!data) return [];
    const out: Row[] = [];
    for (const issue of data.issues) {
      const book = data.books.find((b) => b.id === issue.bookId);
      if (!book || !kinds.includes(issue.kind) || trashed.has(book.id)) continue;
      out.push({ issue, book, series: data.titles.get(book.seriesId) ?? book.title });
    }
    return out;
  };
  const tabs = [
    { label: "エラー", rows: rowsFor(["error", "unsupported"]) },
    { label: "見つからない", rows: rowsFor(["missing"]) },
    { label: "重複", rows: rowsFor(["duplicate"]) },
  ];

  return (
    <Page>
      <PageHeader title="メディア管理" />

      {!data && <TableSkeleton heads={HEADS} rows={3} />}

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
                              <IconAction label="再解析" icon={RefreshCw} onClick={() => toast.success("再解析を開始しました")} />
                              <IconAction label="ゴミ箱へ" icon={Bin} onClick={() => { setConfirm(book); setConfirmOpen(true); }} />
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

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="ゴミ箱へ移動しますか"
        body={`「${confirm?.title ?? ""}」のファイルをゴミ箱に移動します。`}
        confirmLabel="ゴミ箱へ"
        onConfirm={() => {
          if (confirm) setTrashed(new Set([...trashed, confirm.id]));
          toast.success("ゴミ箱へ移動しました");
        }}
      />
    </Page>
  );
}
