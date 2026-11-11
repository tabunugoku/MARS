// Deterministic mock data. Every title, author and summary here is invented
// for the mockup; nothing refers to a real work.
import type {
  Book,
  Collection,
  Library,
  MediaIssue,
  ReadList,
  Series,
  ServerSettings,
  Task,
  User,
} from "@/api/types";

// The mock keeps a seed per cover; mock.ts turns it into thumbnailUrl.
export type MockSeries = Series & { coverSeed: number };
export type MockBook = Book & { coverSeed: number };

function rng(seed: number) {
  let s = seed >>> 0;
  return () => {
    s = (s * 1664525 + 1013904223) >>> 0;
    return s / 0x100000000;
  };
}
const r = rng(20261110);
const pick = <T,>(xs: readonly T[]) => xs[Math.floor(r() * xs.length)];
const daysAgo = (d: number) => new Date(Date.UTC(2026, 10, 10) - d * 86400000).toISOString();

export const libraries: Library[] = [
  { id: "lib-manga", name: "マンガ", root: "/comics/manga", kind: "manga", seriesCount: 0, bookCount: 0, sizeBytes: 0, lastScan: daysAgo(0.1), scanInterval: "daily", watch: true, importComicInfo: true, importLocalArtwork: true, exclude: ["@eaDir", "#recycle"] },
  { id: "lib-webtoon", name: "Webtoon", root: "/comics/webtoon", kind: "webtoon", seriesCount: 0, bookCount: 0, sizeBytes: 0, lastScan: daysAgo(1.2), scanInterval: "daily", watch: true, importComicInfo: true, importLocalArtwork: false, exclude: [] },
  { id: "lib-comics", name: "海外コミック", root: "/comics/western", kind: "comics", seriesCount: 0, bookCount: 0, sizeBytes: 0, lastScan: daysAgo(6), scanInterval: "weekly", watch: false, importComicInfo: true, importLocalArtwork: true, exclude: [] },
];

const mangaTitles = [
  "夜更けの測量士", "潮見坂ブックス", "白線の向こう側", "鉄と紙飛行機", "まどろみ天文台",
  "千年喫茶ほたる", "灯台守のレシピ", "雨音サーキット", "ひだまり探偵社", "砂時計の騎士",
  "月面ベーカリー", "青のアトリエ", "とおりゃんせ電車", "ことのは図書館", "星屑ガレージ",
  "風見鶏の約束", "遠雷マーチ", "硝子の庭師", "うたかた商店街", "銀河郵便局",
];
const webtoonTitles = ["屋上のサイレン", "ネオン金魚", "百階のエレベーター", "放課後リブート", "深海ラジオ"];
const comicsTitles = ["The Last Lamplighter", "Copper Orchard", "Night Shift Atlas", "Paper Comets", "Harbor Lights"];

const authorsPool = ["朝霧 ルカ", "久遠 しずく", "早瀬 透", "冬野 灯", "藍沢 ミナト", "葛城 響", "柚木 遥", "白石 ノア", "Iris Calder", "Owen Pike"];
const publishers = ["月白社", "ほしぞら書房", "潮出版工房", "Lantern Press", "青嵐コミックス"];
const genres = ["SF", "ファンタジー", "日常", "ミステリー", "グルメ", "青春", "冒険", "ホラー", "恋愛", "お仕事"];
const tagsPool = ["完結済み", "アニメ化", "短編集", "フルカラー", "百合", "バトル", "癒やし", "群像劇", "タイムリープ", "職人"];
const summaries = [
  "地図にない町を測るため、見習い測量士は夜ごと屋根の上を歩く。古い測量図に描かれた一本の線が、彼女を町の秘密へ導いていく。",
  "閉店間際の古書店に、毎週同じ時刻に現れる客がいる。彼が探しているのは、まだ書かれていない本だという。",
  "海沿いの小さな町で、灯台守の祖母が遺したレシピ帳を手がかりに、少年は料理と家族の記憶をたどる。",
  "雨の日にだけ開くサーキットで、無口な整備士と負けず嫌いのドライバーが出会う。",
  "宇宙港の片隅にある郵便局は、届け先のない手紙ばかりを集めている。",
];

