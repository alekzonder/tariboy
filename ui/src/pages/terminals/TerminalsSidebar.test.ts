import { describe, expect, it } from "vitest";
import { rowCollision, sidebarAnnouncements } from "./sidebarDnd";

const rect = (top: number) => ({
  left: 0,
  top,
  width: 100,
  height: 20,
  right: 100,
  bottom: top + 20,
});

function collisionArgs(pointerCoordinates: { x: number; y: number } | null) {
  const ids = [
    '["groups","host","team-a"]',
    '["agents","host","agent-b","team-b"]',
    '["agents","host","agent-a2","team-a"]',
  ];
  const rectangles = [rect(0), rect(30), rect(90)];
  return {
    active: { id: '["agents","host","agent-a1","team-a"]' },
    collisionRect: rect(25),
    droppableRects: new Map(ids.map((id, index) => [id, rectangles[index]])),
    droppableContainers: ids.map((id) => ({ id, disabled: false })),
    pointerCoordinates,
  } as Parameters<typeof rowCollision>[0];
}

describe("sidebar row collision", () => {
  it("ignores rows outside the active agent scope for keyboard movement", () => {
    expect(rowCollision(collisionArgs(null))[0]?.id)
      .toBe('["agents","host","agent-a2","team-a"]');
  });

  it("does not snap a pointer outside compatible rows", () => {
    expect(rowCollision(collisionArgs({ x: 50, y: 40 }))).toEqual([]);
  });

  it("announces readable row names instead of encoded drag ids", () => {
    const announcement = sidebarAnnouncements.onDragStart({
      active: { id: '["agents","host","agent-a1","team-a"]' },
    } as Parameters<typeof sidebarAnnouncements.onDragStart>[0]);
    expect(announcement).toBe("Picked up agent agent-a1.");
  });
});
