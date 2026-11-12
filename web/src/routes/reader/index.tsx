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
import { api, useApi, effectivePrefs, errorMessage, getBookPrefs, getGlobalPrefs, getSeriesPrefs, loadReaderPrefs, scopeOf, setBookPrefs, setSeriesPrefs, subscribeReaderPrefs, type Book, type Page, type ReaderPrefs, type ReaderScope, type ReadingDirection, type SeriesReaderPrefs } from "@/api";
import { toast } from "@yusufceng1z/ding-react";
import { typeClass } from "@/lib/type-scale";
import { fontWeights } from "@/lib/font-weight";
import { cn } from "@/lib/utils";
import { BrandAction } from "@/components/media/brand-action";
import { usePageSwipe } from "./use-page-swipe";
import { ReaderSettings } from "./settings";

// ---------------------------------------------------------------------------
// Reader. Priorities, in order: a page turn never waits (neighbours are
// mounted and decoded ahead), the gesture follows the finger, and chrome
// stays out of the way until asked for (tap the centre, or press M).
// ---------------------------------------------------------------------------

/** One or two page numbers shown together. half: only that half of a
 *  landscape page, in reading order ("first" is the right half in RTL). */
type View = { pages: number[]; half?: "first" | "second" };

/** A page scanned as a two-page spread (wider than tall) always stands alone. */
const isWide = (pg: Page | undefined) => !!pg && pg.width > pg.height;

/** Group pages into views. Spread mode pairs pages; "cover" keeps page 1
 *  alone; a page that is already a spread is never paired. With split, such
 *  a page becomes two views, one per half. */
function buildViews(pages: Page[], spread: ReaderPrefs["spread"], split = false): View[] {
  const count = pages.length;
  const views: View[] = [];
  let p = 1;
  const single = (n: number) => {
    if (split && isWide(pages[n - 1])) views.push({ pages: [n], half: "first" }, { pages: [n], half: "second" });
    else views.push({ pages: [n] });
  };
  if (spread === "cover" && count > 0) single(p++);
  while (p <= count) {
    if (spread === "off" || p === count || isWide(pages[p - 1]) || isWide(pages[p])) single(p++);
    else {
      views.push({ pages: [p, p + 1] });
      p += 2;
    }
  }
  return views;
}

/** The view showing page n; its second half when second is set and the page
 *  is split. -1 when no view shows it. */
function findView(views: View[], n: number, second = false): number {
  const i = views.findIndex((v) => v.pages.includes(n));
  return second && views[i + 1]?.half === "second" && views[i + 1].pages[0] === n ? i + 1 : i;
}

/** The server keeps a page number only. Which half of a split page the reader
 *  stopped at is kept in this browser: the page whose second half was showing. */
const halfKey = (bookId: string) => `cominavi.half.${bookId}`;
function readHalf(bookId: string): number | null {
  try {
    return Number(localStorage.getItem(halfKey(bookId))) || null;
  } catch {
    return null;
  }
}
function writeHalf(bookId: string, page: number | null) {
  try {
    if (page) localStorage.setItem(halfKey(bookId), String(page));
    else localStorage.removeItem(halfKey(bookId));
  } catch {
    // storage blocked: the half is not remembered
  }
}

const progressKey = (p: { page: number; completed: boolean }) => `${p.page}|${p.completed}`;

/** Saves the reading position: a second after the last turn, and right away
 *  when the page is hidden or the reader closes.
 *
 *  Nothing is recorded until the reader turns a page: the caller calls the
 *  returned navigated() from its page turns, jumps and user scrolling, so
 *  layout settling (webtoon restore, a spread regroup after a settings
 *  change) never counts as reading.
 *
 *  Every write goes out at once, keepalive (it survives unload), named with
 *  this reader's writer id and an increasing seq; the server ignores a write
 *  older than one it already stored from this writer, so an earlier page
 *  still in flight can never land after a later one. */
