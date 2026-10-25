import { describe, expect, it } from "vitest";
import type { AgentStatus, AgentView } from "@/lib/api";
import { countDirtyByBlock, isStopped } from "./configurationPageModel";

describe("countDirtyByBlock", () => {
  it("counts dirty keys per owning block", () => {
    const counts = countDirtyByBlock(["interval", "timeout", "budget-hour"]);
    expect(Object.fromEntries(counts)).toEqual({ loop: 2, budgets: 1 });
  });

  it("is empty without dirty keys", () => {
    expect(countDirtyByBlock([]).size).toBe(0);
  });
});

describe("isStopped", () => {
  const status = (state: string) => ({ state }) as AgentStatus;
  const view = (fields: Partial<AgentView>) => fields as AgentView;

  it.each<[string, AgentStatus | null, AgentView | null, boolean]>([
    ["a stopped status", status("stopped"), view({ enabled: true }), true],
    ["a running status over a disabled view", status("running"), view({ enabled: false }), false],
    ["a disabled view without status", null, view({ enabled: false }), true],
    ["an enabled view without status", null, view({ enabled: true, state: "stopped" }), false],
    ["a stopped view without the switch", null, view({ state: "stopped" }), true],
    ["a running view without the switch", null, view({ state: "running" }), false],
    ["nothing loaded", null, null, false],
  ])("reads %s", (_, agentStatus, agentView, expected) => {
    expect(isStopped(agentStatus, agentView)).toBe(expected);
  });
});
