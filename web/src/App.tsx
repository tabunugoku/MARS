import { lazy } from "react";
import { createBrowserRouter, RouterProvider } from "react-router";
import { Toaster } from "@yusufceng1z/ding-react";
import { FluidProvider } from "@/lib/fluid-provider";
import { keylineSlots } from "@/lib/icons";
import { ThemeProvider, useTheme } from "@/components/shell/theme";
import { AppShell } from "@/components/shell/app-shell";
import { RouteError } from "@/components/shell/route-error";

// Every screen is its own chunk; only the shell ships in the first load.
const Login = lazy(() => import("@/routes/login"));
const Dashboard = lazy(() => import("@/routes/dashboard"));
const LibraryView = lazy(() => import("@/routes/library"));
const SeriesView = lazy(() => import("@/routes/series"));
const BookView = lazy(() => import("@/routes/book"));
const Reader = lazy(() => import("@/routes/reader"));
const SearchView = lazy(() => import("@/routes/search"));
const Collections = lazy(() => import("@/routes/collections"));
const CollectionView = lazy(() => import("@/routes/collections/detail"));
const ReadLists = lazy(() => import("@/routes/readlists"));
const ReadListView = lazy(() => import("@/routes/readlists/detail"));
const AdminLibraries = lazy(() => import("@/routes/admin/libraries"));
const AdminUsers = lazy(() => import("@/routes/admin/users"));
const AdminTasks = lazy(() => import("@/routes/admin/tasks"));
const AdminMedia = lazy(() => import("@/routes/admin/media"));
const AdminSettings = lazy(() => import("@/routes/admin/settings"));
const ReaderSettingsView = lazy(() => import("@/routes/settings/reader"));

const router = createBrowserRouter([
  { path: "/login", element: <Login />, errorElement: <RouteError /> },
  { path: "/read/:bookId", element: <Reader />, errorElement: <RouteError /> },
  {
    element: <AppShell />,
    errorElement: <RouteError />,
    children: [
      { index: true, element: <Dashboard /> },
      { path: "libraries/:libraryId", element: <LibraryView /> },
      { path: "series/:seriesId", element: <SeriesView /> },
      { path: "books/:bookId", element: <BookView /> },
      { path: "search", element: <SearchView /> },
      { path: "collections", element: <Collections /> },
      { path: "collections/:collectionId", element: <CollectionView /> },
      { path: "readlists", element: <ReadLists /> },
      { path: "readlists/:readListId", element: <ReadListView /> },
      { path: "admin/libraries", element: <AdminLibraries /> },
      { path: "admin/users", element: <AdminUsers /> },
      { path: "admin/tasks", element: <AdminTasks /> },
      { path: "admin/media", element: <AdminMedia /> },
      { path: "admin/settings", element: <AdminSettings /> },
      { path: "settings/reader", element: <ReaderSettingsView /> },
      { path: "*", element: <Dashboard /> },
    ],
  },
]);

function ThemedToaster() {
  const { theme } = useTheme();
  return <Toaster position="bottom-center" theme={theme} labels={{ region: "通知", dismiss: "通知を閉じる" }} />;
}

export default function App() {
  return (
    <ThemeProvider>
      <FluidProvider icons={keylineSlots}>
        <RouterProvider router={router} />
        <ThemedToaster />
      </FluidProvider>
    </ThemeProvider>
  );
}
