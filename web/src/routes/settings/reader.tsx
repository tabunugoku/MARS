import { useState } from "react";
import { Link } from "react-router";
import {
  api,
  useApi,
  errorMessage,
  loadReaderPrefs,
  bgLabel,
  directionLabel,
  fitLabel,
  spreadLabel,
  customBookIds,
  customSeriesIds,
  profileForLabel,
  profileLabel,
  activeProfile,
  detectedProfile,
  getGlobalPrefs,
  getProfileChoice,
  setProfileChoice,
  setBookPrefs,
  setGlobalPrefs,
  setSeriesPrefs,
  type Book,
  type GlobalReaderPrefs,
  type ReaderProfile,
  type ReaderProfileChoice,
} from "@/api";
import { Page, PageHeader, SectionTitle } from "@/components/media/layout";
import { Block } from "@/components/media/skeletons";
import { LoadError } from "@/components/shell/load-error";
import { Button } from "@/components/ui/button";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { RadioGroup, RadioItem } from "@/components/ui/radio-group";
import { Switch } from "@/components/ui/switch";
import { toast } from "@yusufceng1z/ding-react";
import { typeClass } from "@/lib/type-scale";
import { cn } from "@/lib/utils";
import { defineMessages, useT } from "@/i18n";

// The master reader settings. Every volume follows these unless it or its
// series was given its own in the reader's 表示設定 panel. Changes apply at once.
// The master set has a PC and a mobile profile; each browser reads with one.

const m = defineMessages({
  ja: {
    title: "リーダー設定",
    meta: "すべての作品に使う表示設定です。PC 用とモバイル用があり、端末ごとにどちらを使うか選べます。",
    metaPerBook: "シリーズや巻ごとの設定は、リーダーの表示設定で選べ、どちらの端末にも効きます。",
    followSeries: "作品の指定に従う",
    thisDevice: "この端末",
    deviceSettings: "この端末で使う設定",
    deviceHint: "この端末のブラウザにだけ保存されます。",
    auto: "自動（{profile}）",
    display: "表示",
    editing: "変更する設定",
    direction: "読み方向",
    directionHint: "作品の指定に従うと、マンガは右→左、Webtoon は縦スクロールで開きます。",
    spread: "見開き",
    pageSize: "表示サイズ",
    background: "背景",
    splitWide: "縦長の画面では、横長のページを 2 ページに分ける",
    splitWideHint: "見開きで取り込まれたページを、読む順に半分ずつ表示します。",
    resetAll: "すべて戻す",
    customSection: "独自の設定があるシリーズと巻",
    noneCustom: "ありません。すべての作品がこの設定で開きます。",
    scopeSeries: "シリーズ共通",
    scopeBook: "この巻だけ",
    resetOne: "全体設定に戻す",
    resetAllDone: "すべて全体設定に戻しました",
    resetDone: "全体設定に戻しました",
  },
  en: {
    title: "Reader settings",
    meta: "The display settings for every work. There is a PC set and a mobile set, and each device can choose which one it uses.",
    metaPerBook: "Series and volume settings are chosen in the reader's display settings and apply on both devices.",
    followSeries: "Follow the series",
    thisDevice: "This device",
    deviceSettings: "Settings for this device",
    deviceHint: "Saved only in this browser.",
    auto: "Auto ({profile})",
    display: "Display",
    editing: "Settings to edit",
    direction: "Reading direction",
    directionHint: "Follow the series opens manga right to left and webtoons with vertical scrolling.",
    spread: "Spread",
    pageSize: "Page size",
    background: "Background",
    splitWide: "On portrait screens, split wide pages into 2 pages",
    splitWideHint: "Shows pages scanned as spreads one half at a time, in reading order.",
    resetAll: "Reset all",
    customSection: "Series and volumes with their own settings",
    noneCustom: "None. Every work opens with these settings.",
    scopeSeries: "Whole series",
    scopeBook: "This book only",
    resetOne: "Reset to global",
    resetAllDone: "All reset to global settings",
    resetDone: "Reset to global settings",
  },
});

type Target = { kind: "series" | "book"; id: string };

function Header() {
  const t = useT(m);
  return (
    <PageHeader
      title={t("title")}
      meta={
        <>
          {t("meta")}
          <br />
          {t("metaPerBook")}
        </>
      }
    />
  );
}

