import { useState } from "react";
import { Link } from "react-router";
import {
  api,
  useApi,
  errorMessage,
  loadReaderPrefs,
  BG_LABEL,
  DIRECTION_LABEL,
  FIT_LABEL,
  SPREAD_LABEL,
  customBookIds,
  customSeriesIds,
  getGlobalPrefs,
  setBookPrefs,
  setGlobalPrefs,
  setSeriesPrefs,
  type Book,
  type GlobalReaderPrefs,
} from "@/api";
import { Page, PageHeader, SectionTitle } from "@/components/media/layout";
import { Block } from "@/components/media/skeletons";
import { LoadError } from "@/components/shell/load-error";
import { Button } from "@/components/ui/button";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { RadioGroup, RadioItem } from "@/components/ui/radio-group";
import { toast } from "@yusufceng1z/ding-react";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";

// The master reader settings. Every volume follows these unless it or its
// series was given its own in the reader's 表示設定 panel. Changes apply at once.

const DIRECTION_OPTIONS: [GlobalReaderPrefs["direction"], string][] = [
  ["series", "作品の指定に従う"],
  ...(Object.entries(DIRECTION_LABEL) as [GlobalReaderPrefs["direction"], string][]),
];

type Target = { kind: "series" | "book"; id: string };

const HEADER = <PageHeader title="リーダー設定" meta="すべての作品に使う表示設定です。シリーズや巻ごとの設定は、リーダーの表示設定で選べます。" />;

function Choice<T extends string>({ id, label, hint, value, options, onChange }: { id: string; label: string; hint?: string; value: T; options: [T, string][]; onChange: (v: T) => void }) {
  return (
    <Field>
      <FieldLabel id={id}>{label}</FieldLabel>
      <RadioGroup aria-labelledby={id} value={value} onValueChange={(v) => onChange(v as T)}>
        {options.map(([v, text], i) => (
          <RadioItem key={v} index={i} value={v} label={text} />
        ))}
      </RadioGroup>
      {hint && <p className={cn(typeClass("caption"), "text-muted-foreground")}>{hint}</p>}
    </Field>
  );
}

// Loads the reader settings (the cache the getters read) and the titles of the
// works that keep their own settings, then mounts the form.
export default function Screen() {
  const { data, error, reload } = useApi(async () => {
    await loadReaderPrefs();
    const books = await api.booksByIds(customBookIds());
    const seriesIds = new Set([...customSeriesIds(), ...books.map((b) => b.seriesId)]);
    const series = await api.seriesByIds([...seriesIds]);
    return { books, titles: new Map(series.map((s) => [s.id, s.title])) };
  }, []);

  if (error) {
    return (
      <Page>
        {HEADER}
        <LoadError message={error} onRetry={reload} />
      </Page>
    );
  }
  if (!data) {
    return (
      <Page>
        {HEADER}
        <div className="max-w-[640px] space-y-3" aria-hidden>
          <Block className="h-5 w-24" />
          <Block className="h-9 w-full" />
          <Block className="h-9 w-full" />
        </div>
      </Page>
    );
  }
  return <SettingsForm books={data.books} titles={data.titles} />;
}

