// API shapes, generated from api/openapi.yaml (npm run api:types). Screens
// import only from "@/api"; never edit schema.d.ts by hand.
import type { components } from "./schema";

type S = components["schemas"];

export type ReadingDirection = S["ReadingDirection"];
export type SeriesStatus = S["SeriesStatus"];
export type MediaStatus = S["MediaStatus"];
export type BookFormat = S["BookFormat"];
export type Library = S["Library"];
export type LibraryKind = S["LibraryKind"];
export type LibraryCreate = S["LibraryCreate"];
export type LibraryUpdate = S["LibraryUpdate"];
export type Author = S["Author"];
export type Series = S["Series"];
export type SeriesUpdate = S["SeriesUpdate"];
export type ReadProgress = S["ReadProgress"];
export type Book = S["Book"];
export type BookUpdate = S["BookUpdate"];
export type Page = S["Page"];
export type AdjacentBooks = S["AdjacentBooks"];
export type Dashboard = S["Dashboard"];
export type SearchResult = S["SearchResult"];
export type Collection = S["Collection"];
export type CollectionCreate = S["CollectionCreate"];
export type CollectionUpdate = S["CollectionUpdate"];
export type ReadList = S["ReadList"];
export type ReadListCreate = S["ReadListCreate"];
export type ReadListUpdate = S["ReadListUpdate"];
export type Role = S["Role"];
export type User = S["User"];
export type UserCreate = S["UserCreate"];
export type UserUpdate = S["UserUpdate"];
export type ApiKey = S["ApiKey"];
export type ApiKeyCreated = S["ApiKeyCreated"];
export type TaskKind = S["TaskKind"];
export type TaskState = S["TaskState"];
export type Task = S["Task"];
export type MediaIssue = S["MediaIssue"];
export type ServerSettings = S["ServerSettings"];
export type LoginBackdrop = S["LoginBackdrop"];
export type LoginScreen = S["LoginScreen"];
export type Health = S["Health"];
export type LoginCover = S["LoginCover"];
export type SetupRequest = S["SetupRequest"];
export type LoginRequest = S["LoginRequest"];
export type TrashResult = S["TrashResult"];
export type ReaderPrefsAll = S["ReaderPrefsAll"];
export type SeriesSort = S["SeriesSort"];
export type ReadState = S["ReadState"];
export type SeriesFacets = S["SeriesFacets"];
export type SeriesBulkUpdate = S["SeriesBulkUpdate"];
export type BookBulkUpdate = S["BookBulkUpdate"];
export type BulkResult = S["BulkResult"];
export type CblEntry = S["CblEntry"];
export type CblImportResult = S["CblImportResult"];
export type SeriesThumbnailSource = S["SeriesThumbnailSource"];

export interface SeriesQuery {
  libraryId?: string;
  search?: string;
  status?: SeriesStatus[];
  genres?: string[];
  tags?: string[];
  publishers?: string[];
  languages?: string[];
  authors?: string[];
  /** Age rating at most this; series without one are included. */
  ageRatingMax?: number;
  /** true: total book count known and reached; false: the others. */
  complete?: boolean;
  readState?: ReadState[];
  sort?: SeriesSort;
  desc?: boolean;
  limit?: number;
  offset?: number;
}
