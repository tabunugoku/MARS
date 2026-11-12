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
import { CompassBackdrop } from "@/components/brand/compass-backdrop";
import { motion } from "motion/react";
import { defineMessages, useT } from "@/i18n";

const m = defineMessages({
  ja: {
    darkNow: "ダークテーマ（クリックでライト）",
    lightNow: "ライトテーマ（クリックでダーク）",
    mainNav: "メインナビゲーション",
    home: "ホーム",
    library: "ライブラリ",
    search: "検索",
    collections: "コレクション",
  },
  en: {
    darkNow: "Dark theme (click for light)",
    lightNow: "Light theme (click for dark)",
    mainNav: "Main navigation",
    home: "Home",
    library: "Library",
    search: "Search",
    collections: "Collections",
  },
});

/** Desktop: inset sidebar + content card. Mobile (<768px): bottom tabs, and
 *  the sidebar becomes a drawer opened from the "ライブラリ" tab. */
export function AppShell() {
  // viewport-fit=cover (index.html): keep clear of the notch and rounded
  // corners in landscape, and of the status bar if it overlays.
  return (
    <SidebarProvider peek="hover" className="h-svh pl-[env(safe-area-inset-left)] pr-[env(safe-area-inset-right)] pt-[env(safe-area-inset-top)]">
      <AppSidebar />
      <SidebarInset className="overflow-hidden">
        <CompassBackdrop />
        <Topbar />
        <div className="relative min-h-0 flex-1 overflow-y-auto pb-[calc(5rem+env(safe-area-inset-bottom))] md:pb-[env(safe-area-inset-bottom)]" id="cn-scroll">
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
  const t = useT(m);
  // Names the theme in use; a click switches to the other one.
  const label = theme === "dark" ? t("darkNow") : t("lightNow");
  return (
    <SidebarInsetTopbar className="relative pr-2">
      <div className="flex-1" />
      <Tooltip content={label} side="bottom">
        <Button variant="ghost" size="icon" aria-label={label} onClick={(e) => toggle(e.currentTarget)}>
          <ThemeGlyph dark={theme === "dark"} />
        </Button>
      </Tooltip>
    </SidebarInsetTopbar>
  );
}

// The glyph shows the theme in use (moon = dark, sun = light), not the one a
// click switches to. Sun and moon share one cell: the one leaving turns away
// and shrinks while the other turns in, so the swap reads as a dial turning.
const glyphSpring = { type: "spring", mass: 0.8, stiffness: 320, damping: 20 } as const;
function ThemeGlyph({ dark }: { dark: boolean }) {
  const shown = { rotate: 0, scale: 1, opacity: 1 };
  return (
    <span className="relative grid size-4 place-items-center [&>*]:col-start-1 [&>*]:row-start-1">
      <motion.span initial={false} animate={dark ? { rotate: 90, scale: 0.4, opacity: 0 } : shown} transition={glyphSpring}>
        <Sun size={16} />
      </motion.span>
      <motion.span initial={false} animate={dark ? shown : { rotate: -90, scale: 0.4, opacity: 0 }} transition={glyphSpring}>
        <Moon size={16} />
      </motion.span>
    </span>
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
  const t = useT(m);
  return (
    <nav
      aria-label={t("mainNav")}
      className="fixed inset-x-0 bottom-0 z-[var(--z-sticky)] flex border-t border-border bg-background pb-[env(safe-area-inset-bottom)] pl-[env(safe-area-inset-left)] pr-[env(safe-area-inset-right)] md:hidden"
    >
      <Tab to="/" label={t("home")} icon={<House size={18} />} active={pathname === "/"} />
      <Tab label={t("library")} icon={<LibraryBig size={18} />} active={pathname.startsWith("/libraries")} onClick={() => setOpenMobile(true)} />
      <Tab to="/search" label={t("search")} icon={<Search size={18} />} active={pathname.startsWith("/search")} />
      <Tab to="/collections" label={t("collections")} icon={<Grid2x2 size={18} />} active={pathname.startsWith("/collections") || pathname.startsWith("/readlists")} />
    </nav>
  );
}
