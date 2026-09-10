import {
  closestCenter,
  pointerWithin,
  type Announcements,
} from "@dnd-kit/core";

export type ReorderKind = "servers" | "groups" | "agents";
export type DragIdentity = [ReorderKind, string, string, string?];

export const dragId = (...identity: DragIdentity) => JSON.stringify(identity);
export const identityFor = (id: string | number) => JSON.parse(String(id)) as DragIdentity;
export const sameScope = (left: DragIdentity, right: DragIdentity) =>
  left[0] === right[0] && left[1] === right[1] && left[3] === right[3];

const readableDragId = (id: string | number) => {
  const [kind, , name] = identityFor(id);
  return `${kind === "groups" ? "team" : kind.slice(0, -1)} ${name}`;
};

export const rowCollision: typeof closestCenter = (args) => {
  const source = identityFor(args.active.id);
  const droppableContainers = args.droppableContainers.filter((container) =>
    sameScope(source, identityFor(container.id))
  );
  const compatibleArgs = { ...args, droppableContainers };
  return args.pointerCoordinates
    ? pointerWithin(compatibleArgs)
    : closestCenter(compatibleArgs);
};

export const sidebarAnnouncements: Announcements = {
  onDragStart: ({ active }) => `Picked up ${readableDragId(active.id)}.`,
  onDragOver: ({ active, over }) => over
    ? `${readableDragId(active.id)} is over ${readableDragId(over.id)}.`
    : `${readableDragId(active.id)} is not over a compatible row.`,
  onDragEnd: ({ active, over }) => over
    ? `Moved ${readableDragId(active.id)} to ${readableDragId(over.id)}.`
    : `Did not move ${readableDragId(active.id)}.`,
  onDragCancel: ({ active }) => `Cancelled moving ${readableDragId(active.id)}.`,
};
