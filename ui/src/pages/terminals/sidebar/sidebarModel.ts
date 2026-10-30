import type { HostAgents } from "@/lib/aggregate";
import type { AgentRow } from "@/lib/api";
import { customerQuestionAttentionKey } from "@/components/customerQuestionNotificationModel";

/** Identity of one agent across the whole sidebar. The same encoding the
 *  customer-question notifications use, so one key looks both sets up. */
export const agentKey = customerQuestionAttentionKey;

/** One agent, carrying the host it came from — the pinned block, Home and the
 *  search list agents across every host, so a row cannot rely on a
 *  surrounding section to say where it lives. */
export interface SidebarAgent {
  key: string;
  hostId: string;
  hostLabel: string;
  hostError?: string;
  group: string;
  agent: AgentRow;
}

/** Sort `items` by an explicit id order, keeping unlisted items in their
 *  current relative order after the listed ones. */
function ordered<T>(items: T[], ids: string[], id: (item: T) => string): T[] {
  const rank = new Map(ids.map((value, index) => [value, index]));
  return items
    .map((item, index) => ({ item, index, rank: rank.get(id(item)) }))
    .sort(
      (left, right) =>
        (left.rank ?? ids.length + left.index) - (right.rank ?? ids.length + right.index),
    )
    .map(({ item }) => item);
}

/** Agents of one host in the operator's saved order. */
export function hostAgents(host: HostAgents): SidebarAgent[] {
  return ordered(host.agents, host.sidebarOrder?.agents ?? [], (agent) => agent.name).map(
    (agent) => ({
      key: agentKey(host.host.id, agent.name),
      hostId: host.host.id,
      hostLabel: host.host.label,
      hostError: host.error,
      group: agent.group?.trim() ?? "",
      agent,
    }),
  );
}

/** A server's team names, declared or used by an agent, in the saved order. */
export function teamNames(host: HostAgents, agents: SidebarAgent[]): string[] {
  const names = [
    ...new Set([
      ...(host.groups ?? []).map((group) => group.name),
      ...agents.map((row) => row.group).filter(Boolean),
    ]),
  ];
  return ordered(names, host.sidebarOrder?.groups ?? [], (name) => name);
}

/** A server's teams with their members, a declared team with none included. */
export function groupTeams(host: HostAgents, agents: SidebarAgent[]): Map<string, SidebarAgent[]> {
  const teams = new Map<string, SidebarAgent[]>();
  for (const name of teamNames(host, agents)) teams.set(name, []);
  for (const row of agents) {
    if (!row.group) continue;
    teams.set(row.group, [...(teams.get(row.group) ?? []), row]);
  }
  return teams;
}

/** Every agent of every server, servers in sidebar order. */
export function allAgents(hosts: HostAgents[]): SidebarAgent[] {
  return hosts.flatMap(hostAgents);
}

/** One host's part of the agent list: its teams, then its other agents. */
export interface HostSection {
  host: HostAgents;
  teams: Map<string, SidebarAgent[]>;
  individuals: SidebarAgent[];
}

/**
 * The agent list under Agents: pinned agents on top, then each host's teams
 * and individual agents in the saved order. A pinned agent shows only on top.
 * Host headings appear only when the workspace has two or more hosts.
 */
export function sidebarLayout(
  hosts: HostAgents[],
  pinned: ReadonlySet<string>,
): { pinned: SidebarAgent[]; byHost: boolean; sections: HostSection[] } {
  const sections = hosts.map((host) => {
    const agents = hostAgents(host);
    const teams = groupTeams(host, agents);
    for (const [name, rows] of teams)
      teams.set(
        name,
        rows.filter((row) => !pinned.has(row.key)),
      );
    return {
      host,
      teams,
      individuals: agents.filter((row) => !row.group && !pinned.has(row.key)),
    };
  });
  return {
    pinned: allAgents(hosts).filter((row) => pinned.has(row.key)),
    byHost: hosts.length >= 2,
    sections,
  };
}

/** The sidebar footer's line: "Local · ready", "2 hosts · 1 unreachable". */
export function hostsLine(hosts: HostAgents[]): string {
  if (hosts.length === 0) return "No hosts";
  const unreachable = hosts.filter((host) => host.error).length;
  if (hosts.length === 1) {
    const [only] = hosts;
    const name = only.host.id === "" ? "Local" : only.host.label;
    return `${name} · ${unreachable ? "unreachable" : "ready"}`;
  }
  return `${hosts.length} hosts · ${unreachable ? `${unreachable} unreachable` : "ready"}`;
}

/**
 * The Groups tab: one section per group across all servers. A group declared on
 * a server but holding no agents still gets a section, so "No agents." says the
 * group exists rather than leaving the operator to wonder where it went.
 */
export function groupSections(
  hosts: HostAgents[],
  agents: SidebarAgent[],
): Array<{
  name: string;
  agents: SidebarAgent[];
}> {
  const sections = new Map<string, SidebarAgent[]>();
  for (const host of hosts) {
    for (const group of host.groups ?? []) {
      if (!sections.has(group.name)) sections.set(group.name, []);
    }
  }
  for (const row of agents) {
    if (!row.group) continue;
    sections.set(row.group, [...(sections.get(row.group) ?? []), row]);
  }
  return [...sections.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([name, rows]) => ({ name, agents: rows }));
}

/** Keep the hosts (and their agents) that match a search query by agent name or
 *  server label. An empty query filters nothing. */
export function filterHosts(hosts: HostAgents[], query: string): HostAgents[] {
  const q = query.trim().toLowerCase();
  if (!q) return hosts;
  return hosts
    .map((host) => ({
      ...host,
      agents: host.agents.filter((agent) => agent.name.toLowerCase().includes(q)),
    }))
    .filter((host) => host.agents.length > 0 || host.host.label.toLowerCase().includes(q));
}
