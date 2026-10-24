import { describe, expect, it } from "vitest";
import type { HostAgents } from "@/lib/aggregate";
import type { AgentSummary } from "@/lib/types";
import { dragId, reorderFromDrag, rowCollision, sidebarAnnouncements } from "./sidebarDnd";

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
    expect(rowCollision(collisionArgs(null))[0]?.id).toBe('["agents","host","agent-a2","team-a"]');
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

  it("announces the server label and does not claim a no-op moved", () => {
    const id = '["servers","","",null,"This daemon (local)"]';
    expect(sidebarAnnouncements.onDragStart({ active: { id } } as never)).toBe(
      "Picked up server This daemon (local).",
    );
    expect(sidebarAnnouncements.onDragEnd({ active: { id }, over: { id } } as never)).toBe(
      "Did not move server This daemon (local).",
    );
  });
});

const agent = (name: string, group: string | null = null): AgentSummary => ({
  name,
  image: "img:latest",
  state: "running",
  harness: "claude",
  group,
});

const hosts: HostAgents[] = [
  {
    host: { id: "", label: "local" },
    agents: [agent("alpha", "red"), agent("beta"), agent("gamma", "blue")],
    groups: [{ name: "idle", lead: "", members: 0 }],
    sidebarOrder: { version: 1, groups: ["blue"], agents: ["beta"] },
  },
  { host: { id: "prod", label: "prod" }, agents: [] },
];

describe("reorder from a drag", () => {
  it("moves a server among all servers", () => {
    expect(
      reorderFromDrag(hosts, dragId("servers", "", "prod"), dragId("servers", "", "")),
    ).toEqual({ kind: "servers", hostId: "", ids: ["prod", ""] });
  });

  it("moves a team inside its server's saved team order", () => {
    expect(
      reorderFromDrag(hosts, dragId("groups", "", "red"), dragId("groups", "", "blue")),
    ).toEqual({ kind: "groups", hostId: "", ids: ["red", "blue", "idle"] });
  });

  it("moves an agent inside its server's saved agent order", () => {
    expect(
      reorderFromDrag(hosts, dragId("agents", "", "gamma", ""), dragId("agents", "", "beta", "")),
    ).toEqual({ kind: "agents", hostId: "", ids: ["gamma", "beta", "alpha"] });
  });

  it("ignores a drop on itself, outside the scope or on an unknown server", () => {
    const id = dragId("agents", "", "beta", "");
    expect(reorderFromDrag(hosts, id, id)).toBeUndefined();
    expect(reorderFromDrag(hosts, id, dragId("agents", "", "alpha", "red"))).toBeUndefined();
    expect(
      reorderFromDrag(hosts, dragId("agents", "x", "a", ""), dragId("agents", "x", "b", "")),
    ).toBeUndefined();
    expect(reorderFromDrag(hosts, id, undefined)).toBeUndefined();
  });
});
