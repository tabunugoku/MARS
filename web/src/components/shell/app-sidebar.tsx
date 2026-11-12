import { Link, useLocation, useNavigate } from "react-router";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import { MenuItem } from "@/components/ui/menu-item";
import { SidebarUserFooter } from "@/components/sidebar-app/user-footer";
import { SidebarSearchField } from "@/components/sidebar-app/search-field";
import type { IconComponent } from "@/lib/icon-context";
import { useEffect, useRef } from "react";
import { api, useApi } from "@/api";
import { applyUserLocale, defineMessages, useT } from "@/i18n";
import { fontWeights } from "@/lib/font-weight";
import { typeClass } from "@/lib/type-scale";
import {
  House,
  Bookmarks,
  Grid2x2,
  LibraryBig,
  Settings,
  Users,
  Activity,
  FileAlert,
  Sun,
  Moon,
  LogOut,
  BookOpen,
  CircleUser,
} from "@keyline-icons/react";
import { useTheme } from "./theme";
import { LibraryGlyph } from "./library-glyph";
import { AppMark } from "@/components/brand/app-mark";
import { motion } from "motion/react";

const I = (c: unknown) => c as IconComponent;

const m = defineMessages({
  ja: {
    homeLabel: "ComiNavi ホーム",
    searchPlaceholder: "シリーズ・巻・著者を検索",
    search: "検索",
    home: "ホーム",
    collections: "コレクション",
    readLists: "リーディングリスト",
    library: "ライブラリ",
    all: "すべて",
    settings: "設定",
    reader: "リーダー",
    account: "アカウント",
    admin: "管理",
    libraryAdmin: "ライブラリ設定",
    users: "ユーザー",
    tasks: "タスク",
    media: "メディア管理",
    server: "サーバー設定",
    themeDark: "テーマ: ダーク",
    themeLight: "テーマ: ライト",
    logout: "ログアウト",
  },
  en: {
    homeLabel: "ComiNavi home",
    searchPlaceholder: "Search series, books, authors",
    search: "Search",
    home: "Home",
    collections: "Collections",
    readLists: "Read lists",
    library: "Library",
    all: "All",
    settings: "Settings",
    reader: "Reader",
    account: "Account",
    admin: "Admin",
    libraryAdmin: "Libraries",
    users: "Users",
    tasks: "Tasks",
    media: "Media issues",
    server: "Server settings",
    themeDark: "Theme: dark",
    themeLight: "Theme: light",
    logout: "Sign out",
  },
});

function NavRow({ to, icon, label, badge, exact }: { to: string; icon: IconComponent; label: string; badge?: string | number; exact?: boolean }) {
  const { pathname } = useLocation();
  const active = exact ? pathname === to : pathname === to || pathname.startsWith(`${to}/`);
  return (
    <SidebarMenuItem className="relative">
      {/* The route: one faint dot per row in the gutter, and the cyan
          "you are here" dot that glides along it to the current row. */}
      <span aria-hidden className="absolute -left-2.5 top-1/2 -mt-0.5 size-1 rounded-full bg-foreground/20" />
      {active && (
        <motion.span
          layoutId="cn-nav-here"
          aria-hidden
          className="absolute -left-3 top-1/2 z-10 -mt-1 size-2 rounded-full bg-here ring-[3px] ring-here/20"
          transition={{ type: "spring", mass: 0.6, stiffness: 380, damping: 26 }}
        />
      )}
      <SidebarMenuButton asChild icon={icon} isActive={active}>
        <Link to={to}>{label}</Link>
      </SidebarMenuButton>
      {badge !== undefined && <SidebarMenuBadge>{badge}</SidebarMenuBadge>}
    </SidebarMenuItem>
  );
}

/** Keeps the library counts and the task badge current from the event
 *  stream. live.ts is loaded with import() once the shell has mounted, so it
 *  stays out of the initial bundle. Only a task that appears or changes state
 *  reloads the badge; progress ticks do not. The last state seen in an event
 *  counts at once, so ticks that arrive while the list reloads do not start
 *  another reload. */
function useShellLive(reloadLibs: () => void, reloadTasks: () => void, tasks: { id: string; state: string }[] | undefined) {
  const latest = useRef({ reloadLibs, reloadTasks, states: new Map<string, string>() });
  useEffect(() => {
    latest.current = { reloadLibs, reloadTasks, states: new Map(tasks?.map((t) => [t.id, t.state])) };
  });
  useEffect(() => {
    let off: (() => void) | undefined;
    let dead = false;
    import("@/api/live").then(({ subscribeLive, debounce }) => {
      if (dead) return;
      const libsLater = debounce(() => latest.current.reloadLibs(), 2000, 10000);
      const tasksLater = debounce(() => latest.current.reloadTasks(), 500, 3000);
      const seen = new Map<string, string>();
      const unsubscribe = subscribeLive((e) => {
        if (e.type === "reconnect" || e.type === "open") {
          // A task may have ended before the stream opened, or in a gap.
          seen.clear();
          libsLater();
          tasksLater();
        } else if (e.type === "library") {
          libsLater();
        } else if (e.type === "task") {
          const was = seen.get(e.data.id) ?? latest.current.states.get(e.data.id);
          seen.set(e.data.id, e.data.state);
          if (was !== e.data.state) tasksLater();
        }
      });
      off = () => {
        unsubscribe();
        libsLater.cancel();
        tasksLater.cancel();
      };
    });
    return () => {
      dead = true;
      off?.();
    };
  }, []);
}

