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
import { api, useApi } from "@/api";
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
} from "@keyline-icons/react";
import { useTheme } from "./theme";
import { LibraryGlyph } from "./library-glyph";
import { AppMark } from "@/components/media/kind-icon";

const I = (c: unknown) => c as IconComponent;

function NavRow({ to, icon, label, badge, exact }: { to: string; icon: IconComponent; label: string; badge?: string | number; exact?: boolean }) {
  const { pathname } = useLocation();
  const active = exact ? pathname === to : pathname === to || pathname.startsWith(`${to}/`);
  return (
    <SidebarMenuItem>
      <SidebarMenuButton asChild icon={icon} isActive={active}>
        <Link to={to}>{label}</Link>
      </SidebarMenuButton>
      {badge !== undefined && <SidebarMenuBadge>{badge}</SidebarMenuBadge>}
    </SidebarMenuItem>
  );
}

export function AppSidebar() {
  const navigate = useNavigate();
  const { theme, toggle } = useTheme();
  const { data: libs } = useApi(() => api.libraries(), []);
  const { data: me } = useApi(() => api.me(), []);

  return (
    <Sidebar variant="inset">
      <SidebarHeader>
        <Link to="/" className="flex h-9 items-center gap-2 px-2" aria-label="ComiNavi ホーム">
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
          <SidebarSearchField name="q" placeholder="シリーズ・巻・著者を検索" aria-label="検索" shortcut="/" />
        </form>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarMenu>
            <NavRow to="/" exact icon={I(House)} label="ホーム" />
            <NavRow to="/collections" icon={I(Grid2x2)} label="コレクション" />
            <NavRow to="/readlists" icon={I(Bookmarks)} label="リーディングリスト" />
          </SidebarMenu>
        </SidebarGroup>

        <SidebarGroup collapsible>
          <SidebarGroupLabel>ライブラリ</SidebarGroupLabel>
          <SidebarMenu>
            <NavRow to="/libraries/all" icon={I(LibraryBig)} label="すべて" />
            {libs?.map((l) => (
              <NavRow key={l.id} to={`/libraries/${l.id}`} icon={LibraryGlyph(l.kind)} label={l.name} badge={l.seriesCount} />
            ))}
          </SidebarMenu>
        </SidebarGroup>

        {me?.role === "admin" && (
          <SidebarGroup collapsible>
            <SidebarGroupLabel>管理</SidebarGroupLabel>
            <SidebarMenu>
              <NavRow to="/admin/libraries" icon={I(LibraryBig)} label="ライブラリ設定" />
              <NavRow to="/admin/users" icon={I(Users)} label="ユーザー" />
              <NavRow to="/admin/tasks" icon={I(Activity)} label="タスク" badge={2} />
              <NavRow to="/admin/media" icon={I(FileAlert)} label="メディア管理" />
              <NavRow to="/admin/settings" icon={I(Settings)} label="サーバー設定" />
            </SidebarMenu>
          </SidebarGroup>
        )}
      </SidebarContent>

      <SidebarFooter>
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
                icon={I(theme === "dark" ? Sun : Moon)}
                label={theme === "dark" ? "ライトテーマ" : "ダークテーマ"}
                onSelect={toggle}
              />
              <MenuItem index={1} icon={I(Settings)} label="アカウント設定" onSelect={() => navigate("/account")} />
              <MenuItem index={2} icon={I(LogOut)} label="ログアウト" onSelect={() => navigate("/login")} />
            </>
          }
        />
      </SidebarFooter>
    </Sidebar>
  );
}
