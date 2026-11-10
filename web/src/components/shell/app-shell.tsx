import { Suspense, type ReactNode } from "react";
import { Link, Outlet, useLocation } from "react-router";
import { SidebarInset, SidebarProvider, useSidebar } from "@/components/ui/sidebar";
import { SidebarInsetTopbar } from "@/components/sidebar-app/inset-topbar";
import { Button } from "@/components/ui/button";
import { Tooltip } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { typeClass } from "@/lib/type-scale";
import { House, LibraryBig, Search, Grid2x2, Sun, Moon } from "@keyline-icons/react";
import { AppSidebar } from "./app-sidebar";
import { useTheme } from "./theme";
import { PageSkeleton } from "@/components/media/skeletons";

/** Desktop: inset sidebar + content card. Mobile (<768px): bottom tabs, and
 *  the sidebar becomes a drawer opened from the "ライブラリ" tab. */
export function AppShell() {
  return (
    <SidebarProvider peek="hover">
      <AppSidebar />
      <SidebarInset className="overflow-hidden">
        <Topbar />
        <div className="min-h-0 flex-1 overflow-y-auto pb-20 md:pb-0" id="cn-scroll">
          <Suspense fallback={<PageSkeleton />}>
            <Outlet />
          </Suspense>
        </div>
      </SidebarInset>
      <BottomTabs />
    </SidebarProvider>
  );
}

function Topbar() {
  const { theme, toggle } = useTheme();
  return (
    <SidebarInsetTopbar className="pr-2">
      <div className="flex-1" />
      <Tooltip content={theme === "dark" ? "ライトテーマ" : "ダークテーマ"} side="bottom">
        <Button variant="ghost" size="icon" aria-label="テーマを切り替え" onClick={toggle}>
          {theme === "dark" ? <Sun size={16} /> : <Moon size={16} />}
        </Button>
      </Tooltip>
    </SidebarInsetTopbar>
  );
}

function Tab({ to, label, icon, active, onClick }: { to?: string; label: string; icon: ReactNode; active: boolean; onClick?: () => void }) {
  const cls = cn(
    "flex flex-1 flex-col items-center justify-center gap-0.5 py-1.5 transition-colors duration-80",
    typeClass("micro"),
    active ? "text-foreground" : "text-muted-foreground",
  );
  const body = (
    <>
      <span className={cn("flex h-7 w-12 items-center justify-center rounded-full", active && "bg-brand-soft text-brand")}>{icon}</span>
      {label}
    </>
  );
  return to ? (
    <Link to={to} className={cls} aria-current={active ? "page" : undefined}>
      {body}
    </Link>
  ) : (
    <button type="button" className={cls} onClick={onClick}>
      {body}
    </button>
  );
}

function BottomTabs() {
  const { pathname } = useLocation();
  const { setOpenMobile } = useSidebar();
  return (
    <nav
      aria-label="メインナビゲーション"
      className="fixed inset-x-0 bottom-0 z-[var(--z-sticky)] flex border-t border-border bg-background/95 pb-[env(safe-area-inset-bottom)] backdrop-blur md:hidden"
    >
      <Tab to="/" label="ホーム" icon={<House size={18} />} active={pathname === "/"} />
      <Tab label="ライブラリ" icon={<LibraryBig size={18} />} active={pathname.startsWith("/libraries")} onClick={() => setOpenMobile(true)} />
      <Tab to="/search" label="検索" icon={<Search size={18} />} active={pathname.startsWith("/search")} />
      <Tab to="/collections" label="コレクション" icon={<Grid2x2 size={18} />} active={pathname.startsWith("/collections") || pathname.startsWith("/readlists")} />
    </nav>
  );
}