export function AppSidebar() {
  const navigate = useNavigate();
  const { theme, toggle } = useTheme();
  const { data: libs, reload: reloadLibs } = useApi(() => api.libraries(), []);
  const { data: me } = useApi(() => api.me(), []);
  const t = useT(m);
  // The signed-in user's own language choice (see docs/i18n.md).
  useEffect(() => {
    if (me) applyUserLocale(me.locale);
  }, [me]);
  // Running and queued tasks, for the badge on タスク (administrators only).
  const { data: tasks, reload: reloadTasks } = useApi(
    () => (me?.role === "admin" ? api.tasks() : Promise.resolve([])),
    [me?.role],
  );
  const activeTasks = tasks?.filter((t) => t.state === "running" || t.state === "queued").length ?? 0;
  useShellLive(reloadLibs, reloadTasks, tasks);

  return (
    <Sidebar variant="inset">
      <SidebarHeader className="pl-4">
        <Link to="/" className="flex h-9 items-center gap-2 px-2" aria-label={t("homeLabel")}>
          <AppMark size={28} className="-ml-1" />
          <span className={typeClass("subtitle")} style={{ fontVariationSettings: fontWeights.semibold }}>
            ComiNavi
          </span>
        </Link>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            const q = new FormData(e.currentTarget).get("q");
            navigate(`/search?q=${encodeURIComponent(String(q ?? ""))}`);
          }}
        >
          <SidebarSearchField name="q" placeholder={t("searchPlaceholder")} aria-label={t("search")} shortcut="/" />
        </form>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup className="pl-4">
          <SidebarMenu>
            <NavRow to="/" exact icon={I(House)} label={t("home")} />
            <NavRow to="/collections" icon={I(Grid2x2)} label={t("collections")} />
            <NavRow to="/readlists" icon={I(Bookmarks)} label={t("readLists")} />
          </SidebarMenu>
        </SidebarGroup>

        <SidebarGroup collapsible className="pl-4">
          <SidebarGroupLabel>{t("library")}</SidebarGroupLabel>
          <SidebarMenu>
            <NavRow to="/libraries/all" icon={I(LibraryBig)} label={t("all")} />
            {libs?.map((l) => (
              <NavRow key={l.id} to={`/libraries/${l.id}`} icon={LibraryGlyph(l.kind)} label={l.name} badge={l.seriesCount} />
            ))}
          </SidebarMenu>
        </SidebarGroup>

        {/* Settings split by who they belong to: 設定 is the signed-in
            user's own (everyone), 管理 is the server's (admins only). */}
        <SidebarGroup collapsible className="pl-4">
          <SidebarGroupLabel>{t("settings")}</SidebarGroupLabel>
          <SidebarMenu>
            <NavRow to="/settings/reader" icon={I(BookOpen)} label={t("reader")} />
            <NavRow to="/settings/account" icon={I(CircleUser)} label={t("account")} />
          </SidebarMenu>
        </SidebarGroup>

        {me?.role === "admin" && (
          <SidebarGroup collapsible className="pl-4">
            <SidebarGroupLabel>{t("admin")}</SidebarGroupLabel>
            <SidebarMenu>
              <NavRow to="/admin/libraries" icon={I(LibraryBig)} label={t("libraryAdmin")} />
              <NavRow to="/admin/users" icon={I(Users)} label={t("users")} />
              <NavRow to="/admin/tasks" icon={I(Activity)} label={t("tasks")} badge={activeTasks || undefined} />
              <NavRow to="/admin/media" icon={I(FileAlert)} label={t("media")} />
              <NavRow to="/admin/settings" icon={I(Settings)} label={t("server")} />
            </SidebarMenu>
          </SidebarGroup>
        )}
      </SidebarContent>

      <SidebarFooter className="pl-4">
        <SidebarUserFooter
          name={me?.name ?? "…"}
          avatar={
            <span className="flex size-5 items-center justify-center rounded-full bg-muted-foreground text-[length:var(--fs-micro-compact,10px)] leading-[var(--lh-micro-compact,12px)] text-background">
              {(me?.name ?? "?").slice(0, 1).toUpperCase()}
            </span>
          }
          menu={
            <>
              <MenuItem
                index={0}
                icon={I(theme === "dark" ? Moon : Sun)}
                label={theme === "dark" ? t("themeDark") : t("themeLight")}
                onSelect={() => toggle()}
              />
              <MenuItem index={1} icon={I(LogOut)} label={t("logout")} onSelect={() => void api.logout().finally(() => location.assign("/login"))} />
            </>
          }
        />
      </SidebarFooter>
    </Sidebar>
  );
}
