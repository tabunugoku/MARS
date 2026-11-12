// Every call the screens make, against the Go server. The mock in mock.ts
// implements the same Api interface for `VITE_API=mock`.
import { query, request, requestPage } from "./http";
import type {
  AdjacentBooks, ApiKey, ApiKeyCreated, Book, BookUpdate, Collection, CollectionCreate, CollectionUpdate,
  Dashboard, Health, Library, LibraryCreate, LibraryUpdate, LoginRequest, LoginScreen, MediaIssue, Page,
  ReaderPrefsAll, ReadList, ReadListCreate, ReadListUpdate, ReadProgress, SearchResult, Series, SeriesQuery,
  SeriesUpdate, ServerSettings, SetupRequest, Task, TrashResult, User, UserCreate, UserUpdate,
} from "./types";
import type { GlobalReaderPrefs, ReaderPrefs, ReaderProfile } from "./reader-prefs";

const enc = encodeURIComponent;
const get = <T,>(path: string) => request<T>("GET", path);
const byId = <T,>(path: string) => request<T | null>("GET", path, undefined, { nullOn404: true });

const notNull = <T,>(xs: (T | null)[]) => xs.filter((x): x is T => x !== null);

export const realApi = {
  // Sign-in (public: a 401 here is a wrong password, not an expired session)
  loginScreen: () => request<LoginScreen>("GET", "/public/login-screen", undefined, { public: true }),
  /** Liveness and the build version (the release tag, or "dev"). */
  health: () => request<Health>("GET", "/healthz", undefined, { public: true }),
  login: (body: LoginRequest) => request<User>("POST", "/auth/login", body, { public: true }),
  setup: (body: SetupRequest) => request<User>("POST", "/setup", body, { public: true }),
  logout: () => request<void>("POST", "/auth/logout", undefined, { public: true }),

  // The caller
  me: () => get<User>("/me"),
  /** locale "" follows the browser; omitted, the server keeps the stored choice. */
  updateMe: (name: string, locale?: "" | "ja" | "en") =>
    request<User>("PATCH", "/me", locale === undefined ? { name } : { name, locale }),
  changePassword: (currentPassword: string, newPassword: string) =>
    request<void>("PUT", "/me/password", { currentPassword, newPassword }),
  apiKeys: () => get<ApiKey[]>("/me/api-keys"),
  createApiKey: (name: string) => request<ApiKeyCreated>("POST", "/me/api-keys", { name }),
  deleteApiKey: (id: string) => request<void>("DELETE", `/me/api-keys/${enc(id)}`),

  // Libraries
  libraries: () => get<Library[]>("/libraries"),
  library: (id: string) => byId<Library>(`/libraries/${enc(id)}`),
  createLibrary: (body: LibraryCreate) => request<Library>("POST", "/libraries", body),
  updateLibrary: (id: string, body: LibraryUpdate) => request<Library>("PATCH", `/libraries/${enc(id)}`, body),
  deleteLibrary: (id: string) => request<void>("DELETE", `/libraries/${enc(id)}`),
  /** deep: analyse every book again, even unchanged ones (詳細スキャン). */
  scanLibrary: (id: string, deep = false) => request<Task>("POST", `/libraries/${enc(id)}/scan` + query({ deep: deep || undefined })),
  regenerateLibraryThumbnails: (id: string) => request<Task>("POST", `/libraries/${enc(id)}/regenerate-thumbnails`),
  markLibraryRead: (id: string, completed: boolean) =>
    request<void>("PUT", `/libraries/${enc(id)}/read-progress`, { completed }),

  // Series and books
  /** One page of series (limit/offset) and the count of all that match.
   *  There is deliberately no call for every series at once: a big library
   *  has tens of thousands. */
  seriesPage: (q: SeriesQuery & { limit: number; offset: number }) =>
    requestPage<Series>("/series" + query({ ...q })),
  /** Genres of the visible series (the genre filter's options). */
  genres: (libraryId?: string) => get<string[]>("/genres" + query({ libraryId })),
  seriesById: (id: string) => byId<Series>(`/series/${enc(id)}`),
  seriesByIds: async (ids: string[]) => notNull(await Promise.all(ids.map((id) => realApi.seriesById(id)))),
  updateSeries: (id: string, changes: SeriesUpdate) => request<Series>("PATCH", `/series/${enc(id)}`, changes),
  markSeriesRead: (id: string, completed: boolean) =>
    request<void>("PUT", `/series/${enc(id)}/read-progress`, { completed }),
  booksOfSeries: (seriesId: string) => get<Book[]>(`/series/${enc(seriesId)}/books`),
  book: (id: string) => byId<Book>(`/books/${enc(id)}`),
  booksByIds: async (ids: string[]) => notNull(await Promise.all(ids.map((id) => realApi.book(id)))),
  updateBook: (id: string, changes: BookUpdate) => request<Book>("PATCH", `/books/${enc(id)}`, changes),
  adjacentBooks: (id: string) => get<AdjacentBooks>(`/books/${enc(id)}/adjacent`),
  pages: (bookId: string) => get<Page[]>(`/books/${enc(bookId)}/pages`),
  /** writer + seq order writes from one reader: the server ignores a write
   *  older than the last one it stored from the same writer. */
  setProgress: (bookId: string, page: number, completed?: boolean, opts: { keepalive?: boolean; writer?: string; seq?: number } = {}) =>
    request<ReadProgress>("PUT", `/books/${enc(bookId)}/progress`, { page, completed, writer: opts.writer, seq: opts.seq }, { keepalive: opts.keepalive }),
  clearProgress: (bookId: string) => request<void>("DELETE", `/books/${enc(bookId)}/progress`),
  analyzeBook: (bookId: string) => request<Task>("POST", `/books/${enc(bookId)}/analyze`),
  regenerateBookThumbnail: (bookId: string) => request<void>("POST", `/books/${enc(bookId)}/regenerate-thumbnail`),
  setBookCover: (bookId: string, page: number) => request<Book>("PUT", `/books/${enc(bookId)}/cover`, { page }),
  regenerateSeriesThumbnails: (seriesId: string) => request<Task>("POST", `/series/${enc(seriesId)}/regenerate-thumbnails`),

  dashboard: (libraryId?: string) => get<Dashboard>("/dashboard" + query({ libraryId })),
  search: (q: string) => get<SearchResult>("/search" + query({ q: q.trim() })),

  // Collections and read lists
  collections: () => get<Collection[]>("/collections"),
  collection: (id: string) => byId<Collection>(`/collections/${enc(id)}`),
  collectionSeries: (id: string) => get<Series[]>(`/collections/${enc(id)}/series`),
  createCollection: (body: CollectionCreate) => request<Collection>("POST", "/collections", body),
  updateCollection: (id: string, body: CollectionUpdate) => request<Collection>("PATCH", `/collections/${enc(id)}`, body),
  deleteCollection: (id: string) => request<void>("DELETE", `/collections/${enc(id)}`),
  /** One member at a time; each returns the list as the caller now sees it. */
  addCollectionSeries: (id: string, seriesId: string) =>
    request<Collection>("POST", `/collections/${enc(id)}/series/${enc(seriesId)}`),
  removeCollectionSeries: (id: string, seriesId: string) =>
    request<Collection>("DELETE", `/collections/${enc(id)}/series/${enc(seriesId)}`),
  /** seriesIds: exactly the visible members in the new order (409 if they changed). */
  reorderCollection: (id: string, seriesIds: string[]) =>
    request<Collection>("PUT", `/collections/${enc(id)}/order`, { seriesIds }),
  readLists: () => get<ReadList[]>("/read-lists"),
  readList: (id: string) => byId<ReadList>(`/read-lists/${enc(id)}`),
  readListBooks: (id: string) => get<Book[]>(`/read-lists/${enc(id)}/books`),
  createReadList: (body: ReadListCreate) => request<ReadList>("POST", "/read-lists", body),
  updateReadList: (id: string, body: ReadListUpdate) => request<ReadList>("PATCH", `/read-lists/${enc(id)}`, body),
  deleteReadList: (id: string) => request<void>("DELETE", `/read-lists/${enc(id)}`),
  /** One book at a time; each returns the list as the caller now sees it. */
  addReadListBook: (id: string, bookId: string) =>
    request<ReadList>("POST", `/read-lists/${enc(id)}/books/${enc(bookId)}`),
  removeReadListBook: (id: string, bookId: string) =>
    request<ReadList>("DELETE", `/read-lists/${enc(id)}/books/${enc(bookId)}`),
  /** bookIds: exactly the visible members in the new order (409 if they changed). */
  reorderReadList: (id: string, bookIds: string[]) =>
    request<ReadList>("PUT", `/read-lists/${enc(id)}/order`, { bookIds }),

  // Reader settings (per user, on the server)
  readerPrefs: () => get<ReaderPrefsAll>("/me/reader-prefs"),
  putGlobalPrefs: (p: GlobalReaderPrefs, profile: ReaderProfile = "desktop") =>
    request<void>("PUT", `/me/reader-prefs/global${profile === "mobile" ? "?profile=mobile" : ""}`, p),
  putSeriesPrefs: (id: string, p: ReaderPrefs) => request<void>("PUT", `/me/reader-prefs/series/${enc(id)}`, p),
  deleteSeriesPrefs: (id: string) => request<void>("DELETE", `/me/reader-prefs/series/${enc(id)}`),
  putBookPrefs: (id: string, p: ReaderPrefs) => request<void>("PUT", `/me/reader-prefs/books/${enc(id)}`, p),
  deleteBookPrefs: (id: string) => request<void>("DELETE", `/me/reader-prefs/books/${enc(id)}`),

  // Administration
  users: () => get<User[]>("/users"),
  createUser: (body: UserCreate) => request<User>("POST", "/users", body),
  updateUser: (id: string, body: UserUpdate) => request<User>("PATCH", `/users/${enc(id)}`, body),
  deleteUser: (id: string) => request<void>("DELETE", `/users/${enc(id)}`),
  tasks: () => get<Task[]>("/tasks"),
  cancelTask: (id: string) => request<void>("POST", `/tasks/${enc(id)}/cancel`),
  mediaIssues: () => get<MediaIssue[]>("/media/issues"),
  emptyTrash: () => request<TrashResult>("POST", "/trash/empty"),
  settings: () => get<ServerSettings>("/settings"),
  /** Sends only the changed fields; the server merges them into the stored
   *  settings, so an older tab cannot undo a change it never saw. */
  saveSettings: (changes: Partial<ServerSettings>) => request<ServerSettings>("PATCH", "/settings", changes),
};

export type Api = typeof realApi;
