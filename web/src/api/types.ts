// API shapes shared by the mock layer and (later) the Go server.
// Keep these as the contract: screens import only from "@/api".

export type ReadingDirection = "ltr" | "rtl" | "vertical" | "webtoon";
export type SeriesStatus = "ongoing" | "ended" | "hiatus" | "abandoned";
export type MediaStatus = "ready" | "error" | "unsupported" | "outdated";
export type BookFormat = "cbz" | "cbr" | "cb7" | "pdf" | "epub";

export interface Library {
  id: string;
  name: string;
  root: string;
  kind: "manga" | "webtoon" | "comics" | "books";
  seriesCount: number;
  bookCount: number;
  sizeBytes: number;
  lastScan: string; // ISO
  scanInterval: "off" | "hourly" | "daily" | "weekly";
  watch: boolean;
  importComicInfo: boolean;
  importLocalArtwork: boolean;
  exclude: string[];
}

export interface Author {
  name: string;
  role: "writer" | "penciller" | "colorist" | "translator" | "editor" | "cover";
}

export interface Series {
  id: string;
  libraryId: string;
  title: string;
  titleSort: string;
  alternateTitles: string[];
  status: SeriesStatus;
  summary: string;
  publisher: string;
  readingDirection: ReadingDirection;
  ageRating: number | null;
  language: string;
  genres: string[];
  tags: string[];
  authors: Author[];
  totalBookCount: number | null;
  bookCount: number;
  booksRead: number;
  booksInProgress: number;
  coverSeed: number;
  createdAt: string;
  updatedAt: string;
  lockedFields: string[];
}

export interface ReadProgress {
  page: number; // 1-based
  completed: boolean;
  readAt: string;
}

export interface Book {
  id: string;
  seriesId: string;
  libraryId: string;
  title: string;
  number: number;
  numberSort: number;
  summary: string;
  releaseDate: string | null;
  isbn: string;
  authors: Author[];
  tags: string[];
  format: BookFormat;
  path: string;
  sizeBytes: number;
  pageCount: number;
  mediaStatus: MediaStatus;
  progress: ReadProgress | null;
  coverSeed: number;
  createdAt: string;
}

export interface Collection {
  id: string;
  name: string;
  ordered: boolean;
  seriesIds: string[];
}

export interface ReadList {
  id: string;
  name: string;
  summary: string;
  bookIds: string[];
}

export type Role = "admin" | "user";
export interface User {
  id: string;
  name: string;
  email: string;
  role: Role;
  canDownload: boolean;
  canStream: boolean;
  libraryIds: string[] | "all";
  lastActive: string;
}

export type TaskKind = "scan" | "analyze" | "thumbnail" | "hash" | "trash";
export interface Task {
  id: string;
  kind: TaskKind;
  label: string;
  libraryId: string | null;
  state: "running" | "queued" | "done" | "failed";
  progress: number; // 0..1
  startedAt: string | null;
}

export interface MediaIssue {
  bookId: string;
  kind: "error" | "missing" | "duplicate" | "unsupported";
  detail: string;
}

export interface ServerSettings {
  port: number;
  taskThreads: number;
  thumbnailSize: "small" | "medium" | "large" | "xlarge";
  thumbnailFormat: "webp" | "jpeg" | "avif";
  rememberMeDays: number;
  deleteEmptyCollections: boolean;
  opdsEnabled: boolean;
  koboSyncEnabled: boolean;
}

export type SeriesSort = "title" | "updated" | "created" | "releaseDate" | "bookCount";
export interface SeriesQuery {
  libraryId?: string;
  search?: string;
  status?: SeriesStatus[];
  genres?: string[];
  readState?: ("unread" | "inProgress" | "read")[];
  sort?: SeriesSort;
  desc?: boolean;
}
