import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import { motion, useTransform } from "motion/react";
import {
  ChevronLeft,
  Settings,
  Grid2x2,
  Maximize,
  Minimize,
  EyeOff,
} from "@keyline-icons/react";
import { api, useApi, type Book, type ReadingDirection } from "@/api";
import { pageArt, coverArt } from "@/lib/art";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";
import { BrandAction } from "@/components/media/brand-action";
import { usePageSwipe } from "./use-page-swipe";
import { ReaderSettings, type ReaderPrefs, loadPrefs, savePrefs } from "./settings";

// ---------------------------------------------------------------------------
// Reader. Priorities, in order: a page turn never waits (neighbours are
// mounted and decoded ahead), the gesture follows the finger, and chrome
// stays out of the way until asked for (tap the centre, or press M).
// ---------------------------------------------------------------------------

type View = { pages: number[] }; // one or two page numbers shown together

/** Group pages into views. Spread mode pairs pages; "cover" keeps page 1 alone. */
function buildViews(count: number, spread: ReaderPrefs["spread"]): View[] {
  const views: View[] = [];
  let p = 1;
  if (spread !== "off" && spread === "cover") views.push({ pages: [p++] });
  while (p <= count) {
    if (spread === "off" || p === count) views.push({ pages: [p++] });
    else {
      views.push({ pages: [p, p + 1] });
      p += 2;
    }
  }
  return views;
}

const bgClass: Record<ReaderPrefs["bg"], string> = {
  black: "bg-black text-white",
  gray: "bg-neutral-800 text-white",
  white: "bg-[#f4f2ec] text-neutral-900",
};

export default function Reader() {
  const { bookId = "" } = useParams();
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const incognito = params.get("incognito") === "1";

  const { data, loading } = useApi(async () => {
    const book = await api.book(bookId);
    if (!book) return null;
    const [series, adj] = await Promise.all([api.seriesById(book.seriesId), api.adjacentBooks(book.id)]);
    return { book, series, adj };
  }, [bookId]);

  if (loading && !data) return <div className="fixed inset-0 bg-black" aria-busy />;
  if (!data) {
    return (
      <div className="fixed inset-0 flex flex-col items-center justify-center gap-3 bg-black text-white">
        <p className={typeClass("title")}>この本は見つかりませんでした</p>
        <Link to="/" className={cn(typeClass("body"), "text-white/70 underline-offset-4 hover:underline")}>ホームへ戻る</Link>
      </div>
    );
  }
  return (
    <ReaderBody
      key={data.book.id}
      book={data.book}
      seriesTitle={data.series?.title ?? ""}
      defaultDirection={data.series?.readingDirection ?? "rtl"}
      next={data.adj.next}
      startPage={Number(params.get("page")) || (data.book.progress && !data.book.progress.completed ? data.book.progress.page : 1)}
      incognito={incognito}
      onExit={() => navigate(`/books/${data.book.id}`)}
    />
  );
}

