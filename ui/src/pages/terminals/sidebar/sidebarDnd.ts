import { closestCenter, pointerWithin, type Announcements } from "@dnd-kit/core";
import type { HostAgents } from "@/lib/aggregate";
import { hostAgents, teamNames } from "./sidebarModel";

export type ReorderKind = "servers" | "groups" | "agents";
export type DragIdentity = [ReorderKind, string, string, string?, string?];

export const dragId = (...identity: DragIdentity) => JSON.stringify(identity);
export const identityFor = (id: string | number) => JSON.parse(String(id)) as DragIdentity;
export const sameScope = (left: DragIdentity, right: DragIdentity) =>
  left[0] === right[0] && left[1] === right[1] && left[3] === right[3];

function move(ids: string[], active: string, over: string): string[] {
  const from = ids.indexOf(active);
  const to = ids.indexOf(over);
  if (from < 0 || to < 0 || from === to) return ids;
  const next = [...ids];
  next.splice(to, 0, next.splice(from, 1)[0]);
  return next;
}

/** The order a finished drag asks to save, or nothing when the drop moves
 *  nothing: no target, the row itself, another scope or an unknown server. */
export function reorderFromDrag(
  hosts: HostAgents[],
  active: string | number,
  over: string | number | undefined,
): { kind: ReorderKind; hostId: string; ids: string[] } | undefined {
  if (over === undefined || active === over) return undefined;
  const source = identityFor(active);
  const target = identityFor(over);
  if (!sameScope(source, target)) return undefined;
  const [kind, hostId, name] = source;
  if (kind === "servers") {
    return {
      kind,
      hostId: "",
      ids: move(
        hosts.map((host) => host.host.id),
        name,
        target[2],
      ),
    };
  }
  const host = hosts.find((entry) => entry.host.id === hostId);
  if (!host) return undefined;
  const agents = hostAgents(host);
  const current = kind === "groups" ? teamNames(host, agents) : agents.map((row) => row.agent.name);
  return { kind, hostId, ids: move(current, name, target[2]) };
}

const readableDragId = (id: string | number) => {
  const [kind, , name, , label] = identityFor(id);
  return `${kind === "groups" ? "team" : kind.slice(0, -1)} ${label ?? name}`;
};

export const rowCollision: typeof closestCenter = (args) => {
  const source = identityFor(args.active.id);
  const droppableContainers = args.droppableContainers.filter((container) =>
    sameScope(source, identityFor(container.id)),
  );
  const compatibleArgs = { ...args, droppableContainers };
  return args.pointerCoordinates ? pointerWithin(compatibleArgs) : closestCenter(compatibleArgs);
};

export const sidebarAnnouncements: Announcements = {
  onDragStart: ({ active }) => `Picked up ${readableDragId(active.id)}.`,
  onDragOver: ({ active, over }) =>
    over
      ? `${readableDragId(active.id)} is over ${readableDragId(over.id)}.`
      : `${readableDragId(active.id)} is not over a compatible row.`,
  onDragEnd: ({ active, over }) =>
    over && active.id !== over.id
      ? `Moved ${readableDragId(active.id)} to ${readableDragId(over.id)}.`
      : `Did not move ${readableDragId(active.id)}.`,
  onDragCancel: ({ active }) => `Cancelled moving ${readableDragId(active.id)}.`,
};
