import type { HostAgents } from "@/lib/aggregate";
import type { DaemonMeta } from "@/lib/daemons";
import { hostConnected } from "@/lib/desktop";
import { hostToParam } from "@/lib/terminalsHost";
import type { WorkspaceHost } from "./HostWorkspaces";

type SidebarOrder = NonNullable<HostAgents["sidebarOrder"]>;

export interface CreateHost {
  id: string;
  label: string;
  revision: string;
}

export const fallbackHostLabel = (id: string) => (id === "" ? "This daemon (local)" : id);

/** `/agents/<host>/<agent>/<tab>` plus an optional query such as `?view=all`. */
export const agentPath = (hostId: string, agent: string, tab: string, query = "") =>
  `/agents/${hostToParam(hostId)}/${encodeURIComponent(agent)}/${tab}${query}`;

/** Every host an agent can be created on: the local daemon when there is one,
 *  then every host the aggregate or the registry knows. The revision changes
 *  whenever reachability, connection state or address does. */
export function buildCreateHosts(
  hosts: HostAgents[],
  daemons: DaemonMeta[],
  local: boolean,
): CreateHost[] {
  const aggregateById = new Map(hosts.map((entry) => [entry.host.id, entry]));
  const metadataById = new Map(daemons.map((entry) => [entry.id, entry]));
  const ids = new Set([
    ...(local ? [""] : []),
    ...hosts.map((entry) => entry.host.id),
    ...daemons.map((entry) => entry.id),
  ]);
  return [...ids].map((id) => {
    const aggregate = aggregateById.get(id);
    const metadata = metadataById.get(id);
    return {
      id,
      label: aggregate?.host.label ?? metadata?.label ?? fallbackHostLabel(id),
      revision: [
        aggregate ? (aggregate.error ?? "reachable") : "pending",
        metadata?.state ?? "",
        metadata?.baseURL ?? "",
      ].join("\u0000"),
    };
  });
}

/** The workspace menus' hosts: ready when reachable and connected. */
export function buildWorkspaceHosts(
  createHosts: CreateHost[],
  hosts: HostAgents[],
  daemons: DaemonMeta[],
): WorkspaceHost[] {
  return createHosts.map((host) => {
    const aggregate = hosts.find((entry) => entry.host.id === host.id);
    const state = daemons.find((entry) => entry.id === host.id)?.state ?? "ready";
    return {
      id: host.id,
      label: host.label,
      ready: Boolean(aggregate && !aggregate.error) && hostConnected(state),
    };
  });
}

/** A failing host keeps showing its last good agents, with the current error. */
export function withLastSuccessful(
  hosts: HostAgents[],
  lastSuccessful: ReadonlyMap<string, HostAgents>,
): HostAgents[] {
  return hosts.map((entry) => {
    if (!entry.error) return entry;
    const snapshot = lastSuccessful.get(entry.host.id);
    return snapshot ? { ...snapshot, error: entry.error } : entry;
  });
}

/** The route's host as polled now, and the one to show: the live host while it
 *  answers, else its last good snapshot. */
export function routeHost(
  hostId: string | undefined,
  hosts: HostAgents[],
  lastSuccessful: ReadonlyMap<string, HostAgents>,
): { liveSelectedHost?: HostAgents; selectedHost?: HostAgents } {
  if (hostId === undefined) return {};
  const liveSelectedHost = hosts.find((entry) => entry.host.id === hostId);
  if (liveSelectedHost && !liveSelectedHost.error) {
    return { liveSelectedHost, selectedHost: liveSelectedHost };
  }
  return { liveSelectedHost, selectedHost: lastSuccessful.get(hostId) ?? liveSelectedHost };
}

/** The saved server order, or none when it is missing or of another version. */
export function parseServerOrder(parsed: unknown): string[] {
  const value = parsed as { version?: number; ids?: unknown } | null;
  return value?.version === 1 && Array.isArray(value.ids)
    ? value.ids.filter((id): id is string => typeof id === "string")
    : [];
}

/** The sidebar reorders only the visible workspace's hosts: they take back the
 *  slots they held, so hidden hosts keep their places. */
export function mergeVisibleServerOrder(allIds: string[], visibleIds: string[]): string[] {
  const visible = new Set(visibleIds);
  let next = 0;
  return allIds.map((id) => (visible.has(id) ? visibleIds[next++] : id));
}

/** Every host in server order, each with its pending sidebar order applied. */
export function rankHosts(
  hosts: HostAgents[],
  serverOrder: string[],
  overrides: ReadonlyMap<string, SidebarOrder>,
): HostAgents[] {
  const rank = new Map(serverOrder.map((id, index) => [id, index]));
  return hosts
    .map((entry, index) => ({ entry, index, rank: rank.get(entry.host.id) }))
    .sort(
      (left, right) =>
        (left.rank ?? serverOrder.length + left.index) -
        (right.rank ?? serverOrder.length + right.index),
    )
    .map(({ entry }) => ({
      ...entry,
      sidebarOrder: overrides.get(entry.host.id) ?? entry.sidebarOrder,
    }));
}

/** The route host's label: as polled, else as registered, else its id. */
export function hostLabelFor(
  hostId: string,
  host: HostAgents | undefined,
  daemons: DaemonMeta[],
): string {
  return (
    host?.host.label ??
    daemons.find((daemon) => daemon.id === hostId)?.label ??
    fallbackHostLabel(hostId)
  );
}
