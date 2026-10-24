import { hostConnected } from "@/lib/desktop";
import type { DaemonMeta } from "@/lib/daemons";

/** Whether the route's host exists at all, and whether it can serve requests
 *  now. The implicit local host ("") is always both. */
export function routeHostState(
  hostId: string,
  daemons: DaemonMeta[],
): { knownHost: boolean; hostReady: boolean } {
  if (hostId === "") return { knownHost: true, hostReady: true };
  const host = daemons.find((entry) => entry.id === hostId);
  if (!host) return { knownHost: false, hostReady: false };
  return {
    knownHost: true,
    hostReady: !!host.baseURL && (host.kind !== "ssh" || hostConnected(host.state)),
  };
}
