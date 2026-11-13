import { lazy, useEffect } from "react";
import { createBrowserRouter, RouterProvider } from "react-router";
import { Toaster } from "@yusufceng1z/ding-react";
import { FluidProvider } from "@/lib/fluid-provider";
import { keylineSlots } from "@/lib/icons";
import { ThemeProvider, useTheme } from "@/components/shell/theme";
import { AppShell } from "@/components/shell/app-shell";
import { RouteError } from "@/components/shell/route-error";
import { api } from "@/api";
import { applyUserLocale, defineMessages, useLocale, useT } from "@/i18n";

const m = defineMessages({
  ja: {
    notifications: "通知",
    dismiss: "通知を閉じる",
  },
  en: {
    notifications: "Notifications",
    dismiss: "Dismiss notification",
  },
});

// Every screen is its own chunk; only the shell ships in the first load.
const Login = lazy(() => import("@/routes/login"));
const Dashboard = lazy(() => import("@/routes/dashboard"));
const LibrariesView = lazy(() => import("@/routes/libraries"));
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
const AccountSettings = lazy(() => import("@/routes/settings/account"));

const router = createBrowserRouter([
  { path: "/login", element: <Login />, errorElement: <RouteError /> },
  { path: "/read/:bookId", element: <Reader />, errorElement: <RouteError /> },
  {
    element: <AppShell />,
    errorElement: <RouteError />,
    children: [
      { index: true, element: <Dashboard /> },
      { path: "libraries", element: <LibrariesView /> },
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
      { path: "settings/account", element: <AccountSettings /> },
      { path: "*", element: <Dashboard /> },
    ],
  },
]);

function ThemedToaster() {
  const { theme } = useTheme();
  const t = useT(m);
  return <Toaster position="bottom-center" theme={theme} labels={{ region: t("notifications"), dismiss: t("dismiss") }} />;
}

// The signed-in user's language (see docs/dev/i18n.md). It sits above the router,
// so it asks once per page load on every route, the reader included. A
// language change remounts the router below, but not this, so nothing repeats.
function LocaleSync() {
  useEffect(() => {
    api.sessionUser().then((u) => {
      if (u) applyUserLocale(u.locale);
    }, () => {});
  }, []);
  return null;
}

export default function App() {
  // A language change remounts the screens, so text that components keep in
  // state or memo (Fluid's labels among them) is drawn again in the new one.
  const locale = useLocale();
  return (
    <ThemeProvider>
      <FluidProvider icons={keylineSlots}>
        <LocaleSync />
        <RouterProvider key={locale} router={router} />
        <ThemedToaster />
      </FluidProvider>
    </ThemeProvider>
  );
}
