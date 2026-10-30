import { useSearchParams } from "react-router-dom";
import { useDaemons } from "@/components/daemonContext";
import { hostToParam, paramToHost, serverPath } from "./terminalsHost";

/** The host-scoped pages of the sidebar. Each takes an optional `?host=`. */
export type ScopedSection = "tasks" | "inbox" | "activity";

const LEGACY_SECTIONS: Record<string, ScopedSection> = {
  mail: "inbox",
  tasks: "tasks",
  activity: "activity",
};

/** A place the sidebar or the tour opens. Library and Settings still live
 *  under `/servers/:hostId/…` until they get their own routes. */
export type NavItem =
  | "home"
  | "tasks"
  | "inbox"
  | "library-images"
  | "library-workflows"
  | "library-sources"
  | "settings";

/** The path of a nav item on the page host `hostId`. */
export function navPath(item: NavItem, hostId: string): string {
  switch (item) {
    case "home":
      return "/";
    case "tasks":
    case "inbox":
      return `/${item}`;
    case "library-images":
      return serverPath(hostId, "images");
    case "library-workflows":
      return serverPath(hostId, "workflows");
    case "library-sources":
      return serverPath(hostId, "stores");
    case "settings":
      return serverPath(hostId, "settings");
  }
}

/** The sidebar item a path belongs to; an agent or a team belongs to none. */
export function activeNav(
  pathname: string,
): "home" | "tasks" | "inbox" | "library" | "settings" | null {
  if (pathname === "/" || pathname === "/activity") return "home";
  if (pathname === "/tasks" || pathname === "/inbox") return pathname.slice(1) as "tasks" | "inbox";
  if (pathname === "/app-settings") return "settings";
  const section = /^\/servers\/[^/]+\/([^/]+)/.exec(pathname)?.[1];
  if (section === "images" || section === "workflows" || section === "stores") return "library";
  if (section === "settings") return "settings";
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
 * into `?host=` and keep the rest of the query, and the former All tasks view
 * (`?view=all`) opens Tasks.
 */
export function legacyServerRedirect(pathname: string, search: string): string | null {
  const params = new URLSearchParams(search);
  const match = /^\/servers\/([^/]+)\/(mail|tasks|activity)\/?$/.exec(pathname);
  if (match) {
    const next = new URLSearchParams({ host: decodeURIComponent(match[1]) });
    for (const [key, value] of params) if (key !== "host") next.append(key, value);
    return `/${LEGACY_SECTIONS[match[2]]}?${next.toString()}`;
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

/** The host of a scoped page: `?host=` when the URL names one, otherwise the
 *  remembered active host. */
export function useScopeHost(): string {
  const [searchParams] = useSearchParams();
  const { activeId } = useDaemons();
  const param = searchParams.get("host");
  return param !== null ? paramToHost(param) : activeId;
}
