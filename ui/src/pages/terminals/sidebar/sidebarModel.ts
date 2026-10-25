import type { HostAgents } from "@/lib/aggregate";
import type { AgentRow, ChatSummaryRow } from "@/lib/api";
import { customerQuestionAttentionKey } from "@/components/customerQuestionNotificationModel";

/** Identity of one agent across the whole sidebar. The same encoding the
 *  customer-question notifications use, so one key looks both sets up. */
export const agentKey = customerQuestionAttentionKey;

/** One agent, carrying the host it came from — the Agents and Groups tabs list
 *  agents across every server, so a row cannot rely on a surrounding section to
 *  say where it lives. */
export interface SidebarAgent {
  key: string;
  hostId: string;
  hostLabel: string;
  hostError?: string;
  group: string;
  agent: AgentRow;
  /** This agent's chats folded into one row, when it has any. */
  chat?: ChatSummaryRow;
}

/**
 * One sidebar row stands for the whole agent, and an agent owns three chats —
 * its conversation, its task notifications and its service wake-ups. The row
 * shows whichever of them moved last and counts every one of them as unread,
 * so a task the customer has not read cannot hide behind a quiet conversation.
 */
function foldAgentChats(chats: ChatSummaryRow[]): Map<string, ChatSummaryRow> {
  const byAgent = new Map<string, ChatSummaryRow>();
  for (const chat of chats) {
    if (!chat.agent) continue;
    const seen = byAgent.get(chat.agent);
    if (!seen) {
      byAgent.set(chat.agent, { ...chat });
      continue;
    }
    seen.unread += chat.unread;
    if (chat.last_ts > seen.last_ts) {
      Object.assign(seen, chat, { unread: seen.unread });
    }
  }
  return byAgent;
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
  const chats = foldAgentChats(host.chats ?? []);
  return ordered(host.agents, host.sidebarOrder?.agents ?? [], (agent) => agent.name).map(
    (agent) => ({
      key: agentKey(host.host.id, agent.name),
      hostId: host.host.id,
      hostLabel: host.host.label,
      hostError: host.error,
      group: agent.group?.trim() ?? "",
      agent,
      chat: chats.get(agent.name),
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

/**
 * The Agents tab order, and the only place that decides it: pinned agents on
 * top, then the ones calling for a person (an unread customer question), then
 * the rest as a chat list — most recent conversation first, silent agents last
 * in the operator's own order. The whole sort happens here, across every
 * server, because only the Desktop sees all of them.
 */
export function rankAgents(
  agents: SidebarAgent[],
  pinned: ReadonlySet<string>,
  attention: ReadonlyMap<string, number>,
): { pinned: SidebarAgent[]; rest: SidebarAgent[] } {
  const band = (row: SidebarAgent) => (attention.has(row.key) ? 0 : 1);
  const spokeAt = (row: SidebarAgent) => row.chat?.last_ts ?? "";
  return {
    pinned: agents.filter((row) => pinned.has(row.key)),
    rest: agents
      .filter((row) => !pinned.has(row.key))
      .map((row, index) => ({ row, index }))
      .sort(
        (left, right) =>
          band(left.row) - band(right.row) ||
          spokeAt(right.row).localeCompare(spokeAt(left.row)) ||
          left.index - right.index,
      )
      .map(({ row }) => row),
  };
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
