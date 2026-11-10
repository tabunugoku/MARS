import { useState } from "react";
import { Link } from "react-router";
import { api, useApi, type Task, type TaskKind } from "@/api";
import { EmptyState, Page, PageHeader, SectionTitle } from "@/components/media/layout";
import { Block } from "@/components/media/skeletons";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableEmpty, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { toast } from "@yusufceng1z/ding-react";
import { fontWeights } from "@/lib/font-weight";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { ConfirmDialog, TableSkeleton, relTime } from "./_parts";

const KIND_LABEL: Record<TaskKind, string> = {
  scan: "スキャン",
  analyze: "解析",
  thumbnail: "サムネイル",
  hash: "ハッシュ",
  trash: "ゴミ箱",
};
const HISTORY_HEADS = ["種別", "ラベル", "状態", "開始時刻"];

export default function Screen() {
  const { data } = useApi(() => Promise.all([api.tasks(), api.libraries()]), []);
  // Mock layer has no mutations: task state changes live here until the API exists.
  const [edited, setEdited] = useState<Task[] | null>(null);
  const [confirm, setConfirm] = useState<"abort" | "trash" | null>(null);

  const tasks = edited ?? data?.[0];
  const libNames = new Map((data?.[1] ?? []).map((l) => [l.id, l.name]));
  const active = tasks?.filter((t) => t.state === "running" || t.state === "queued") ?? [];
  const history = tasks?.filter((t) => t.state === "done" || t.state === "failed") ?? [];

  const abort = (id: string) => {
    if (!tasks) return;
    setEdited(tasks.map((t): Task => (t.id === id ? { ...t, state: "failed" } : t)));
    toast.success("タスクを中止しました");
  };

  const abortAll = () => {
    if (!tasks) return;
    setEdited(tasks.map((t): Task => (t.state === "running" || t.state === "queued" ? { ...t, state: "failed" } : t)));
    toast.success("すべてのタスクを中止しました");
  };

  return (
    <Page>
      <PageHeader
        title="タスク"
        actions={
          tasks && (
            <>
              <Button variant="tertiary" disabled={active.length === 0} onClick={() => setConfirm("abort")}>
                すべて中止
              </Button>
              <Button variant="secondary" onClick={() => setConfirm("trash")}>
                ゴミ箱を空にする
              </Button>
            </>
          )
        }
      />

      <section className="mb-10">
        <SectionTitle>実行中・待機中</SectionTitle>
        {!tasks && (
          <div className="flex flex-col gap-2" aria-hidden>
            {[0, 1].map((i) => (
              <Block key={i} className="h-[88px] w-full rounded-xl" />
            ))}
          </div>
        )}
        {tasks && active.length === 0 && (
          <EmptyState
            title="実行中のタスクはありません"
            body="スキャンや解析は、ライブラリ設定から始められます"
            action={
              <Button variant="secondary" asChild>
                <Link to="/admin/libraries">ライブラリ設定へ</Link>
              </Button>
            }
          />
        )}
        {active.length > 0 && (
          <ul className="flex flex-col gap-2">
            {active.map((t) => {
              const pct = Math.round(t.progress * 100);
              const running = t.state === "running";
              return (
                <li key={t.id} className="flex items-center gap-4 rounded-xl bg-card p-4">
                  <div className="min-w-0 flex-1 space-y-1.5">
                    <p className={cn(typeClass("subtitle"), "truncate text-foreground")} style={{ fontVariationSettings: fontWeights.semibold }}>
                      {t.label}
                    </p>
                    <p className={cn(typeClass("caption"), "text-muted-foreground")}>
                      {t.libraryId ? libNames.get(t.libraryId) : "全ライブラリ"}
                    </p>
                    <div className="flex items-center gap-3">
                      <div
                        role="progressbar"
                        aria-label={t.label}
                        aria-valuemin={0}
                        aria-valuemax={100}
                        aria-valuenow={pct}
                        className="h-1 flex-1 overflow-hidden rounded-full bg-muted"
                      >
                        {running && <div className="h-full rounded-full bg-brand" style={{ width: `${pct}%` }} />}
                      </div>
                      <span className={cn(typeClass("caption"), "w-9 text-right text-muted-foreground tabular-nums")}>{pct}%</span>
                    </div>
                  </div>
                  <Button variant="ghost" onClick={() => abort(t.id)}>
                    中止
                  </Button>
                </li>
              );
            })}
          </ul>
        )}
      </section>

      <section>
        <SectionTitle>履歴</SectionTitle>
        {!tasks && <TableSkeleton heads={HISTORY_HEADS} rows={3} />}
        {tasks && (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  {HISTORY_HEADS.map((h) => (
                    <TableHead key={h}>{h}</TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {history.map((t, i) => (
                  <TableRow key={t.id} index={i}>
                    <TableCell className="whitespace-nowrap">{KIND_LABEL[t.kind]}</TableCell>
                    <TableCell className="text-foreground">{t.label}</TableCell>
                    <TableCell>
                      {t.state === "done" ? (
                        <Badge variant="ghost" color="gray">
                          完了
                        </Badge>
                      ) : (
                        <Badge variant="ghost" color="red">
                          失敗
                        </Badge>
                      )}
                    </TableCell>
                    <TableCell className="whitespace-nowrap">{relTime(t.startedAt)}</TableCell>
                  </TableRow>
                ))}
                {history.length === 0 && (
                  <TableEmpty columns={HISTORY_HEADS.length} title="履歴はまだありません" rows={2} />
                )}
              </TableBody>
            </Table>
          </div>
        )}
      </section>

      <ConfirmDialog
        open={confirm === "abort"}
        onOpenChange={(open) => !open && setConfirm(null)}
        title="すべてのタスクを中止しますか"
        body="実行中と待機中のタスクをすべて止めます。"
        confirmLabel="すべて中止"
        onConfirm={abortAll}
      />
      <ConfirmDialog
        open={confirm === "trash"}
        onOpenChange={(open) => !open && setConfirm(null)}
        title="ゴミ箱を空にしますか"
        body="ゴミ箱のファイルを完全に削除します。元に戻せません。"
        confirmLabel="空にする"
        onConfirm={() => toast.success("ゴミ箱を空にしました")}
      />
    </Page>
  );
}
