import type { IconComponent } from "@/lib/icon-context";
import { useMemo, useState, type ReactNode } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router";
import { toast } from "@yusufceng1z/ding-react";
import { ArrowUpDown, CircleCheck, MoreHorizontal, Radar, Scan, Settings, SlidersHorizontal, X } from "@keyline-icons/react";
import { api, formatBytes, statusLabel, useApi, type SeriesSort, type SeriesStatus } from "@/api";
import { SeriesCard } from "@/components/media/cards";
import { CountUp } from "@/components/media/count-up";
import { CoverGrid, EmptyState, Page, PageHeader } from "@/components/media/layout";
import { KindIcon } from "@/components/media/kind-icon";
import { GridSkeleton } from "@/components/media/skeletons";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { DropdownContent, DropdownMenu, DropdownTrigger } from "@/components/ui/dropdown";
import { MenuItem } from "@/components/ui/menu-item";
import { Select, SelectContent, SelectItem, SelectTrigger } from "@/components/ui/select";
import { Tooltip } from "@/components/ui/tooltip";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";

const SORTS: { value: SeriesSort; label: string }[] = [
  { value: "title", label: "タイトル" },
  { value: "updated", label: "更新日" },
  { value: "created", label: "追加日" },
  { value: "bookCount", label: "巻数" },
];
type ReadState = "unread" | "inProgress" | "read";
const READ_STATES: { value: ReadState; label: string }[] = [
  { value: "unread", label: "未読" },
  { value: "inProgress", label: "読みかけ" },
  { value: "read", label: "既読" },
];
const STATUSES: SeriesStatus[] = ["ongoing", "ended", "hiatus", "abandoned"];