function useProgressSaver(
  bookId: string,
  page: number,
  completed: boolean,
  position: string,
  stored: Book["progress"],
  disabled: boolean,
) {
  const latest = useRef({ page, completed, position });
  const moved = useRef(false);
  const sent = useRef(stored ? progressKey(stored) : "");
  const writer = useRef<string | null>(null);
  const seq = useRef(0);
  const flush = useCallback(
    (leaving: boolean) => {
      const v = latest.current;
      const key = progressKey(v);
      if (disabled || !moved.current || key === sent.current) return;
      sent.current = key;
      // One turn, one write: a later regroup (a settings change) without a
      // new turn does not write again.
      moved.current = false;
      // getRandomValues, not randomUUID: the latter needs HTTPS, and the server is often plain HTTP on a LAN.
      writer.current ??= Array.from(crypto.getRandomValues(new Uint8Array(12)), (b) => b.toString(16).padStart(2, "0")).join("");
      api.setProgress(bookId, v.page, v.completed, { keepalive: true, writer: writer.current, seq: ++seq.current }).catch((err: unknown) => {
        // Let the next flush retry.
        if (sent.current === key) sent.current = "";
        moved.current = true;
        if (!leaving) toast.error(`読書の位置を保存できませんでした：${errorMessage(err)}`);
      });
    },
    [bookId, disabled],
  );
  useEffect(() => {
    latest.current = { page, completed, position };
    const t = setTimeout(() => flush(false), 1000);
    return () => clearTimeout(t);
  }, [page, completed, position, flush]);
  useEffect(() => {
    const onHide = () => flush(true);
    window.addEventListener("pagehide", onHide);
    return () => {
      window.removeEventListener("pagehide", onHide);
      flush(true);
    };
  }, [flush]);
  return useCallback(() => {
    moved.current = true;
  }, []);
}

/** Which touch gestures the browser keeps. A page larger than the screen
 *  ("original", "height") pans natively in both directions, so a drag moves
 *  the image and pages turn with the tap zones and keys; "width" keeps the
 *  vertical scroll and leaves horizontal swipes to the page turn. */