function ReaderBody({
  book,
  seriesTitle,
  defaultDirection,
  next,
  startPage,
  incognito,
  onExit,
}: {
  book: Book;
  seriesTitle: string;
  defaultDirection: ReadingDirection;
  next: Book | null;
  startPage: number;
  incognito: boolean;
  onExit: () => void;
}) {
  const [prefs, setPrefs] = useState<ReaderPrefs>(() => loadPrefs(book.seriesId, defaultDirection));
  useEffect(() => savePrefs(book.seriesId, prefs), [book.seriesId, prefs]);
  const [chrome, setChrome] = useState(true);
  const [panel, setPanel] = useState<"none" | "settings" | "pages">("none");
  const [fullscreen, setFullscreen] = useState(false);

  const webtoon = prefs.direction === "webtoon";
  const views = useMemo(() => buildViews(book.pageCount, webtoon ? "off" : prefs.spread), [book.pageCount, prefs.spread, webtoon]);
  // index views.length = the end card
  const [vi, setVi] = useState(() => Math.max(0, views.findIndex((v) => v.pages.includes(startPage))));
  const viewIndex = Math.min(vi, views.length);
  const atEnd = viewIndex === views.length;
  const currentPage = atEnd ? book.pageCount : views[viewIndex].pages[0];

  // The page the reader is "on", independent of grouping (Infinity = end card).
  // When the spread layout changes, re-find it in the new views. This effect
  // is declared before the one that records the anchor, so it reads the page
  // from the previous layout, not the one recomputed with the stale index.
  const anchor = useRef<number>(currentPage);
  const [webtoonPage, setWebtoonPage] = useState(currentPage);
  useEffect(() => {
    const a = anchor.current;
    setVi(a === Infinity ? views.length : Math.max(0, views.findIndex((v) => v.pages.includes(a))));
    setWebtoonPage(a === Infinity ? book.pageCount : a);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [views]);
  useEffect(() => {
    if (!webtoon) anchor.current = atEnd ? Infinity : currentPage;
  }, [atEnd, currentPage, webtoon]);
  const [jump, setJump] = useState<{ page: number; n: number } | null>(null);

  const axis = prefs.direction === "vertical" ? "y" : "x";
  const forward = prefs.direction === "rtl" ? -1 : 1;

  const stageRef = useRef<HTMLDivElement>(null);
  const commit = useCallback(
    (d: 1 | -1) => {
      setVi((i) => Math.max(0, Math.min(views.length, i + d)));
      setChrome(false);
    },
    [views.length],
  );

  const onTap = useCallback(
    (e: PointerEvent) => {
      const el = stageRef.current;
      if (!el) return;
      const r = el.getBoundingClientRect();
      const t = axis === "x" ? (e.clientX - r.left) / r.width : (e.clientY - r.top) / r.height;
      if (t > 0.3 && t < 0.7) return setChrome((c) => !c);
      // Zones follow the reading direction: in RTL the left side advances.
      const towardStart = t <= 0.3;
      const delta: 1 | -1 = axis === "y" ? (towardStart ? -1 : 1) : forward === -1 ? (towardStart ? 1 : -1) : towardStart ? -1 : 1;
      swipe.turn(delta);
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [axis, forward],
  );

  const swipe = usePageSwipe({
    stageRef,
    axis,
    forward,
    canPrev: viewIndex > 0,
    canNext: viewIndex < views.length,
    onCommit: commit,
    onTap,
  });
  const stripTransform = useTransform(swipe.offset, (v) => (axis === "x" ? `translate3d(${v}px,0,0)` : `translate3d(0,${v}px,0)`));

  // Keyboard: arrows follow the direction; space/PageDown always advance.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const k = e.key;
      // Shared shortcuts work in every mode and inside panels.
      if (k === "Escape") return panel !== "none" ? setPanel("none") : onExit();
      if (k === "m" || k === "M") return setChrome((c) => !c);
      // Page keys yield to focused controls and open panels (radios, slider, buttons keep their own keys).
      if (webtoon || panel !== "none") return;
      if ((e.target as HTMLElement)?.closest("input,textarea,select,button,a,[role=slider],[role=radio]")) return;
      let d: 0 | 1 | -1 = 0;
      if (k === " " || k === "PageDown") d = 1;
      else if (k === "PageUp") d = -1;
      else if (k === "ArrowRight") d = axis === "x" ? (forward === 1 ? 1 : -1) : 0;
      else if (k === "ArrowLeft") d = axis === "x" ? (forward === 1 ? -1 : 1) : 0;
      else if (k === "ArrowDown") d = axis === "y" ? 1 : 0;
      else if (k === "ArrowUp") d = axis === "y" ? -1 : 0;
      else if (k === "Home") return setVi(0);
      else if (k === "End") return setVi(views.length - 1);
      if (d) {
        e.preventDefault();
        swipe.turn(d);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [axis, forward, views.length, panel, onExit, swipe, webtoon]);

  const toggleFullscreen = () => {
    if (document.fullscreenElement) document.exitFullscreen();
    else document.documentElement.requestFullscreen?.();
  };
  useEffect(() => {
    const f = () => setFullscreen(!!document.fullscreenElement);
    document.addEventListener("fullscreenchange", f);
    return () => document.removeEventListener("fullscreenchange", f);
  }, []);

  const goToPage = (p: number) => {
    if (webtoon) {
      anchor.current = p;
      setWebtoonPage(p);
      setJump((j) => ({ page: p, n: (j?.n ?? 0) + 1 }));
    } else setVi(Math.max(0, views.findIndex((v) => v.pages.includes(p))));
    setPanel("none");
  };
  const shownPage = webtoon ? webtoonPage : currentPage;

  // Neighbour views are mounted beside the current one (prev/next) so a turn
  // shows decoded pixels; two further views are preloaded invisibly.
  const slot = (i: number) =>
    i < 0 ? null : i === views.length ? <EndCard key="end" book={book} next={next} seriesId={book.seriesId} /> : i > views.length ? null : (
      <PageView key={i} view={views[i]} seed={book.coverSeed} rtl={forward === -1} fit={prefs.fit} webtoon={false} />
    );

  const tone = bgClass[prefs.bg];

  return (
    <div className={cn("fixed inset-0 select-none overflow-clip", tone)} data-reader>
      {webtoon ? (
        <WebtoonView
          book={book}
          next={next}
          startPage={webtoonPage}
          jump={jump}
          onPage={(p) => {
            anchor.current = p;
            setWebtoonPage(p);
          }}
          onToggleChrome={() => setChrome((c) => !c)}
        />
      ) : (
        <div
          ref={stageRef}
          className="absolute inset-0 touch-none overflow-clip"
          style={{ touchAction: prefs.fit === "width" && axis === "x" ? "pan-y" : "none" }}
          {...swipe.handlers}
        >
          <motion.div className="absolute inset-0" style={{ transform: stripTransform }}>
            {[-1, 0, 1].map((d) => {
              const i = viewIndex + d;
              const node = slot(i);
              if (!node) return null;
              const shift = d * forward * 100; // LTR: next sits right; RTL: next sits left
              return (
                <div
                  key={`slot-${i}`}
                  className="absolute inset-0 flex items-center justify-center"
                  style={{ transform: axis === "x" ? `translateX(${shift}%)` : `translateY(${d * 100}%)` }}
                  aria-hidden={d !== 0}
                  inert={d !== 0}
                >
                  {node}
                </div>
              );
            })}
          </motion.div>
          <Preload seed={book.coverSeed} pages={[viewIndex + 2, viewIndex + 3].flatMap((i) => views[i]?.pages ?? [])} />
        </div>
      )}

      {/* Chrome: top and bottom bars. They slide out of the way, never block the page when hidden. */}
      <motion.header
        initial={false}
        animate={{ y: chrome ? 0 : "-100%", opacity: chrome ? 1 : 0 }}
        transition={{ type: "spring", duration: 0.24, bounce: 0 }}
        className="absolute inset-x-0 top-0 z-20 flex items-center gap-2 bg-gradient-to-b from-black/80 to-black/0 px-2 pb-6 pt-[max(env(safe-area-inset-top),8px)] text-white"
        style={{ pointerEvents: chrome ? "auto" : "none" }}
        inert={!chrome}
      >
        <IconBtn label="閉じる" onClick={onExit}>
          <ChevronLeft size={20} />
        </IconBtn>
        <div className="min-w-0 flex-1">
          <p className={cn(typeClass("subtitle"), "truncate")} style={{ fontVariationSettings: fontWeights.semibold }}>
            {seriesTitle}
          </p>
          <p className={cn(typeClass("caption"), "truncate text-white/70")}>{book.title}</p>
        </div>
        {incognito && (
          <span className={cn(typeClass("caption"), "inline-flex items-center gap-1 rounded-full bg-white/15 px-2 py-0.5")}>
            <EyeOff size={14} />
            シークレット
          </span>
        )}
        <IconBtn label="ページ一覧" onClick={() => setPanel((p) => (p === "pages" ? "none" : "pages"))} pressed={panel === "pages"}>
          <Grid2x2 size={18} />
        </IconBtn>
        <IconBtn label="表示設定" onClick={() => setPanel((p) => (p === "settings" ? "none" : "settings"))} pressed={panel === "settings"}>
          <Settings size={18} />
        </IconBtn>
        <IconBtn label={fullscreen ? "全画面を終了" : "全画面"} onClick={toggleFullscreen}>
          {fullscreen ? <Minimize size={18} /> : <Maximize size={18} />}
        </IconBtn>
      </motion.header>

      {!webtoon && (
        <motion.footer
          initial={false}
          animate={{ y: chrome ? 0 : "100%", opacity: chrome ? 1 : 0 }}
          transition={{ type: "spring", duration: 0.24, bounce: 0 }}
          className="absolute inset-x-0 bottom-0 z-20 bg-gradient-to-t from-black/80 to-black/0 px-4 pb-[max(env(safe-area-inset-bottom),12px)] pt-8 text-white"
          style={{ pointerEvents: chrome ? "auto" : "none" }}
          inert={!chrome}
        >
          <div className="mx-auto flex max-w-3xl items-center gap-3">
            <span className={cn(typeClass("caption"), "w-16 tabular-nums text-white/80")}>
              {currentPage} / {book.pageCount}
            </span>
            {/* Slider runs in reading direction: RTL books fill from the right. */}
            <input
              type="range"
              min={1}
              max={book.pageCount}
              value={currentPage}
              onChange={(e) => goToPage(Number(e.target.value))}
              aria-label="ページ"
              dir={forward === -1 ? "rtl" : "ltr"}
              className="h-1 flex-1 cursor-pointer accent-[var(--brand)]"
            />
            <span className={cn(typeClass("caption"), "w-16 text-right text-white/60")}>
              {prefs.direction === "rtl" ? "右から左" : prefs.direction === "ltr" ? "左から右" : "縦"}
            </span>
          </div>
        </motion.footer>
      )}

      {panel === "pages" && (
        <PageStrip
          book={book}
          current={shownPage}
          rtl={forward === -1}
          onPick={goToPage}
          onClose={() => setPanel("none")}
        />
      )}
      <ReaderSettings open={panel === "settings"} prefs={prefs} onChange={setPrefs} onClose={() => setPanel("none")} />
    </div>
  );
}

function IconBtn({ label, onClick, children, pressed }: { label: string; onClick: () => void; children: React.ReactNode; pressed?: boolean }) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      aria-pressed={pressed}
      onClick={onClick}
      className={cn(
        "flex size-10 items-center justify-center rounded-full transition-colors duration-80 hover:bg-white/10 active:bg-white/20",
        "outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)]",
        pressed && "bg-white/15",
      )}
    >
      {children}
    </button>
  );
}