function SettingsForm({ books, titles }: { books: Book[]; titles: Map<string, string> }) {
  const [prefs, setPrefs] = useState<GlobalReaderPrefs>(getGlobalPrefs);
  const [customSeries, setCustomSeries] = useState<string[]>(customSeriesIds);
  const [customBooks, setCustomBooks] = useState<string[]>(customBookIds);
  // One write at a time: changes made while one is in flight are ignored.
  const [busy, setBusy] = useState(false);

  const rows = [
    ...customSeries.map((id) => ({ key: `s:${id}`, kind: "series" as const, id, to: `/series/${id}`, title: titles.get(id) ?? id, scope: "シリーズ共通" })),
    ...customBooks.map((id) => {
      const b = books.find((x) => x.id === id);
      return { key: `b:${id}`, kind: "book" as const, id, to: `/books/${id}`, title: b ? `${titles.get(b.seriesId) ?? ""} ${b.title}`.trim() : id, scope: "この巻だけ" };
    }),
  ];

  const patch = async (p: Partial<GlobalReaderPrefs>) => {
    if (busy) return;
    const previous = prefs;
    const next = { ...prefs, ...p };
    setPrefs(next);
    setBusy(true);
    try {
      await setGlobalPrefs(next);
    } catch (err) {
      // Go back to the saved choice, and write it again so the cache matches the server.
      setPrefs(previous);
      setGlobalPrefs(previous).catch(() => {});
      toast.error(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const reset = async (targets: Target[]) => {
    if (busy) return;
    setBusy(true);
    const results = await Promise.allSettled(
      targets.map((t) => (t.kind === "series" ? setSeriesPrefs(t.id, { mode: "global" }) : setBookPrefs(t.id, { mode: "global" }))),
    );
    // Drop only the rows the server accepted; a failed one stays listed.
    const doneSeries = new Set(targets.filter((t, i) => t.kind === "series" && results[i].status === "fulfilled").map((t) => t.id));
    const doneBooks = new Set(targets.filter((t, i) => t.kind === "book" && results[i].status === "fulfilled").map((t) => t.id));
    setCustomSeries((xs) => xs.filter((id) => !doneSeries.has(id)));
    setCustomBooks((xs) => xs.filter((id) => !doneBooks.has(id)));
    setBusy(false);
    const failed = results.find((r): r is PromiseRejectedResult => r.status === "rejected");
    if (failed) toast.error(errorMessage(failed.reason));
    else toast.success(targets.length > 1 ? "すべて全体設定に戻しました" : "全体設定に戻しました");
  };

  return (
    <Page>
      {HEADER}

      <div className="max-w-[640px] space-y-10">
        <section>
          <SectionTitle>表示</SectionTitle>
          <FieldGroup>
            <Choice
              id="rp-direction"
              label="読み方向"
              hint="作品の指定に従うと、マンガは右→左、Webtoon は縦スクロールで開きます。"
              value={prefs.direction}
              options={DIRECTION_OPTIONS}
              onChange={(direction) => patch({ direction })}
            />
            <Choice id="rp-spread" label="見開き" value={prefs.spread} options={Object.entries(SPREAD_LABEL) as [GlobalReaderPrefs["spread"], string][]} onChange={(spread) => patch({ spread })} />
            <Choice id="rp-fit" label="表示サイズ" value={prefs.fit} options={Object.entries(FIT_LABEL) as [GlobalReaderPrefs["fit"], string][]} onChange={(fit) => patch({ fit })} />
            <Choice id="rp-bg" label="背景" value={prefs.bg} options={Object.entries(BG_LABEL) as [GlobalReaderPrefs["bg"], string][]} onChange={(bg) => patch({ bg })} />
          </FieldGroup>
        </section>

        <section>
          <SectionTitle action={rows.length > 1 ? <Button variant="secondary" size="compact" disabled={busy} onClick={() => reset(rows)}>すべて戻す</Button> : undefined}>
            独自の設定があるシリーズと巻
          </SectionTitle>
          {rows.length === 0 ? (
            <p className={cn(typeClass("body"), "text-muted-foreground")}>ありません。すべての作品がこの設定で開きます。</p>
          ) : (
            <ul className="divide-y divide-border rounded-xl border border-border bg-card">
              {rows.map((r) => (
                <li key={r.key} className="flex items-center gap-3 px-4 py-2">
                  <Link to={r.to} className={cn(typeClass("body"), "min-w-0 flex-1 truncate text-foreground hover:underline")}>
                    {r.title}
                  </Link>
                  <span className={cn(typeClass("caption"), "shrink-0 text-muted-foreground")}>{r.scope}</span>
                  <Button variant="secondary" size="compact" disabled={busy} onClick={() => reset([r])}>
                    全体設定に戻す
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </Page>
  );
}
