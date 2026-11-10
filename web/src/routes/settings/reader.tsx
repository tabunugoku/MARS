import { useState } from "react";
import { Link } from "react-router";
import {
  api,
  useApi,
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
  type GlobalReaderPrefs,
} from "@/api";
import { Page, PageHeader, SectionTitle } from "@/components/media/layout";
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

export default function Screen() {
  const [prefs, setPrefs] = useState<GlobalReaderPrefs>(getGlobalPrefs);
  const [customSeries, setCustomSeries] = useState<string[]>(customSeriesIds);
  const [customBooks, setCustomBooks] = useState<string[]>(customBookIds);
  const { data: series } = useApi(() => api.series(), []);
  const { data: books } = useApi(() => api.booksByIds(customBooks), [customBooks.join()]);
  const titles = new Map(series?.map((s) => [s.id, s.title]));
  const rows = [
    ...customSeries.map((id) => ({ key: `s:${id}`, kind: "series" as const, id, to: `/series/${id}`, title: titles.get(id) ?? id, scope: "シリーズ共通" })),
    ...customBooks.map((id) => {
      const b = books?.find((x) => x.id === id);
      return { key: `b:${id}`, kind: "book" as const, id, to: `/books/${id}`, title: b ? `${titles.get(b.seriesId) ?? ""} ${b.title}`.trim() : id, scope: "この巻だけ" };
    }),
  ];

  const patch = (p: Partial<GlobalReaderPrefs>) => {
    const next = { ...prefs, ...p };
    setPrefs(next);
    setGlobalPrefs(next);
  };
  const reset = (targets: { kind: "series" | "book"; id: string }[]) => {
    targets.forEach((t) => (t.kind === "series" ? setSeriesPrefs : setBookPrefs)(t.id, { mode: "global" }));
    setCustomSeries(customSeriesIds());
    setCustomBooks(customBookIds());
    toast.success(targets.length > 1 ? "すべて全体設定に戻しました" : "全体設定に戻しました");
  };

  return (
    <Page>
      <PageHeader title="リーダー設定" meta="すべての作品に使う表示設定です。シリーズや巻ごとの設定は、リーダーの表示設定で選べます。" />

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
          <SectionTitle action={rows.length > 1 ? <Button variant="secondary" size="compact" onClick={() => reset(rows)}>すべて戻す</Button> : undefined}>
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
                  <Button variant="secondary" size="compact" onClick={() => reset([r])}>
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