export const series: MockSeries[] = [];
export const books: MockBook[] = [];

function addSeries(libraryId: string, title: string, idx: number, webtoon = false, western = false) {
  const id = `s-${libraryId.slice(4)}-${idx}`;
  const bookCount = webtoon ? 20 + Math.floor(r() * 60) : 1 + Math.floor(r() * 18);
  const status = pick(["ongoing", "ongoing", "ended", "hiatus"] as const);
  const authors = [
    { name: pick(authorsPool), role: "writer" as const },
    ...(r() > 0.6 ? [{ name: pick(authorsPool), role: "penciller" as const }] : []),
  ];
  const updated = r() * 40;
  const s: MockSeries = {
    id,
    libraryId,
    title,
    titleSort: title,
    alternateTitles: r() > 0.7 ? [`${title}（新装版）`] : [],
    status,
    summary: pick(summaries),
    publisher: pick(publishers),
    readingDirection: webtoon ? "webtoon" : western ? "ltr" : "rtl",
    ageRating: r() > 0.85 ? 15 : null,
    language: western ? "en" : "ja",
    genres: Array.from(new Set([pick(genres), pick(genres)])),
    tags: Array.from(new Set([pick(tagsPool), pick(tagsPool)])),
    authors,
    totalBookCount: status === "ended" ? bookCount : null,
    bookCount,
    booksRead: 0,
    booksInProgress: 0,
    coverSeed: Math.floor(r() * 10000),
    thumbnailUrl: "",
    createdAt: daysAgo(updated + r() * 300),
    updatedAt: daysAgo(updated),
    lockedFields: r() > 0.8 ? ["summary"] : [],
  };
  // Reading state: some finished, some mid-way, most untouched.
  const readUpTo = r() < 0.35 ? Math.floor(r() * bookCount) : 0;
  for (let n = 1; n <= bookCount; n++) {
    const pageCount = webtoon ? 40 + Math.floor(r() * 30) : 160 + Math.floor(r() * 60);
    let progress: Book["progress"] = null;
    if (n <= readUpTo) progress = { page: pageCount, completed: true, readAt: daysAgo(10 + (readUpTo - n)) };
    else if (n === readUpTo + 1 && readUpTo > 0 && r() > 0.3)
      progress = { page: 1 + Math.floor(r() * (pageCount - 2)), completed: false, readAt: daysAgo(r() * 6) };
    const b: MockBook = {
      id: `${id}-b${n}`,
      seriesId: id,
      libraryId,
      title: webtoon ? `第${n}話` : western ? `Issue #${n}` : `第${n}巻`,
      number: n,
      numberSort: n,
      summary: n === 1 ? s.summary : "",
      releaseDate: daysAgo(updated + (bookCount - n) * 90).slice(0, 10),
      isbn: western || webtoon ? "" : `978-4-${String(100000 + Math.floor(r() * 899999))}-${n % 10}`,
      authors,
      tags: [],
      format: western ? "cbr" : r() > 0.9 ? "pdf" : "cbz",
      path: `${libraries.find((l) => l.id === libraryId)!.root}/${title}/${title} ${String(n).padStart(2, "0")}.cbz`,
      sizeBytes: Math.floor((webtoon ? 18 : 60 + r() * 80) * 1024 * 1024),
      pageCount,
      mediaStatus: r() > 0.985 ? "error" : "ready",
      progress,
      coverSeed: s.coverSeed + n * 7,
      thumbnailUrl: "",
      lockedFields: [],
      createdAt: daysAgo(updated + (bookCount - n) * 3),
    };
    books.push(b);
    if (progress?.completed) s.booksRead++;
    else if (progress) s.booksInProgress++;
  }
  series.push(s);
}