function panTouch(fit: ReaderPrefs["fit"], axis: "x" | "y"): string {
  if (fit === "original" || fit === "height") return "pan-x pan-y";
  if (fit === "width" && axis === "x") return "pan-y";
  return "none";
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

  const { data, loading, error } = useApi(async () => {
    const book = await api.book(bookId);
    if (!book) return null;
    const [series, adj, pages] = await Promise.all([
      api.seriesById(book.seriesId),
      api.adjacentBooks(book.id),
      api.pages(book.id),
      loadReaderPrefs(),
    ]);
    return { book, series, adj, pages };
  }, [bookId]);

  if (loading && !data) return <div className="fixed inset-0 bg-black" aria-busy />;
  if (!data || data.pages.length === 0) {
    return (
      <div className="fixed inset-0 flex flex-col items-center justify-center gap-3 bg-black text-white">
        <p className={typeClass("title")}>
          {error ?? (data ? "この本はまだ読めません（解析が済んでいないか、ページがありません）" : "この本は見つかりませんでした")}
        </p>
        <Link to="/" className={cn(typeClass("body"), "text-white/70 underline-offset-4 hover:underline")}>ホームへ戻る</Link>
      </div>
    );
  }
  return (
    <ReaderBody
      key={data.book.id}
      book={data.book}
      pages={data.pages}
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
  pages,
  seriesTitle,
  defaultDirection,
  next,
  startPage,
  incognito,
  onExit,
}: {
  book: Book;
  pages: Page[];
  seriesTitle: string;
  defaultDirection: ReadingDirection;
  next: Book | null;
  startPage: number;
  incognito: boolean;
  onExit: () => void;
}) {
  // The global reader settings are the master; the series and this volume
  // can each keep their own (volume > series > global), chosen in 表示設定.
  const [globalPrefs] = useState(getGlobalPrefs);
  const [seriesPrefs, setSeriesState] = useState<SeriesReaderPrefs>(() => getSeriesPrefs(book.seriesId));
  const [bookPrefs, setBookState] = useState<SeriesReaderPrefs>(() => getBookPrefs(book.id));
  const scope = scopeOf(seriesPrefs, bookPrefs);
  const prefs = useMemo(() => effectivePrefs(seriesPrefs, bookPrefs, globalPrefs, defaultDirection), [seriesPrefs, bookPrefs, globalPrefs, defaultDirection]);
  // The screen follows the shared cache: a change shows at once, and a
  // refused save rolls back here too, even if another reader made it.
  useEffect(
    () =>
      subscribeReaderPrefs(() => {
        setSeriesState(getSeriesPrefs(book.seriesId));
        setBookState(getBookPrefs(book.id));
      }),
    [book.seriesId, book.id],
  );
  const saveError = (err: unknown) => toast.error(`表示設定を保存できませんでした：${errorMessage(err)}`);
  const saveSeries = (p: SeriesReaderPrefs) => void setSeriesPrefs(book.seriesId, p).catch(saveError);
  const saveBook = (p: SeriesReaderPrefs) => void setBookPrefs(book.id, p).catch(saveError);
  const changeScope = (s: ReaderScope) => {
    if (s === scope) return;
    if (s === "book") return saveBook({ mode: "custom", prefs });
    saveBook({ mode: "global" });
    // 全体設定 also clears the series' shared set; シリーズ共通 starts from what is on screen.
    saveSeries(s === "series" ? { mode: "custom", prefs } : { mode: "global" });
  };
  const changePrefs = (p: ReaderPrefs) => (scope === "book" ? saveBook : saveSeries)({ mode: "custom", prefs: p });
  const [chrome, setChrome] = useState(true);
  const [panel, setPanel] = useState<"none" | "settings" | "pages">("none");
  const [fullscreen, setFullscreen] = useState(false);

  const webtoon = prefs.direction === "webtoon";
  const pageCount = pages.length;
  // A landscape page in halves on a portrait screen, when this profile asks.
  const portrait = useMatchMedia("(orientation: portrait)");
  const split = !webtoon && !!globalPrefs.splitWide && portrait;
  const views = useMemo(() => buildViews(pages, webtoon ? "off" : prefs.spread, split), [pages, prefs.spread, webtoon, split]);
  // index views.length = the end card
  const [vi, setVi] = useState(() => Math.max(0, findView(views, startPage, !incognito && readHalf(book.id) === startPage)));
  const viewIndex = Math.min(vi, views.length);
  const atEnd = viewIndex === views.length;
  const currentPage = atEnd ? pageCount : views[viewIndex].pages[0];
  const half = atEnd ? undefined : views[viewIndex].half;
  const url = (p: number) => pages[p - 1]?.url ?? "";

  // The page the reader is "on", independent of grouping (Infinity = end card).
  // When the spread layout changes, re-find it in the new views. This effect
  // is declared before the one that records the anchor, so it reads the page
  // from the previous layout, not the one recomputed with the stale index.
  const anchor = useRef<number>(currentPage);
  // The page whose second half is showing, kept while a landscape screen shows it whole.
  const secondOf = useRef<number | null>(half === "second" ? currentPage : null);
  const [webtoonPage, setWebtoonPage] = useState(currentPage);
  const [webtoonEnd, setWebtoonEnd] = useState(false);
  useEffect(() => {
    const a = anchor.current;
    setVi(a === Infinity ? views.length : Math.max(0, findView(views, a, secondOf.current === a)));
    setWebtoonPage(a === Infinity ? pageCount : a);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [views]);
  useEffect(() => {
    if (webtoon) return;
    anchor.current = atEnd ? Infinity : currentPage;
    if (half) secondOf.current = half === "second" ? currentPage : null;
    else if (secondOf.current !== currentPage) secondOf.current = null;
    // Remember it for the next visit too.
    if (!incognito) writeHalf(book.id, atEnd ? null : secondOf.current);
  }, [atEnd, currentPage, half, webtoon, incognito, book.id]);
  const [jump, setJump] = useState<{ page: number; n: number } | null>(null);

  // The end card counts as the last page, finished (in webtoon too, where it
  // may come into view without the page changing). Incognito reads leave no trace.
  const shownPage = webtoon ? (webtoonEnd ? pageCount : webtoonPage) : currentPage;
  const lastShown = webtoon ? shownPage : atEnd ? pageCount : Math.max(...views[viewIndex].pages);
  const finished = webtoon ? webtoonEnd : atEnd;
  // The first half of a split last page leaves the second half unread.
  const navigated = useProgressSaver(book.id, shownPage, lastShown >= pageCount && half !== "first", finished ? "end" : String(shownPage), book.progress, incognito);

  const axis = prefs.direction === "vertical" ? "y" : "x";
  const forward = prefs.direction === "rtl" ? -1 : 1;

  const stageRef = useRef<HTMLDivElement>(null);
  const commit = useCallback(
    (d: 1 | -1) => {
      setVi((i) => Math.max(0, Math.min(views.length, i + d)));
      setChrome(false);
      navigated();
    },
    [views.length, navigated],
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
      else if (k === "Home") return navigated(), setVi(0);
      else if (k === "End") return navigated(), setVi(views.length - 1);
      if (d) {
        e.preventDefault();
        swipe.turn(d);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [axis, forward, views.length, panel, onExit, swipe, webtoon, navigated]);

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
    } else {
      secondOf.current = null;
      setVi(Math.max(0, findView(views, p)));
    }
    setPanel("none");
    navigated();
  };

  // Neighbour views are mounted beside the current one (prev/next) so a turn
  // shows decoded pixels; two further views are preloaded invisibly.
  const slot = (i: number) =>
    i < 0 ? null : i === views.length ? <EndCard key="end" book={book} next={next} seriesId={book.seriesId} incognito={incognito} /> : i > views.length ? null : (
      <PageView key={i} view={views[i]} url={url} rtl={forward === -1} fit={prefs.fit} pageInfo={(p) => pages[p - 1]} />
    );

  const tone = bgClass[prefs.bg];

  return (
    <div className={cn("fixed inset-0 select-none overflow-clip", tone)} data-reader>
      {webtoon ? (
        <WebtoonView
          book={book}
          pages={pages}
          next={next}
          incognito={incognito}
          startPage={webtoonPage}
          jump={jump}
          active={panel === "none"}
          onPage={(p, byUser) => {
            anchor.current = p;
            setWebtoonPage(p);
            if (byUser) navigated();
          }}
          onEnd={(visible, byUser) => {
            setWebtoonEnd(visible);
            if (byUser) navigated();
          }}
          onToggleChrome={() => setChrome((c) => !c)}
        />
      ) : (
        <div
          ref={stageRef}
          className="absolute inset-0 touch-none overflow-clip"
          style={{ touchAction: panTouch(prefs.fit, axis) }}
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
          <Preload urls={[viewIndex + 2, viewIndex + 3].flatMap((i) => views[i]?.pages ?? []).map(url)} />
        </div>
      )}

      {/* Chrome: top and bottom bars. They slide out of the way, never block the page when hidden. */}
      <motion.header
        initial={false}
        animate={{ y: chrome ? 0 : "-100%", opacity: chrome ? 1 : 0 }}
        transition={{ type: "spring", duration: 0.24, bounce: 0 }}
        className="absolute inset-x-0 top-0 z-20 flex items-center gap-2 border-b border-white/8 bg-reader/90 pb-2 pl-[max(env(safe-area-inset-left),8px)] pr-[max(env(safe-area-inset-right),8px)] pt-[max(env(safe-area-inset-top),8px)] text-white"
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
          className="absolute inset-x-0 bottom-0 z-20 border-t border-white/8 bg-reader/90 pb-[max(env(safe-area-inset-bottom),12px)] pl-[max(env(safe-area-inset-left),16px)] pr-[max(env(safe-area-inset-right),16px)] pt-3 text-white"
          style={{ pointerEvents: chrome ? "auto" : "none" }}
          inert={!chrome}
        >
          <div className="mx-auto flex max-w-3xl items-center gap-3">
            <span className={cn(typeClass("caption"), "w-20 font-mono tabular-nums text-white/80")}>
              P.{currentPage} / {pageCount}
            </span>
            <DotRail value={currentPage} max={pageCount} rtl={forward === -1} onChange={goToPage} />
            <span className={cn(typeClass("caption"), "w-16 text-right text-white/60")}>
              {prefs.direction === "rtl" ? "右から左" : prefs.direction === "ltr" ? "左から右" : "縦"}
            </span>
          </div>
        </motion.footer>
      )}

      {panel === "pages" && (
        <PageStrip
          pages={pages}
          current={shownPage}
          rtl={forward === -1}
          onPick={goToPage}
          onClose={() => setPanel("none")}
        />
      )}
      <ReaderSettings open={panel === "settings"} scope={scope} prefs={prefs} onScope={changeScope} onChange={changePrefs} onClose={() => setPanel("none")} />
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
        "flex size-11 items-center justify-center rounded-full transition-colors duration-80 hover:bg-white/10 active:bg-white/20",
        "outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)]",
        pressed && "bg-white/15",
      )}
    >
      {children}
    </button>
  );
}