function PageImg({ seed, page, fit, className }: { seed: number; page: number; fit: ReaderPrefs["fit"]; className?: string }) {
  const fitCls = {
    screen: "max-h-full max-w-full object-contain",
    height: "h-full w-auto max-w-none",
    width: "w-full h-auto max-h-none",
    original: "max-w-none max-h-none",
  }[fit];
  return <img src={pageArt(seed, page)} alt={`${page}ページ`} draggable={false} decoding="sync" className={cn(fitCls, className)} />;
}

function PageView({ view, seed, rtl, fit }: { view: View; seed: number; rtl: boolean; fit: ReaderPrefs["fit"]; webtoon: boolean }) {
  const pages = rtl ? [...view.pages].reverse() : view.pages;
  const scroll = fit === "width" || fit === "original";
  if (pages.length === 1) {
    return (
      <div className={cn("flex size-full justify-center", scroll ? "items-start overflow-auto" : "items-center")}>
        <PageImg seed={seed} page={pages[0]} fit={fit} />
      </div>
    );
  }
  // A spread: two halves meet at the gutter, each fitted to half the stage.
  return (
    <div className={cn("flex size-full justify-center", scroll ? "items-start overflow-auto" : "items-center")}>
      {pages.map((p) => (
        <div key={p} className={cn("flex h-full w-1/2 items-center", p === pages[0] ? "justify-end" : "justify-start")}>
          <PageImg seed={seed} page={p} fit={fit === "width" ? "width" : "screen"} />
        </div>
      ))}
    </div>
  );
}

