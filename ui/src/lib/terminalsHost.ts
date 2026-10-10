import { cachedDaemon, unresolvedDaemon, type Daemon } from "@/lib/daemons";

// URL param mapping for /terminals/:hostId/:agent. The same-origin local
// daemon has registry id "" which cannot appear in a path segment, so it is
// spelled "local" in URLs.
export const LOCAL_PARAM = "local";
export const hostToParam = (id: string): string => (id === "" ? LOCAL_PARAM : id);
export const paramToHost = (p: string): string => (p === LOCAL_PARAM ? "" : p);
export type ServerSection = "tasks" | "images" | "workflows" | "stores" | "settings";
export const serverPath = (hostId: string, section: ServerSection): string =>
  `/servers/${encodeURIComponent(hostToParam(hostId))}/${section}`;

// targetFor resolves a host id to an explicit api target: null = same-origin
// local (NOT the active daemon), a Daemon = that registered host.
// The placeholder is reused so a re-render does not hand the page a new target.
const unresolved = new Map<string, Daemon>();
export function targetFor(hostId: string): Daemon | null {
  if (hostId === "") return null;
  const cached = cachedDaemon(hostId);
  if (cached) return cached;
  if (!unresolved.has(hostId)) unresolved.set(hostId, unresolvedDaemon(hostId));
  return unresolved.get(hostId)!;
}