export default function Screen() {
  const { libraryId = "all" } = useParams();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const [panelOpen, setPanelOpen] = useState(false);

  const isAll = libraryId === "all";
  const scope = isAll ? undefined : libraryId;

  // URL is the source of truth for sort and filters (comma-separated lists).
  // Unknown values from the URL are dropped (sort falls back to "title").
  const sort = parseSort(params.get("sort"));
  const desc = params.get("desc") === "1";
  const read = list(params.get("read")).filter(isReadState);
  const status = list(params.get("status")).filter(isStatus);
  const genre = list(params.get("genre"));
  const filterCount = read.length + status.length + genre.length;

  const update = (patch: Record<string, string | null>) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const [k, v] of Object.entries(patch)) {
          if (v) next.set(k, v);
          else next.delete(k);
        }
        return next;
      },
      { replace: true },
    );

  const toggle = (key: string, current: string[], value: string) => {
    const next = current.includes(value) ? current.filter((x) => x !== value) : [...current, value];
    update({ [key]: next.join(",") || null });
  };
  const clearAll = () => update({ read: null, status: null, genre: null });

  const libs = useApi(() => api.libraries(), []);
  // Unfiltered list of this scope: source for the genre options.
  const scoped = useApi(() => api.series({ libraryId: scope }), [scope]);
  const { data: series } = useApi(
    () =>
      api.series({
        libraryId: scope,
        sort,
        desc,
        readState: read.length ? read : undefined,
        status: status.length ? status : undefined,
        genres: genre.length ? genre : undefined,
      }),
    [scope, sort, desc, read.join(), status.join(), genre.join()],
  );

  const genres = useMemo(
    () => [...new Set((scoped.data ?? []).flatMap((s) => s.genres))].sort((a, b) => a.localeCompare(b, "ja")),
    [scoped.data],
  );

  const library = libs.data?.find((l) => l.id === libraryId);
  const libraryList = libs.data ?? [];
  const totals = isAll
    ? libraryList.reduce(
        (t, l) => ({ series: t.series + l.seriesCount, books: t.books + l.bookCount, size: t.size + l.sizeBytes }),
        { series: 0, books: 0, size: 0 },
      )
    : library && { series: library.seriesCount, books: library.bookCount, size: library.sizeBytes };
  const title = isAll ? "すべてのライブラリ" : (library?.name ?? "ライブラリ");
  const meta = totals ? (
    <span className="font-mono">
      シリーズ <CountUp value={totals.series} /> · 本 <CountUp value={totals.books} /> · {formatBytes(totals.size)}
    </span>
  ) : undefined;

  const scanItems = [
    { label: "スキャン", icon: Scan, run: () => toast.success("スキャンを開始しました") },
    { label: "詳細スキャン", icon: Radar, run: () => toast.success("スキャンを開始しました") },
  ];
  const menuItems = [
    ...(isAll ? [] : scanItems),
    { label: "ライブラリを既読にする", icon: CircleCheck, run: () => toast.success("既読にしました") },
    ...(isAll ? [] : [{ label: "ライブラリ設定", icon: Settings, run: () => navigate("/admin/libraries") }]),
  ];

  const chips = [
    ...read.map((v) => ({ key: `read-${v}`, label: READ_STATES.find((r) => r.value === v)?.label ?? v, remove: () => toggle("read", read, v) })),
    ...status.map((v) => ({ key: `status-${v}`, label: statusLabel[v], remove: () => toggle("status", status, v) })),
    ...genre.map((v) => ({ key: `genre-${v}`, label: v, remove: () => toggle("genre", genre, v) })),
  ];

  const header = (
    <PageHeader
      icon={library ? <KindIcon kind={library.kind} size={52} /> : undefined}
      title={title}
      meta={meta}
      actions={
        isAll ? undefined : (
          <DropdownMenu>
            <Tooltip content="その他の操作">
              <span className="inline-flex">
                <DropdownTrigger render={<Button variant="ghost" size="icon" aria-label="その他の操作" />}>
                  <MoreHorizontal size={16} />
                </DropdownTrigger>
              </span>
            </Tooltip>
            <DropdownContent align="end">
              {menuItems.map((item, i) => (
                <MenuItem key={item.label} index={i} label={item.label} icon={item.icon} onSelect={item.run} />
              ))}
            </DropdownContent>
          </DropdownMenu>
        )
      }
    />
  );

  if (!series) {
    return (
      <Page>
        {header}
        <GridSkeleton />
      </Page>
    );
  }

  return (
    <Page>
      {header}

      <div className="sticky top-0 z-10 mb-4 flex flex-wrap items-center justify-between gap-2 bg-surface-2 py-2">
        <p className={cn(typeClass("caption"), "font-mono text-muted-foreground tabular-nums")}>{series.length} シリーズ</p>
        <div className="flex items-center gap-2">
          <Select value={sort} onValueChange={(v) => update({ sort: v === "title" ? null : v })}>
            <SelectTrigger aria-label="並び順" />
            <SelectContent>
              {SORTS.map((s, i) => (
                <SelectItem key={s.value} index={i} value={s.value}>
                  {s.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Tooltip content={desc ? "降順" : "昇順"}>
            <Button
              variant="ghost"
              size="icon"
              aria-label="並び順を逆にする"
              onClick={() => update({ desc: desc ? null : "1" })}
            >
              <ArrowUpDown size={16} />
            </Button>
          </Tooltip>
          <Button
            variant="secondary"
            aria-expanded={panelOpen}
            onClick={() => setPanelOpen((o) => !o)}
            leadingIcon={SlidersHorizontal as IconComponent}
          >
            {filterCount > 0 ? `絞り込み ${filterCount}` : "絞り込み"}
          </Button>
        </div>
      </div>

      {panelOpen && (
        <div className="mb-4 grid gap-4 rounded-[12px] bg-card p-4 md:grid-cols-3">
          <FilterGroup title="既読状態">
            {READ_STATES.map((o) => (
              <CheckRow key={o.value} label={o.label} checked={read.includes(o.value)} onChange={() => toggle("read", read, o.value)} />
            ))}
          </FilterGroup>
          <FilterGroup title="ステータス">
            {STATUSES.map((v) => (
              <CheckRow key={v} label={statusLabel[v]} checked={status.includes(v)} onChange={() => toggle("status", status, v)} />
            ))}
          </FilterGroup>
          <FilterGroup title="ジャンル">
            {genres.map((g) => (
              <CheckRow key={g} label={g} checked={genre.includes(g)} onChange={() => toggle("genre", genre, g)} />
            ))}
          </FilterGroup>
          <div className="flex justify-end md:col-span-3">
            <Button variant="ghost" onClick={clearAll}>
              クリア
            </Button>
          </div>
        </div>
      )}

      {chips.length > 0 && (
        <div className="mb-4 flex flex-wrap gap-2">
          {chips.map((c) => (
            <button
              key={c.key}
              type="button"
              onClick={c.remove}
              aria-label={`${c.label}を外す`}
              className={cn(
                typeClass("body"),
                "inline-flex h-9 items-center gap-1 rounded-full bg-muted px-3 text-foreground outline-none focus-visible:ring-2 ring-[var(--focus-ring)]",
              )}
            >
              <span className="[text-box:trim-both_cap_alphabetic]">{c.label}</span>
              <X size={14} />
            </button>
          ))}
        </div>
      )}

      {series.length === 0 ? (
        filterCount > 0 ? (
          <EmptyState
            title="条件に合うシリーズがありません"
            action={
              <Button variant="secondary" onClick={clearAll}>
                絞り込みをクリア
              </Button>
            }
          />
        ) : (
          <EmptyState title="シリーズがありません" body="ライブラリをスキャンすると、ここに表示されます" />
        )
      ) : (
        <CoverGrid>
          {series.map((s, i) => (
            <SeriesCard key={s.id} series={s} priority={i < 8} />
          ))}
        </CoverGrid>
      )}
    </Page>
  );
}

function list(raw: string | null): string[] {
  return raw ? raw.split(",").filter(Boolean) : [];
}

function isSort(v: string | null): v is SeriesSort {
  return SORTS.some((s) => s.value === v);
}

function parseSort(raw: string | null): SeriesSort {
  return isSort(raw) ? raw : "title";
}

function isReadState(v: string): v is ReadState {
  return READ_STATES.some((r) => r.value === v);
}

function isStatus(v: string): v is SeriesStatus {
  return STATUSES.includes(v as SeriesStatus);
}

function FilterGroup({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="space-y-2">
      <p className={cn(typeClass("caption"), "text-muted-foreground")}>{title}</p>
      <div className="flex flex-col gap-2">{children}</div>
    </div>
  );
}

function CheckRow({ label, checked, onChange }: { label: string; checked: boolean; onChange: () => void }) {
  return (
    <label className={cn(typeClass("body"), "flex min-h-9 cursor-pointer items-center gap-2 text-foreground")}>
      <Checkbox checked={checked} onCheckedChange={onChange} />
      {label}
    </label>
  );
}