function Preload({ seed, pages }: { seed: number; pages: number[] }) {
  return (
    <div className="pointer-events-none absolute size-px overflow-hidden opacity-0" aria-hidden>
      {pages.map((p) => (
        <img key={p} src={pageArt(seed, p)} alt="" decoding="async" />
      ))}
    </div>
  );
}

function EndCard({ book, next, seriesId }: { book: Book; next: Book | null; seriesId: string }) {
  return (
    <div className="flex max-w-sm flex-col items-center gap-4 px-6 text-center" onPointerDown={(e) => e.stopPropagation()}>
      <p className={typeClass("title")} style={{ fontVariationSettings: fontWeights.semibold }}>
        {book.title}を読み終わりました
      </p>
      {next ? (
        <>
          <img src={coverArt(next.coverSeed, next.title, String(next.number))} alt="" className="w-32 rounded-md shadow-lg" />
          <BrandAction to={`/read/${next.id}?page=1`}>次の巻を読む（{next.title}）</BrandAction>
        </>
      ) : (
        <p className={cn(typeClass("body"), "opacity-70")}>このシリーズの最新巻です。</p>
      )}
      <Link to={`/series/${seriesId}`} className={cn(typeClass("body"), "opacity-70 underline-offset-4 hover:underline")}>
        シリーズに戻る
      </Link>
    </div>
  );
}

