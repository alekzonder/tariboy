import { describe, expect, it } from "vitest";
import type { HostAgents } from "@/lib/aggregate";
import { agentRow } from "@/test/fixtures/apiFixtures";
import { elapsed, recentAgents, runningAgents } from "./homeModel";

const agent = (name: string, over: Record<string, string> = {}) =>
  agentRow({ name, image: "img", state: "idle", harness: "claude", ...over });

const hosts: HostAgents[] = [
  {
    host: { id: "", label: "Local" },
    agents: [
      agent("a", { last_iteration_at: "2026-10-30T10:00:00Z" }),
      agent("b"),
      agent("c", {
        state: "running",
        current_iteration_started_at: "2026-10-30T12:00:00Z",
        last_iteration_at: "2026-10-30T12:00:00Z",
      }),
    ],
    sidebarOrder: { version: 1, groups: [], agents: ["b", "a", "c"] },
  },
  {
    host: { id: "d1", label: "prod" },
    agents: [agent("d", { last_iteration_at: "2026-10-30T11:00:00Z" }), agent("e")],
  },
];

describe("recentAgents", () => {
  it("puts the newest iteration first, then agents without one in sidebar order", () => {
    expect(recentAgents(hosts, 8).map((row) => row.agent.name)).toEqual(["c", "d", "a", "b", "e"]);
  });

  it("keeps the first n", () => {
    expect(recentAgents(hosts, 2).map((row) => row.agent.name)).toEqual(["c", "d"]);
  });
});

describe("runningAgents", () => {
  it("lists only running agents", () => {
    expect(runningAgents(hosts).map((row) => row.agent.name)).toEqual(["c"]);
  });
});

describe("elapsed", () => {
  const now = new Date("2026-10-30T12:30:00Z");
  it.each([
    ["2026-10-30T12:29:40Z", "under a minute"],
    ["2026-10-30T12:18:00Z", "12 min"],
    ["2026-10-30T09:30:00Z", "3 h"],
    ["2026-10-28T12:30:00Z", "2 d"],
    ["", ""],
    ["not a time", ""],
  ])("reads %s as %s", (start, text) => {
    expect(elapsed(start, now)).toBe(text);
  });
});