mangaTitles.forEach((t, i) => addSeries("lib-manga", t, i));
webtoonTitles.forEach((t, i) => addSeries("lib-webtoon", t, i, true));
comicsTitles.forEach((t, i) => addSeries("lib-comics", t, i, false, true));

for (const l of libraries) {
  const ss = series.filter((s) => s.libraryId === l.id);
  const bs = books.filter((b) => b.libraryId === l.id);
  l.seriesCount = ss.length;
  l.bookCount = bs.length;
  l.sizeBytes = bs.reduce((a, b) => a + b.sizeBytes, 0);
}

export const collections: Collection[] = [
  { id: "c-1", name: "夏に読みたい", ordered: false, seriesIds: series.slice(0, 6).map((s) => s.id) },
  { id: "c-2", name: "食べ物が出てくる話", ordered: true, seriesIds: [series[5], series[6], series[10]].map((s) => s.id) },
  { id: "c-3", name: "完結済みの名作", ordered: false, seriesIds: series.filter((s) => s.status === "ended").slice(0, 8).map((s) => s.id) },
];

export const readLists: ReadList[] = [
  { id: "rl-1", name: "銀河郵便局 時系列順", summary: "外伝を本編の時系列に並べ替えたリスト", bookIds: books.filter((b) => b.seriesId === series[19].id).slice(0, 8).map((b) => b.id) },
  { id: "rl-2", name: "週末の一気読み", summary: "1巻だけ集めた試し読み用", bookIds: series.slice(0, 10).map((s) => `${s.id}-b1`) },
];

export const users: User[] = [
  { id: "u-1", name: "admin", email: "admin@example.test", role: "admin", canDownload: true, canStream: true, allLibraries: true, libraryIds: [], lastActive: daysAgo(0) },
  { id: "u-2", name: "ゆう", email: "yu@example.test", role: "user", canDownload: true, canStream: true, allLibraries: false, libraryIds: ["lib-manga", "lib-webtoon"], lastActive: daysAgo(1) },
  { id: "u-3", name: "kids", email: "kids@example.test", role: "user", canDownload: false, canStream: true, allLibraries: false, libraryIds: ["lib-manga"], lastActive: daysAgo(8) },
];

export const tasks: Task[] = [
  { id: "t-1", kind: "scan", label: "マンガ をスキャン", libraryId: "lib-manga", state: "running", progress: 0.62, startedAt: daysAgo(0.002), error: "" },
  { id: "t-2", kind: "thumbnail", label: "サムネイルを生成（214件）", libraryId: "lib-manga", state: "queued", progress: 0, startedAt: null, error: "" },
  { id: "t-3", kind: "analyze", label: "メディアを解析（38件）", libraryId: "lib-webtoon", state: "queued", progress: 0, startedAt: null, error: "" },
  { id: "t-4", kind: "scan", label: "海外コミック をスキャン", libraryId: "lib-comics", state: "done", progress: 1, startedAt: daysAgo(0.3), error: "" },
  { id: "t-5", kind: "hash", label: "ファイルハッシュを計算", libraryId: null, state: "failed", progress: 0.4, startedAt: daysAgo(1), error: "" },
];

export const mediaIssues: MediaIssue[] = [
  ...books.filter((b) => b.mediaStatus === "error").map((b) => ({ bookId: b.id, kind: "error" as const, detail: "アーカイブを開けません（CRC エラー）" })),
  { bookId: books[3].id, kind: "duplicate", detail: "同じハッシュのファイルが別のフォルダにあります" },
  { bookId: books[12].id, kind: "missing", detail: "スキャン時にファイルが見つかりませんでした（ゴミ箱に移動済み）" },
];

export const settings: ServerSettings = {
  port: 25600,
  taskThreads: 2,
  thumbnailSize: "medium",
  thumbnailFormat: "jpeg",
  rememberMeDays: 30,
  deleteEmptyCollections: true,
  opdsEnabled: true,
  koboSyncEnabled: false,
  loginBackdrop: "compass",
};

export const currentUser = users[0];
