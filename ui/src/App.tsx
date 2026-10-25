import { useState } from "react";
import { Link, useLocation, useSearchParams } from "react-router-dom";
import { Toaster } from "@/components/ui/sonner";
import { ThemeToggle } from "@/components/ThemeToggle";
import { DaemonBanner } from "@/components/DaemonBanner";
import { DaemonProvider } from "@/components/DaemonProvider";
import { Button } from "@/components/ui/button";
import { Settings } from "lucide-react";
import { isDesktop } from "@/lib/desktop";
import { SidebarStateProvider } from "@/pages/terminals/SidebarStateProvider";
import { TitlebarSlotContext } from "@/pages/terminals/titlebarSlotContext";
import { CustomerQuestionNotifications } from "@/components/CustomerQuestionNotifications";
import { DesktopUpdatesProvider, UpdateBanner } from "@/components/DesktopUpdates";
import { SidebarToggle } from "@/components/SidebarToggle";
import { ViewSwitch } from "@/components/ViewSwitch";
import { appChromeState } from "@/appChrome";
import { AppRoutes } from "@/routes";

export default function App() {
  return (
    <DaemonProvider>
      <CustomerQuestionNotifications>
        <SidebarStateProvider>
          <DesktopUpdatesProvider>
            <div className="flex h-screen flex-col">
              <MainApp />
            </div>
          </DesktopUpdatesProvider>
        </SidebarStateProvider>
      </CustomerQuestionNotifications>
      <Toaster richColors />
    </DaemonProvider>
  );
}

function MainApp() {
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const [titlebarSlot, setTitlebarSlot] = useState<HTMLElement | null>(null);
  const { sidebarRoute, viewSwitch, allTasks } = appChromeState({
    pathname: location.pathname,
    view: searchParams.get("view"),
  });
  return (
    <TitlebarSlotContext.Provider value={titlebarSlot}>
      {/* Topbar: 42px, no bottom border — the separation between chrome and the
          content island is made by the background, not by a line. */}
      <header
        className="flex h-10.5 shrink-0 items-center gap-2.5 px-3"
        data-tauri-drag-region="deep"
        data-testid="app-titlebar"
      >
        <div className="flex min-w-0 items-center">
          {/* Room for the macOS traffic lights under the Desktop app's overlay
              titlebar; a browser or the Android app has none. */}
          {isDesktop() && (
            <div
              aria-hidden="true"
              data-testid="titlebar-window-controls-gap"
              className="h-full w-18 shrink-0"
            />
          )}
          {sidebarRoute && !allTasks && <SidebarToggle />}
          {/* The workspace switcher of the console pages portals in here. */}
          {sidebarRoute && <div ref={setTitlebarSlot} className="ml-1 flex min-w-0" />}
          {viewSwitch && <ViewSwitch allTasks={allTasks} />}
        </div>
        <div aria-hidden="true" className="h-full min-w-4 flex-1" />
        <div className="flex items-center gap-0.5">
          <Button
            asChild
            variant="ghost"
            size="icon-sm"
            className="size-6.5 rounded-control text-muted-foreground hover:bg-accent hover:text-foreground"
          >
            <Link to="/app-settings" aria-label="Application settings" title="Application settings">
              <Settings className="size-3.75" />
            </Link>
          </Button>
          <ThemeToggle
            variant="ghost"
            className="size-6.5 rounded-control text-muted-foreground hover:bg-accent hover:text-foreground"
          />
        </div>
      </header>
      <UpdateBanner />
      <DaemonBanner />
      <main className="min-h-0 flex-1 overflow-hidden">
        <AppRoutes />
      </main>
    </TitlebarSlotContext.Provider>
  );
}
