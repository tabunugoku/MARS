import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { api, errorMessage, useApi, type Task, type TaskKind } from "@/api";
import { useLiveConnected, useLiveEvents, useLiveReload } from "@/api/live";
import { LoadError } from "@/components/shell/load-error";
import { EmptyState, Page, PageHeader, SectionTitle } from "@/components/media/layout";
import { CountUp } from "@/components/media/count-up";
import { DotTrack } from "@/components/media/dot-track";
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
const POLL_MS = 3000;

export default function Screen() {
  const { data, error, reload } = useApi(() => Promise.all([api.tasks(), api.libraries()]), []);
  const [confirm, setConfirm] = useState<"abort" | "trash" | null>(null);
  const [busyIds, setBusyIds] = useState<Set<string>>(new Set());
  const [aborting, setAborting] = useState(false);
  const [emptying, setEmptying] = useState(false);

  // Live task rows by id, laid over the last loaded list until the next load.
  const [patch, setPatch] = useState<{ base: unknown; rows: Map<string, Task> }>({ base: undefined, rows: new Map() });
  const loaded = data?.[0];
  const rows = patch.base === loaded ? patch.rows : undefined;
  const tasks = loaded?.map((t) => rows?.get(t.id) ?? t);
  const libNames = new Map((data?.[1] ?? []).map((l) => [l.id, l.name]));
  const active = tasks?.filter((t) => t.state === "running" || t.state === "queued") ?? [];
  const history = tasks?.filter((t) => t.state === "done" || t.state === "failed" || t.state === "cancelled") ?? [];
  const hasActive = active.length > 0;

  // Progress arrives over the event stream and is patched in place. A task
  // the list does not have yet, or one that just ended (it moves to the
  // history), loads the list again.
  const known = useRef(new Set<string>());
  known.current = new Set(loaded?.map((t) => t.id));
  useLiveEvents((e) => {
    if (e.type !== "task" || !known.current.has(e.data.id)) return;
    setPatch((p) => ({ base: loaded, rows: new Map(p.base === loaded ? p.rows : undefined).set(e.data.id, e.data) }));
  });
  useLiveReload(
    reload,
    (e) => e.type === "task" && (!known.current.has(e.data.id) || !(e.data.state === "running" || e.data.state === "queued")),
    { quiet: 300, maxWait: 2000 },
  );

  // Without the stream, poll while anything runs or waits. The ref keeps the
  // interval from restarting on every render, since reload is a new function
  // each time.
  const live = useLiveConnected();
  const reloadRef = useRef(reload);
  useEffect(() => {
    reloadRef.current = reload;
  });
  useEffect(() => {
    if (!hasActive || live) return;
    const id = setInterval(() => reloadRef.current(), POLL_MS);
    return () => clearInterval(id);
  }, [hasActive, live]);

  const cancel = async (id: string) => {
    setBusyIds((s) => new Set(s).add(id));
    try {
      await api.cancelTask(id);
      toast.success("タスクを中止しました");
      reload();
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setBusyIds((s) => {
        const next = new Set(s);
        next.delete(id);
        return next;
      });
    }
  };

  const cancelAll = async () => {
    const targets = active.map((t) => t.id);
    setAborting(true);
    try {
      const results = await Promise.allSettled(targets.map((id) => api.cancelTask(id)));
      const failed = results.filter((r): r is PromiseRejectedResult => r.status === "rejected");
      if (failed.length === 0) {
        toast.success("すべてのタスクを中止しました");
      } else {
        toast.error(`${failed.length} 件を中止できませんでした：${errorMessage(failed[0].reason)}`);
      }
      reload();
    } finally {
      setAborting(false);
    }
  };

  const emptyTrash = async () => {
    setEmptying(true);
    try {
      const r = await api.emptyTrash();
      toast.success(`ゴミ箱を空にしました（シリーズ ${r.series} 件、本 ${r.books} 件）`);
      reload();
    } catch (err) {
      toast.error(errorMessage(err));
    } finally {
      setEmptying(false);
    }
  };

  return (
    <Page>
      <PageHeader
        title="タスク"
        actions={
          tasks && (
            <>
              <Button variant="tertiary" disabled={!hasActive || aborting} onClick={() => setConfirm("abort")}>
                すべて中止
              </Button>
              <Button variant="secondary" disabled={emptying} onClick={() => setConfirm("trash")}>
                ゴミ箱を空にする
              </Button>
            </>
          )
        }
      />

      {error && !tasks && <LoadError message={error} onRetry={reload} />}

      {!error && (
        <>
          <section className="mb-10">
            <SectionTitle>実行中・待機中</SectionTitle>
            {!tasks && (
              <div className="flex flex-col gap-2" aria-hidden>
                {[0, 1].map((i) => (
                  <Block key={i} className="h-[88px] w-full rounded-xl" />
                ))}
              </div>
            )}
            {tasks && !hasActive && (
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
            {hasActive && (
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
                          <DotTrack
                            total={24}
                            value={running ? Math.round((pct / 100) * 24) : 0}
                            label={t.label}
                            size="sm"
                            className="flex-1"
                          />
                          <span className={cn(typeClass("caption"), "w-9 text-right font-mono text-muted-foreground tabular-nums")}>
                            <CountUp value={pct} />%
                          </span>
                        </div>
                      </div>
                      <Button variant="ghost" disabled={busyIds.has(t.id)} onClick={() => cancel(t.id)}>
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
                        <TableCell className="text-foreground">
                          <span>{t.label}</span>
                          {t.state === "failed" && t.error && (
                            <p className={cn(typeClass("caption"), "text-muted-foreground")}>{t.error}</p>
                          )}
                        </TableCell>
                        <TableCell>
                          {t.state === "done" && (
                            <Badge variant="ghost" color="gray">
                              完了
                            </Badge>
                          )}
                          {t.state === "failed" && (
                            <Badge variant="ghost" color="red">
                              失敗
                            </Badge>
                          )}
                          {t.state === "cancelled" && (
                            <Badge variant="ghost" color="gray">
                              中止
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
        </>
      )}

      <ConfirmDialog
        open={confirm === "abort"}
        onOpenChange={(open) => !open && setConfirm(null)}
        title="すべてのタスクを中止しますか"
        body="実行中と待機中のタスクをすべて止めます。"
        confirmLabel="すべて中止"
        onConfirm={() => void cancelAll()}
      />
      <ConfirmDialog
        open={confirm === "trash"}
        onOpenChange={(open) => !open && setConfirm(null)}
        title="ゴミ箱を空にしますか"
        body="ゴミ箱のファイルを完全に削除します。元に戻せません。"
        confirmLabel="空にする"
        onConfirm={() => void emptyTrash()}
      />
    </Page>
  );
}