function PageImg({ src, page, fit, className }: { src: string; page: number; fit: ReaderPrefs["fit"]; className?: string }) {
  const fitCls = {
    // Fills the stage both ways and keeps the aspect, so low-resolution scans grow too.
    screen: "size-full object-contain",
    height: "h-full w-auto max-w-none",
    width: "w-full h-auto max-h-none",
    original: "max-w-none max-h-none",
  }[fit];
  return <img src={src} alt={`${page}ページ`} draggable={false} decoding="sync" className={cn(fitCls, className)} />;
}

/** Whether a media query matches now; follows changes (rotation, resize). */
function useMatchMedia(query: string): boolean {
  const [on, setOn] = useState(() => typeof matchMedia !== "undefined" && matchMedia(query).matches);
  useEffect(() => {
    const m = matchMedia(query);
    const update = () => setOn(m.matches);
    update();
    m.addEventListener("change", update);
    return () => m.removeEventListener("change", update);
  }, [query]);
  return on;
}

/** One half of a landscape page, as large as the stage allows: a box with
 *  the half's aspect, sized with container units, cropping the image. */
function HalfPage({ src, page, width, height, side }: { src: string; page: number; width: number; height: number; side: "left" | "right" }) {
  const ar = width / 2 / height;
  return (
    <div className="flex size-full items-center justify-center" style={{ containerType: "size" }}>
      <div className="overflow-hidden" style={{ width: `min(100cqw, ${100 * ar}cqh)`, height: `min(100cqh, ${100 / ar}cqw)` }}>
        <img
          src={src}
          alt={`${page}ページ（${side === "right" ? "右" : "左"}半分）`}
          draggable={false}
          decoding="sync"
          className={cn("size-full object-cover", side === "right" ? "object-right" : "object-left")}
        />
      </div>
    </div>
  );
}