function PageStrip({ book, current, rtl, onPick, onClose }: { book: Book; current: number; rtl: boolean; onPick: (p: number) => void; onClose: () => void }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    ref.current?.querySelector<HTMLElement>(`[data-page="${current}"]`)?.scrollIntoView({ inline: "center", block: "nearest" });
  }, [current]);
  return (
    <div className="absolute inset-x-0 bottom-0 z-30 bg-black/85 pb-[max(env(safe-area-inset-bottom),12px)] pt-3 text-white backdrop-blur" role="dialog" aria-label="ページ一覧">
      <div className="mb-2 flex items-center justify-between px-4">
        <p className={typeClass("subtitle")}>ページ一覧</p>
        <button type="button" onClick={onClose} className={cn(typeClass("caption"), "rounded px-2 py-1 text-white/70 hover:text-white")}>
          閉じる
        </button>
      </div>
      <div ref={ref} dir={rtl ? "rtl" : "ltr"} className="shelf-scroll flex gap-2 overflow-x-auto px-4 pb-1">
        {Array.from({ length: book.pageCount }, (_, i) => i + 1).map((p) => (
          <button
            key={p}
            type="button"
            data-page={p}
            onClick={() => onPick(p)}
            className={cn("w-16 shrink-0 rounded outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)]", p === current && "ring-2 ring-[var(--brand)]")}
            aria-label={`${p}ページへ`}
            aria-current={p === current ? "page" : undefined}
          >
            <img src={pageArt(book.coverSeed, p)} alt="" loading="lazy" className="aspect-[2/3] w-full rounded object-cover" />
            <span className={cn(typeClass("micro"), "mt-1 block text-center tabular-nums text-white/70")}>{p}</span>
          </button>
        ))}
      </div>
    </div>
  );
}

/** Continuous vertical scroll for webtoons: native scroll, pages lazy-load as they near the viewport. */
function WebtoonView({
  book,
  next,
  startPage,
  jump,
  onPage,
  onToggleChrome,
}: {
  book: Book;
  next: Book | null;
  startPage: number;
  jump: { page: number; n: number } | null;
  onPage: (p: number) => void;
  onToggleChrome: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  // Explicit jumps (page list) scroll; ordinary scrolling only reports.
  useEffect(() => {
    if (jump) ref.current?.querySelector(`[data-page="${jump.page}"]`)?.scrollIntoView({ block: "start" });
  }, [jump]);
  useEffect(() => {
    ref.current?.querySelector(`[data-page="${startPage}"]`)?.scrollIntoView({ block: "start" });
    const io = new IntersectionObserver(
      (es) => es.forEach((e) => e.isIntersecting && onPage(Number((e.target as HTMLElement).dataset.page))),
      { root: ref.current, rootMargin: "-50% 0px -50% 0px" },
    );
    ref.current?.querySelectorAll("[data-page]").forEach((el) => io.observe(el));
    return () => io.disconnect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return (
    <div ref={ref} className="absolute inset-0 overflow-y-auto" onClick={onToggleChrome}>
      <div className="mx-auto max-w-[800px]">
        {Array.from({ length: book.pageCount }, (_, i) => i + 1).map((p) => (
          <img
            key={p}
            data-page={p}
            src={pageArt(book.coverSeed, p, { webtoon: true })}
            alt={`${p}ページ`}
            loading={Math.abs(p - startPage) < 3 ? "eager" : "lazy"}
            className="block aspect-[800/1280] w-full"
          />
        ))}
        <div className="flex justify-center py-16">
          <EndCard book={book} next={next} seriesId={book.seriesId} />
        </div>
      </div>
    </div>
  );
}
