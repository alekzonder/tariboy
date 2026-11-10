import { useMemo, useState } from "react";
import { useLocation } from "react-router-dom";
import { Toaster } from "@/components/ui/sonner";
import { ThemeToggle } from "@/components/ThemeToggle";
import { DaemonBanner } from "@/components/DaemonBanner";
import { DaemonProvider } from "@/components/DaemonProvider";
import { isDesktop } from "@/lib/desktop";
import { SidebarStateProvider } from "@/pages/terminals/SidebarStateProvider";
import { TitlebarSlotContext } from "@/pages/terminals/titlebarSlotContext";
import { CustomerQuestionNotifications } from "@/components/CustomerQuestionNotifications";
import { DesktopUpdatesProvider, UpdateBanner } from "@/components/DesktopUpdates";
import { SidebarToggle } from "@/components/SidebarToggle";
import { HelpMenu } from "@/components/onboarding/HelpMenu";
import { OnboardingProvider } from "@/components/onboarding/OnboardingProvider";
import { sidebarRoute } from "@/appChrome";
import { AppRoutes } from "@/routes";

export default function App() {
  return (
    <DaemonProvider>
      <CustomerQuestionNotifications>
        <SidebarStateProvider>
          <DesktopUpdatesProvider>
            <OnboardingProvider>
              <div className="flex h-screen flex-col">
                <MainApp />
              </div>
            </OnboardingProvider>
          </DesktopUpdatesProvider>
        </SidebarStateProvider>
      </CustomerQuestionNotifications>
      <Toaster richColors />
    </DaemonProvider>
  );
}

function MainApp() {
  const location = useLocation();
  const [workspaceSlot, setWorkspaceSlot] = useState<HTMLElement | null>(null);
  const [searchSlot, setSearchSlot] = useState<HTMLElement | null>(null);
  const titlebarSlots = useMemo(
    () => ({ workspace: workspaceSlot, search: searchSlot }),
    [workspaceSlot, searchSlot],
  );
  const withSidebar = sidebarRoute(location.pathname);
  return (
    <TitlebarSlotContext.Provider value={titlebarSlots}>
      {/* Topbar: 42px, no bottom border — the separation between chrome and the
          content island is made by the background, not by a line. */}
      <header
        className="flex h-10.5 shrink-0 items-center gap-2.5 px-3"
        data-tauri-drag-region="deep"
        data-testid="app-titlebar"
      >
        <div className="flex min-w-0 items-center gap-1.5">
          {/* Room for the macOS traffic lights under the Desktop app's overlay
              titlebar; a browser or the Android app has none. */}
          {isDesktop() && (
            <div
              aria-hidden="true"
              data-testid="titlebar-window-controls-gap"
              className="h-full w-18 shrink-0"
            />
          )}
          {withSidebar && <SidebarToggle />}
          <span className="text-body font-semibold">Tariboy</span>
          {/* The console pages portal the workspace switcher in here. */}
          {withSidebar && <div ref={setWorkspaceSlot} className="flex min-w-0" />}
        </div>
        <div aria-hidden="true" className="h-full min-w-4 flex-1" />
        {/* The console pages portal the global search in here. */}
        {withSidebar && <div ref={setSearchSlot} className="flex min-w-0" />}
        <div className="flex items-center gap-0.5">
          <HelpMenu />
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