function PageView({ view, url, rtl, fit, pageInfo }: { view: View; url: (page: number) => string; rtl: boolean; fit: ReaderPrefs["fit"]; pageInfo: (page: number) => Page | undefined }) {
  if (view.half) {
    const p = view.pages[0];
    const info = pageInfo(p);
    // In RTL the right half is read first.
    const side = (view.half === "first") === rtl ? "right" : "left";
    return <HalfPage src={url(p)} page={p} width={info?.width ?? 2} height={info?.height ?? 1} side={side} />;
  }
  const pages = rtl ? [...view.pages].reverse() : view.pages;
  const scroll = fit !== "screen";
  if (pages.length === 1) {
    return (
      <div className={cn("flex size-full justify-center", scroll ? "items-start overflow-auto" : "items-center")}>
        <PageImg src={url(pages[0])} page={pages[0]} fit={fit} />
      </div>
    );
  }
  // A spread: two halves meet at the gutter, each fitted to half the stage.
  return (
    <div className={cn("flex size-full justify-center", scroll ? "items-start overflow-auto" : "items-center")}>
      {pages.map((p) => (
        <div key={p} className={cn("flex h-full w-1/2 items-center", p === pages[0] ? "justify-end" : "justify-start")}>
          <PageImg
            src={url(p)}
            page={p}
            fit={fit === "width" ? "width" : "screen"}
            className={fit === "width" ? undefined : p === pages[0] ? "object-right" : "object-left"}
          />
        </div>
      ))}
    </div>
  );
}

