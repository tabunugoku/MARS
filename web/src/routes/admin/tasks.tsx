import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { api, errorMessage, useApi, type Task, type TaskKind } from "@/api";
import { useLiveConnected, useLiveEvents, useLiveReload } from "@/api/live";
import { defineMessages, useT } from "@/i18n";
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

const m = defineMessages({
  ja: {
    title: "タスク",
    activeTitle: "実行中・待機中",
    historyTitle: "履歴",
    kindScan: "スキャン",
    kindAnalyze: "解析",
    kindThumbnail: "サムネイル",
    kindHash: "ハッシュ",
    kindTrash: "ゴミ箱",
    colKind: "種別",
    colLabel: "ラベル",
    colState: "状態",
    colStarted: "開始時刻",
    emptyActiveTitle: "実行中のタスクはありません",
    emptyActiveBody: "スキャンや解析は、ライブラリ設定から始められます",
    goLibraries: "ライブラリ設定へ",
    historyEmpty: "履歴はまだありません",
    allLibraries: "全ライブラリ",
    stop: "中止",
    stopAll: "すべて中止",
    stopped: "中止",
    done: "完了",
    failed: "失敗",
    taskStopped: "タスクを中止しました",
    allStopped: "すべてのタスクを中止しました",
    stopFailed: ({ n, error }) => `${n} 件を中止できませんでした：${error}`,
    emptyTrash: "ゴミ箱を空にする",
    emptied: ({ series, books }) => `ゴミ箱を空にしました（シリーズ ${series} 件、本 ${books} 件）`,
    stopAllTitle: "すべてのタスクを中止しますか",
    stopAllBody: "実行中と待機中のタスクをすべて止めます。",
    emptyTrashTitle: "ゴミ箱を空にしますか",
    emptyTrashBody: "ゴミ箱のファイルを完全に削除します。元に戻せません。",
    empty: "空にする",
  },
  en: {
    title: "Tasks",
    activeTitle: "Running and queued",
    historyTitle: "History",
    kindScan: "Scan",
    kindAnalyze: "Analyze",
    kindThumbnail: "Thumbnail",
    kindHash: "Hash",
    kindTrash: "Trash",
    colKind: "Kind",
    colLabel: "Label",
    colState: "Status",
    colStarted: "Started",
    emptyActiveTitle: "No tasks running",
    emptyActiveBody: "Start a scan or analysis from library settings",
    goLibraries: "Go to library settings",
    historyEmpty: "No history yet",
    allLibraries: "All libraries",
    stop: "Stop",
    stopAll: "Stop all",
    stopped: "Stopped",
    done: "Done",
    failed: "Failed",
    taskStopped: "Task stopped",
    allStopped: "All tasks stopped",
    stopFailed: ({ n, error }) => `Could not stop ${n} ${n === 1 ? "task" : "tasks"}: ${error}`,
    emptyTrash: "Empty trash",
    emptied: ({ series, books }) =>
      `Emptied the trash: ${series} series, ${books} ${books === 1 ? "book" : "books"}`,
    stopAllTitle: "Stop all tasks?",
    stopAllBody: "Stops all running and queued tasks.",
    emptyTrashTitle: "Empty trash?",
    emptyTrashBody: "Permanently deletes the files in the trash. This cannot be undone.",
    empty: "Empty",
  },
});

const POLL_MS = 3000;

