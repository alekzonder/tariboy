import type { ReactNode } from "react";
import { NavLink, Outlet, useLocation } from "react-router-dom";
import { cn } from "@/lib/utils";
import { LazyBoundary } from "@/components/LazyBoundary";
import { useActiveLinkIntoView } from "@/hooks/useActiveLinkIntoView";
import type { HostsOutletContext } from "./hostsContext";

const PAGES = [
  { path: "/settings/hosts", label: "Hosts" },
  { path: "/settings/teams", label: "Teams" },
  { path: "/settings/usage", label: "Usage & budgets" },
  { path: "/settings/agent-defaults", label: "Agent defaults" },
  { path: "/settings/integrations", label: "Integrations" },
  { path: "/settings/maintenance", label: "Maintenance" },
  // The titlebar has no gear: the application's own settings are here.
  { path: "/app-settings", label: "Application" },
];

const HOSTS = "/settings/hosts";

/**
 * Settings: one flat list of pages and the open page. Hosts reads the local
 * host registry, so it opens even when the selected host is unreachable;
 * every other page reads one host and sits inside `boundary`, under the host
 * selector.
 */
export default function SettingsPage({
  hostControl,
  boundary = (page) => page,
  hostsContext,
}: {
  /** The host selector, shown when the workspace has two or more hosts. */
  hostControl?: ReactNode;
  /** Connects a host page to the selected host. */
  boundary?: (page: ReactNode) => ReactNode;
  hostsContext?: HostsOutletContext;
}) {
  const location = useLocation();
  const strip = useActiveLinkIntoView<HTMLElement>();
  const hostsPage = location.pathname === HOSTS;
  const link = ({ isActive }: { isActive: boolean }) =>
    cn(
      "block rounded px-3 py-1.5 text-sm whitespace-nowrap hover:bg-accent max-lg:shrink-0 lg:py-2",
      isActive && "bg-accent font-medium",
    );
  return (
    // Below lg the side column is one sideways-scrolling strip above the page.
    <div className="flex h-full min-h-0 max-lg:flex-col">
      <aside
        ref={strip}
        className="w-52 shrink-0 overflow-y-auto border-r p-3 max-lg:scrollbar-none max-lg:flex max-lg:w-auto max-lg:items-center max-lg:gap-2 max-lg:overflow-x-auto max-lg:border-r-0 max-lg:border-b max-lg:py-1.5"
      >
        <h1 className="mb-3 text-lg font-semibold max-lg:sr-only">Settings</h1>
        <nav
          aria-label="Settings"
          className="space-y-1 max-lg:flex max-lg:shrink-0 max-lg:gap-1 max-lg:space-y-0"
        >
          {PAGES.map((page) => (
            <NavLink key={page.path} to={page.path} className={link}>
              {page.label}
            </NavLink>
          ))}
        </nav>
      </aside>
      <section className="flex min-w-0 flex-1 flex-col overflow-auto p-5 max-sm:p-3">
        {!hostsPage && hostControl && (
          <div className="mb-3 flex shrink-0 justify-end">{hostControl}</div>
        )}
        <div className="min-h-0 flex-1">
          <LazyBoundary resetKey={location.pathname}>
            {hostsPage ? <Outlet context={hostsContext} /> : boundary(<Outlet />)}
          </LazyBoundary>
        </div>
      </section>
    </div>
  );
}