function Preload({ urls }: { urls: string[] }) {
  return (
    <div className="pointer-events-none absolute size-px overflow-hidden opacity-0" aria-hidden>
      {urls.map((u) => (
        <img key={u} src={u} alt="" decoding="async" />
      ))}
    </div>
  );
}

function EndCard({ book, next, seriesId, incognito }: { book: Book; next: Book | null; seriesId: string; incognito: boolean }) {
  return (
    <div className="flex max-w-sm flex-col items-center gap-4 px-6 text-center" onPointerDown={(e) => e.stopPropagation()}>
      <p className={typeClass("title")} style={{ fontVariationSettings: fontWeights.semibold }}>
        {book.title}を読み終わりました
      </p>
      {next ? (
        <>
          <img src={next.thumbnailUrl} alt="" className="w-32 rounded-md shadow-lg" />
          <BrandAction to={`/read/${next.id}?page=1${incognito ? "&incognito=1" : ""}`}>次の巻を読む（{next.title}）</BrandAction>
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

function PageStrip({ pages, current, rtl, onPick, onClose }: { pages: Page[]; current: number; rtl: boolean; onPick: (p: number) => void; onClose: () => void }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    ref.current?.querySelector<HTMLElement>(`[data-page="${current}"]`)?.scrollIntoView({ inline: "center", block: "nearest" });
  }, [current]);
  return (
    <div className="absolute inset-x-0 bottom-0 z-30 border-t border-white/8 bg-reader pb-[max(env(safe-area-inset-bottom),12px)] pt-3 text-white" role="dialog" aria-label="ページ一覧">
      <div className="mb-2 flex items-center justify-between pl-[max(env(safe-area-inset-left),16px)] pr-[max(env(safe-area-inset-right),4px)]">
        <p className={typeClass("subtitle")}>ページ一覧</p>
        <button type="button" onClick={onClose} className={cn(typeClass("caption"), "-my-2 inline-flex h-11 items-center rounded px-3 text-white/70 hover:text-white")}>
          <span className="[text-box:trim-both_cap_alphabetic]">閉じる</span>
        </button>
      </div>
      <div ref={ref} dir={rtl ? "rtl" : "ltr"} className="shelf-scroll flex gap-2 overflow-x-auto px-4 pb-1">
        {pages.map(({ number: p, thumbnailUrl }) => (
          <button
            key={p}
            type="button"
            data-page={p}
            onClick={() => onPick(p)}
            style={{ contentVisibility: "auto", containIntrinsicSize: "64px 120px" }}
            className={cn("w-16 shrink-0 rounded outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)]", p === current && "ring-2 ring-[var(--brand)]")}
            aria-label={`${p}ページへ`}
            aria-current={p === current ? "page" : undefined}
          >
            <img src={thumbnailUrl} alt="" loading="lazy" className="aspect-[2/3] w-full rounded object-cover" />
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
  pages,
  next,
  incognito,
  startPage,
  jump,
  active,
  onPage,
  onEnd,
  onToggleChrome,
}: {
  book: Book;
  pages: Page[];
  next: Book | null;
  incognito: boolean;
  startPage: number;
  jump: { page: number; n: number } | null;
  /** No panel is open: keys go to the strip. */
  active: boolean;
  /** byUser: the user has scrolled the strip, so the change is reading, not
   *  the restore scroll or images settling. */
  onPage: (p: number, byUser: boolean) => void;
  onEnd: (visible: boolean, byUser: boolean) => void;
  onToggleChrome: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const byUser = useRef(false);
  const activeRef = useRef(active);
  useEffect(() => {
    activeRef.current = active;
  }, [active]);
  // Scrolling input on the strip itself; keys only while no panel is open
  // (the strip is not focusable, so they arrive on the page).
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const mark = () => {
      byUser.current = true;
    };
    const onKey = () => {
      if (activeRef.current) mark();
    };
    const kinds = ["wheel", "touchstart", "pointerdown"] as const;
    kinds.forEach((k) => el.addEventListener(k, mark, { passive: true }));
    window.addEventListener("keydown", onKey);
    return () => {
      kinds.forEach((k) => el.removeEventListener(k, mark));
      window.removeEventListener("keydown", onKey);
    };
  }, []);
  // Explicit jumps (page list) scroll; ordinary scrolling only reports.
  useEffect(() => {
    if (jump) ref.current?.querySelector(`[data-page="${jump.page}"]`)?.scrollIntoView({ block: "start" });
  }, [jump]);
  useEffect(() => {
    ref.current?.querySelector(`[data-page="${startPage}"]`)?.scrollIntoView({ block: "start" });
    const io = new IntersectionObserver(
      (es) => es.forEach((e) => e.isIntersecting && onPage(Number((e.target as HTMLElement).dataset.page), byUser.current)),
      { root: ref.current, rootMargin: "-50% 0px -50% 0px" },
    );
    ref.current?.querySelectorAll("[data-page]").forEach((el) => io.observe(el));
    // The end card in view counts as finishing the volume (it may not change the page).
    const endIo = new IntersectionObserver(([e]) => onEnd(e.isIntersecting, byUser.current), { root: ref.current, threshold: 0.5 });
    const endEl = ref.current?.querySelector("[data-end]");
    if (endEl) endIo.observe(endEl);
    return () => {
      io.disconnect();
      endIo.disconnect();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return (
    <div ref={ref} className="absolute inset-0 overflow-y-auto" onClick={onToggleChrome}>
      <div className="mx-auto max-w-[800px]">
        {pages.map(({ number: p, url, width, height }) => (
          <img
            key={p}
            data-page={p}
            src={url}
            alt={`${p}ページ`}
            loading={Math.abs(p - startPage) < 3 ? "eager" : "lazy"}
            className="block w-full [content-visibility:auto]"
            style={{ aspectRatio: `${width || 800} / ${height || 1280}` }}
          />
        ))}
        <div data-end className="flex justify-center py-16">
          <EndCard book={book} next={next} seriesId={book.seriesId} incognito={incognito} />
        </div>
      </div>
    </div>
  );
}

/** The page slider drawn as a route of dots: violet up to the current page,
 *  a cyan dot where you are, faint dots ahead. Runs in reading direction (RTL
 *  books fill from the right). The real control is a native range input laid
 *  over it at opacity 0, so keyboard, touch and screen readers keep working. */
function DotRail({ value, max, rtl, onChange }: { value: number; max: number; rtl: boolean; onChange: (page: number) => void }) {
  const p = max > 1 ? (value - 1) / (max - 1) : 0;
  const side = rtl ? "right" : "left";
  const dots = "[background-image:radial-gradient(currentColor_1.5px,transparent_2px)] [background-size:9px_9px] [background-position:0_50%]";
  return (
    <div className="relative h-11 flex-1">
      <div className={cn("absolute inset-x-0 top-1/2 h-3 -translate-y-1/2 text-white/20", dots)} />
      <div
        className={cn("absolute inset-x-0 top-1/2 h-3 -translate-y-1/2 text-brand", dots)}
        style={{ clipPath: rtl ? `inset(0 0 0 ${(1 - p) * 100}%)` : `inset(0 ${(1 - p) * 100}% 0 0)` }}
      />
      <span
        aria-hidden
        className="absolute top-1/2 size-3.5 -translate-y-1/2 rounded-full bg-here ring-4 ring-here/25"
        style={{ [side]: `calc(${p * 100}% - 7px)` }}
      />
      <input
        type="range"
        min={1}
        max={max}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        aria-label="ページ"
        dir={rtl ? "rtl" : "ltr"}
        className="absolute inset-0 h-full w-full cursor-pointer opacity-0"
      />
    </div>
  );
}
