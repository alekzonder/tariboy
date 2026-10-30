import { useSearchParams } from "react-router-dom";
import { useDaemons, useOptionalDaemons } from "@/components/daemonContext";
import { hasLocalDaemon } from "./desktop";
import { hostToParam, paramToHost } from "./terminalsHost";

/** The host-scoped pages of the sidebar. Each takes an optional `?host=`. */
export type ScopedSection = "tasks" | "inbox" | "activity";

const LEGACY_SECTIONS: Record<string, ScopedSection> = {
  mail: "inbox",
  tasks: "tasks",
  activity: "activity",
};

/** The Library's old `/servers/:hostId/…` sections and their tabs now. */
const LEGACY_LIBRARY: Record<string, string> = {
  images: "images",
  workflows: "workflows",
  stores: "sources",
};

/** The pages of Settings, each at `/settings/<page>`. */
const SETTINGS_PAGES = new Set([
  "hosts",
  "teams",
  "usage",
  "agent-defaults",
  "integrations",
  "maintenance",
]);

/** The old Settings pages, by their tail under `/servers/:hostId/settings`. */
const LEGACY_SETTINGS: Record<string, string> = {
  "": "agent-defaults",
  "advanced/groups": "teams",
  "advanced/usage": "usage",
  "advanced/budgets": "usage",
  operations: "maintenance",
  "advanced/ops": "maintenance",
  "advanced/plugins": "integrations",
};

/** The Settings page of a Settings tail, old or new: an old page opens the
 *  page that holds it now, an unknown one opens Hosts. */
export function legacySettingsPath(tail: string): string {
  const path = tail.replace(/^\/+|\/+$/g, "");
  if (path in LEGACY_SETTINGS) return `/settings/${LEGACY_SETTINGS[path]}`;
  const operation = /^(?:maintenance\/)?operations\/([^/]+)$/.exec(path);
  if (operation) return `/settings/maintenance/operations/${operation[1]}`;
  if (/^integrations\/[^/]+$/.test(path) || SETTINGS_PAGES.has(path)) return `/settings/${path}`;
  return "/settings/hosts";
}

/** A place the sidebar or the tour opens. */
export type NavItem =
  | "home"
  | "tasks"
  | "inbox"
  | "library-images"
  | "library-workflows"
  | "library-sources"
  | "settings";

/** The path of a nav item on the page host `hostId`. */
export function navPath(item: NavItem, _hostId: string): string {
  switch (item) {
    case "home":
      return "/";
    case "tasks":
    case "inbox":
      return `/${item}`;
    case "library-images":
      return "/library/images";
    case "library-workflows":
      return "/library/workflows";
    case "library-sources":
      return "/library/sources";
    case "settings":
      return "/settings/hosts";
  }
}

/** The sidebar item a path belongs to; an agent or a team belongs to none. */
export function activeNav(
  pathname: string,
): "home" | "tasks" | "inbox" | "library" | "settings" | null {
  if (pathname === "/" || pathname === "/activity") return "home";
  if (pathname === "/tasks" || pathname === "/inbox") return pathname.slice(1) as "tasks" | "inbox";
  if (pathname === "/app-settings") return "settings";
  if (pathname === "/library" || pathname.startsWith("/library/")) return "library";
  if (pathname === "/settings" || pathname.startsWith("/settings/")) return "settings";
  return null;
}

/** `/tasks`, `/inbox` or `/activity`, with `?host=` only when a host is given:
 *  a link names its host only when the workspace has two or more. */
export function scopedPath(
  section: ScopedSection,
  hostId?: string,
  query: Record<string, string> = {},
): string {
  const params = new URLSearchParams();
  if (hostId !== undefined) params.set("host", hostToParam(hostId));
  for (const [key, value] of Object.entries(query)) params.set(key, value);
  const search = params.toString();
  return `/${section}${search ? `?${search}` : ""}`;
}

/**
 * Where an old URL now lives, or null when it still opens as it is: the
 * host-exact `/servers/:hostId/mail|tasks|activity` pages move the path host
 * into `?host=` and keep the rest of the query, the Library sections
 * `/servers/:hostId/images|workflows|stores/…` move to `/library/…` with the
 * rest of the path unchanged, an old Settings page (host-exact or bare
 * `/settings/…`) opens the `/settings/<page>` that holds it now, and the
 * former All tasks view (`?view=all`) opens Tasks.
 */
export function legacyServerRedirect(pathname: string, search: string): string | null {
  const params = new URLSearchParams(search);
  const withHost = (host: string) => {
    const next = new URLSearchParams({ host: decodeURIComponent(host) });
    for (const [key, value] of params) if (key !== "host") next.append(key, value);
    return next.toString();
  };
  const settings = /^\/servers\/([^/]+)\/settings(\/.*)?$/.exec(pathname);
  if (settings) return `${legacySettingsPath(settings[2] ?? "")}?${withHost(settings[1])}`;
  const bareSettings = /^\/settings(\/.*)?$/.exec(pathname);
  if (bareSettings) {
    const next = legacySettingsPath(bareSettings[1] ?? "");
    return next === pathname ? null : `${next}${search}`;
  }
  const match = /^\/servers\/([^/]+)\/(mail|tasks|activity)\/?$/.exec(pathname);
  if (match) return `/${LEGACY_SECTIONS[match[2]]}?${withHost(match[1])}`;
  const library = /^\/servers\/([^/]+)\/(images|workflows|stores)(\/.*)?$/.exec(pathname);
  if (library) {
    const rest = (library[3] ?? "").replace(/\/$/, "");
    return `/library/${LEGACY_LIBRARY[library[2]]}${rest}?${withHost(library[1])}`;
  }
  if (params.get("view") !== "all") return null;
  if (pathname !== "/" && !pathname.startsWith("/agents/")) return null;
  params.delete("view");
  const rest = params.toString();
  return `/tasks${rest ? `?${rest}` : ""}`;
}

/** Where the same page lives on another host: `/servers/<host>/…` swaps the
 *  path host, any other page takes `?host=`. */
export function hostScopeTarget(pathname: string, search: string, hostId: string): string {
  const param = hostToParam(hostId);
  const server = /^\/servers\/[^/]+(\/.*)?$/.exec(pathname);
  if (server) return `/servers/${encodeURIComponent(param)}${server[1] ?? ""}${search}`;
  const params = new URLSearchParams(search);
  params.set("host", param);
  return `${pathname}?${params.toString()}`;
}

/** Whether more than one host is registered: a link the UI builds then names
 *  its host. */
export function useMultiHost(): boolean {
  const daemons = useOptionalDaemons()?.daemons ?? [];
  return (hasLocalDaemon() ? 1 : 0) + daemons.length >= 2;
}

/** The host of a scoped page: `?host=` when the URL names one, otherwise the
 *  remembered active host. */
export function useScopeHost(): string {
  const [searchParams] = useSearchParams();
  const { activeId } = useDaemons();
  const param = searchParams.get("host");
  return param !== null ? paramToHost(param) : activeId;
}
