import type { TerminalDock } from "./workspaceModel";
import type { TerminalIdentity } from "./workspaceState";

export const DRAG_THRESHOLD_PX = 5;

export type WorkspaceDragSource =
  | { kind: "external"; identity: TerminalIdentity }
  | { kind: "internal"; identity: TerminalIdentity; nodeId: string };

export interface PointerPoint {
  x: number;
  y: number;
}

export interface PaneRect {
  left: number;
  top: number;
  width: number;
  height: number;
}

export interface WorkspaceDropTarget {
  nodeId: string | null;
  dock: TerminalDock;
  preview: PaneRect;
}

export function crossedDragThreshold(start: PointerPoint, current: PointerPoint): boolean {
  return Math.hypot(current.x - start.x, current.y - start.y) >= DRAG_THRESHOLD_PX;
}

export function dropTargetForPoint(
  nodeId: string,
  rect: PaneRect,
  clientX: number,
  clientY: number,
): WorkspaceDropTarget {
  const distances: Array<{ dock: TerminalDock; distance: number }> = [
    { dock: "left", distance: Math.abs(clientX - rect.left) / rect.width },
    { dock: "right", distance: Math.abs(rect.left + rect.width - clientX) / rect.width },
    { dock: "top", distance: Math.abs(clientY - rect.top) / rect.height },
    { dock: "bottom", distance: Math.abs(rect.top + rect.height - clientY) / rect.height },
  ];
  let dock = distances[0].dock;
  let nearest = distances[0].distance;
  for (const candidate of distances.slice(1)) {
    if (candidate.distance < nearest) {
      dock = candidate.dock;
      nearest = candidate.distance;
    }
  }

  const horizontal = dock === "left" || dock === "right";
  const width = horizontal ? rect.width / 2 : rect.width;
  const height = horizontal ? rect.height : rect.height / 2;
  const left = dock === "right" ? rect.left + width : rect.left;
  const top = dock === "bottom" ? rect.top + height : rect.top;

  return {
    nodeId,
    dock,
    preview: { left, top, width, height },
  };
}