const profileOptions = () => Object.entries(profileLabel()) as [ReaderProfile, string][];

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
        <Header />
        <LoadError message={error} onRetry={reload} />
      </Page>
    );
  }
  if (!data) {
    return (
      <Page>
        <Header />
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
  const t = useT(m);
  const directionOpts: [GlobalReaderPrefs["direction"], string][] = [
    ["series", t("followSeries")],
    ...(Object.entries(directionLabel()) as [GlobalReaderPrefs["direction"], string][]),
  ];
  // Which profile this browser reads with, and which one the form edits.
  const [choice, setChoice] = useState<ReaderProfileChoice>(getProfileChoice);
  const [editing, setEditing] = useState<ReaderProfile>(activeProfile);
  const [prefs, setPrefs] = useState<GlobalReaderPrefs>(() => getGlobalPrefs(activeProfile()));
  const [customSeries, setCustomSeries] = useState<string[]>(customSeriesIds);
  const [customBooks, setCustomBooks] = useState<string[]>(customBookIds);
  // One write at a time: changes made while one is in flight are ignored.
  const [busy, setBusy] = useState(false);

  const rows = [
    ...customSeries.map((id) => ({ key: `s:${id}`, kind: "series" as const, id, to: `/series/${id}`, title: titles.get(id) ?? id, scope: t("scopeSeries") })),
    ...customBooks.map((id) => {
      const b = books.find((x) => x.id === id);
      return { key: `b:${id}`, kind: "book" as const, id, to: `/books/${id}`, title: b ? `${titles.get(b.seriesId) ?? ""} ${b.title}`.trim() : id, scope: t("scopeBook") };
    }),
  ];

  const patch = async (p: Partial<GlobalReaderPrefs>) => {
    if (busy) return;
    const next = { ...prefs, ...p };
    setPrefs(next);
    setBusy(true);
    try {
      await setGlobalPrefs(next, editing);
    } catch (err) {
      // The cache is already back to what the server holds (for a mobile set
      // never saved, nothing: it keeps following the PC one). Show that.
      setPrefs(getGlobalPrefs(editing));
      toast.error(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const edit = (p: ReaderProfile) => {
    if (busy) return;
    setEditing(p);
    setPrefs(getGlobalPrefs(p));
  };
  const choose = (c: ReaderProfileChoice) => {
    setProfileChoice(c);
    setChoice(c);
    edit(c === "auto" ? detectedProfile() : c);
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
    else toast.success(targets.length > 1 ? t("resetAllDone") : t("resetDone"));
  };

  return (
    <Page>
      <Header />

      <div className="max-w-[640px] space-y-10">
        <section>
          <SectionTitle>{t("thisDevice")}</SectionTitle>
          <FieldGroup>
            <Choice
              id="rp-profile"
              label={t("deviceSettings")}
              hint={t("deviceHint")}
              value={choice}
              options={[["auto", t("auto", { profile: profileLabel()[detectedProfile()] })], ...profileOptions()]}
              onChange={choose}
            />
          </FieldGroup>
        </section>

        <section>
          <SectionTitle>{t("display")}</SectionTitle>
          <FieldGroup>
            <Choice id="rp-editing" label={t("editing")} value={editing} options={profileOptions().map(([v]) => [v, profileForLabel()[v]])} onChange={edit} />
            <Choice
              id="rp-direction"
              label={t("direction")}
              hint={t("directionHint")}
              value={prefs.direction}
              options={directionOpts}
              onChange={(direction) => patch({ direction })}
            />
            <Choice id="rp-spread" label={t("spread")} value={prefs.spread} options={Object.entries(spreadLabel()) as [GlobalReaderPrefs["spread"], string][]} onChange={(spread) => patch({ spread })} />
            <Choice id="rp-fit" label={t("pageSize")} value={prefs.fit} options={Object.entries(fitLabel()) as [GlobalReaderPrefs["fit"], string][]} onChange={(fit) => patch({ fit })} />
            <Choice id="rp-bg" label={t("background")} value={prefs.bg} options={Object.entries(bgLabel()) as [GlobalReaderPrefs["bg"], string][]} onChange={(bg) => patch({ bg })} />
            <Field>
              <Switch
                label={t("splitWide")}
                checked={!!prefs.splitWide}
                disabled={busy}
                onToggle={() => patch({ splitWide: !prefs.splitWide })}
              />
              <p className={cn(typeClass("caption"), "text-muted-foreground")}>{t("splitWideHint")}</p>
            </Field>
          </FieldGroup>
        </section>

        <section>
          <SectionTitle action={rows.length > 1 ? <Button variant="secondary" size="compact" disabled={busy} onClick={() => reset(rows)}>{t("resetAll")}</Button> : undefined}>
            {t("customSection")}
          </SectionTitle>
          {rows.length === 0 ? (
            <p className={cn(typeClass("body"), "text-muted-foreground")}>{t("noneCustom")}</p>
          ) : (
            <ul className="divide-y divide-border rounded-xl border border-border bg-card">
              {rows.map((r) => (
                <li key={r.key} className="flex items-center gap-3 px-4 py-2">
                  <Link to={r.to} className={cn(typeClass("body"), "min-w-0 flex-1 truncate text-foreground hover:underline")}>
                    {r.title}
                  </Link>
                  <span className={cn(typeClass("caption"), "shrink-0 text-muted-foreground")}>{r.scope}</span>
                  <Button variant="secondary" size="compact" disabled={busy} onClick={() => reset([r])}>
                    {t("resetOne")}
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