export default function Screen() {
  const t = useT(m);
  const kindLabel: Record<TaskKind, string> = {
    scan: t("kindScan"),
    analyze: t("kindAnalyze"),
    thumbnail: t("kindThumbnail"),
    hash: t("kindHash"),
    trash: t("kindTrash"),
  };
  const historyHeads = [t("colKind"), t("colLabel"), t("colState"), t("colStarted")];
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
    // A task may have ended between loading the list and the stream opening;
    // polling stops once it is open, so load the list again.
    if (e.type === "open") reload();
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
      toast.success(t("taskStopped"));
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
        toast.success(t("allStopped"));
      } else {
        toast.error(t("stopFailed", { n: failed.length, error: errorMessage(failed[0].reason) }));
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
      toast.success(t("emptied", { series: r.series, books: r.books }));
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
        title={t("title")}
        actions={
          tasks && (
            <>
              <Button variant="tertiary" disabled={!hasActive || aborting} onClick={() => setConfirm("abort")}>
                {t("stopAll")}
              </Button>
              <Button variant="secondary" disabled={emptying} onClick={() => setConfirm("trash")}>
                {t("emptyTrash")}
              </Button>
            </>
          )
        }
      />

      {error && !tasks && <LoadError message={error} onRetry={reload} />}

      {!error && (
        <>
          <section className="mb-10">
            <SectionTitle>{t("activeTitle")}</SectionTitle>
            {!tasks && (
              <div className="flex flex-col gap-2" aria-hidden>
                {[0, 1].map((i) => (
                  <Block key={i} className="h-[88px] w-full rounded-xl" />
                ))}
              </div>
            )}
            {tasks && !hasActive && (
              <EmptyState
                title={t("emptyActiveTitle")}
                body={t("emptyActiveBody")}
                action={
                  <Button variant="secondary" asChild>
                    <Link to="/admin/libraries">{t("goLibraries")}</Link>
                  </Button>
                }
              />
            )}
            {hasActive && (
              <ul className="flex flex-col gap-2">
                {active.map((task) => {
                  const pct = Math.round(task.progress * 100);
                  const running = task.state === "running";
                  return (
                    <li key={task.id} className="flex items-center gap-4 rounded-xl bg-card p-4">
                      <div className="min-w-0 flex-1 space-y-1.5">
                        <p className={cn(typeClass("subtitle"), "truncate text-foreground")} style={{ fontVariationSettings: fontWeights.semibold }}>
                          {task.label}
                        </p>
                        <p className={cn(typeClass("caption"), "text-muted-foreground")}>
                          {task.libraryId ? libNames.get(task.libraryId) : t("allLibraries")}
                        </p>
                        <div className="flex items-center gap-3">
                          <DotTrack
                            total={24}
                            value={running ? Math.round((pct / 100) * 24) : 0}
                            label={task.label}
                            size="sm"
                            className="flex-1"
                          />
                          <span className={cn(typeClass("caption"), "w-9 text-right font-mono text-muted-foreground tabular-nums")}>
                            <CountUp value={pct} />%
                          </span>
                        </div>
                      </div>
                      <Button variant="ghost" disabled={busyIds.has(task.id)} onClick={() => cancel(task.id)}>
                        {t("stop")}
                      </Button>
                    </li>
                  );
                })}
              </ul>
            )}
          </section>

          <section>
            <SectionTitle>{t("historyTitle")}</SectionTitle>
            {!tasks && <TableSkeleton heads={historyHeads} rows={3} />}
            {tasks && (
              <div className="overflow-x-auto overflow-y-hidden">
                <Table>
                  <TableHeader>
                    <TableRow>
                      {historyHeads.map((h) => (
                        <TableHead key={h}>{h}</TableHead>
                      ))}
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {history.map((task, i) => (
                      <TableRow key={task.id} index={i}>
                        <TableCell className="whitespace-nowrap">{kindLabel[task.kind]}</TableCell>
                        <TableCell className="text-foreground">
                          <span>{task.label}</span>
                          {task.state === "failed" && task.error && (
                            <p className={cn(typeClass("caption"), "text-muted-foreground")}>{task.error}</p>
                          )}
                        </TableCell>
                        <TableCell>
                          {task.state === "done" && (
                            <Badge variant="ghost" color="gray">
                              {t("done")}
                            </Badge>
                          )}
                          {task.state === "failed" && (
                            <Badge variant="ghost" color="red">
                              {t("failed")}
                            </Badge>
                          )}
                          {task.state === "cancelled" && (
                            <Badge variant="ghost" color="gray">
                              {t("stopped")}
                            </Badge>
                          )}
                        </TableCell>
                        <TableCell className="whitespace-nowrap">{relTime(task.startedAt)}</TableCell>
                      </TableRow>
                    ))}
                    {history.length === 0 && (
                      <TableEmpty columns={historyHeads.length} title={t("historyEmpty")} rows={2} />
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
        title={t("stopAllTitle")}
        body={t("stopAllBody")}
        confirmLabel={t("stopAll")}
        onConfirm={() => void cancelAll()}
      />
      <ConfirmDialog
        open={confirm === "trash"}
        onOpenChange={(open) => !open && setConfirm(null)}
        title={t("emptyTrashTitle")}
        body={t("emptyTrashBody")}
        confirmLabel={t("empty")}
        onConfirm={() => void emptyTrash()}
      />
    </Page>
  );
}
