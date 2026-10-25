import { describe, expect, it, vi } from "vitest";
import type { AgentScript, AgentScriptRun } from "@/lib/api";
import {
  canRerun,
  isRunActive,
  logNeedsRefresh,
  nextGeneration,
  runDuration,
  scriptsNeedPolling,
  toggledSet,
  when,
} from "./scriptsModel";

vi.mock("@/lib/time", () => ({ fmtDateTime: (value: string) => `fmt:${value}` }));

const run: AgentScriptRun = {
  id: "r1",
  script_id: "s1",
  agent: "alpha",
  status: "succeeded",
  created_at: "2026-08-20T10:00:00Z",
  started_at: "",
  finished_at: "",
  log_path: "",
  cancel_requested: false,
};
const definition: AgentScript = {
  id: "s1",
  agent: "alpha",
  name: "n",
  description: "d",
  command: "c",
  mode: "once",
  interval_seconds: 0,
  state: "completed",
  created_at: "2026-08-20T10:00:00Z",
  next_run_at: "",
};

describe("isRunActive", () => {
  it.each([
    ["pending", true],
    ["running", true],
    ["failed", false],
    ["", false],
    [undefined, false],
  ])("%j -> %j", (status, expected) => {
    expect(isRunActive(status)).toBe(expected);
  });
});

describe("scriptsNeedPolling", () => {
  it("polls while a script is active or its latest run is active", () => {
    expect(scriptsNeedPolling([definition])).toBe(false);
    expect(scriptsNeedPolling([{ ...definition, state: "active" }])).toBe(true);
    expect(scriptsNeedPolling([{ ...definition, latest_run: { ...run, status: "running" } }])).toBe(
      true,
    );
  });
});

describe("canRerun", () => {
  it.each<[string, Partial<AgentScript>, boolean]>([
    ["a completed script", {}, true],
    ["a cancelled script", { state: "cancelled" }, false],
    ["an active one-shot", { state: "active" }, false],
    ["an idle active schedule", { state: "active", mode: "every" }, true],
    [
      "a busy active schedule",
      { state: "active", mode: "every", latest_run: { ...run, status: "pending" } },
      false,
    ],
  ])("%s", (_, patch, expected) => {
    expect(canRerun({ ...definition, ...patch })).toBe(expected);
  });
});

describe("logNeedsRefresh", () => {
  const expanded = new Set(["r1"]);
  it("skips collapsed runs", () => {
    expect(logNeedsRefresh({ run, expanded: new Set(), forceLogs: true })).toBe(false);
  });
  it("refreshes when forced, active, or previously active", () => {
    expect(logNeedsRefresh({ run, expanded, forceLogs: false })).toBe(false);
    expect(logNeedsRefresh({ run, expanded, forceLogs: true })).toBe(true);
    expect(
      logNeedsRefresh({ run: { ...run, status: "running" }, expanded, forceLogs: false }),
    ).toBe(true);
    expect(logNeedsRefresh({ run, expanded, forceLogs: false, previousStatus: "running" })).toBe(
      true,
    );
  });
});

describe("toggledSet", () => {
  it("adds a closed id and removes an open one without mutating", () => {
    const current = new Set(["a"]);
    expect([...toggledSet(current, "b", false)]).toEqual(["a", "b"]);
    expect([...toggledSet(current, "a", true)]).toEqual([]);
    expect([...current]).toEqual(["a"]);
  });
});

describe("nextGeneration", () => {
  it("counts per key", () => {
    const generations: Record<string, number> = {};
    expect(nextGeneration(generations, "a")).toBe(1);
    expect(nextGeneration(generations, "a")).toBe(2);
    expect(nextGeneration(generations, "b")).toBe(1);
  });
});

describe("when", () => {
  it("formats a time or shows a dash", () => {
    expect(when("2026-08-20T10:00:00Z")).toBe("fmt:2026-08-20T10:00:00Z");
    expect(when(undefined)).toBe("—");
  });
});

describe("runDuration", () => {
  it.each<[Partial<AgentScriptRun>, string]>([
    [{}, "—"],
    [{ started_at: "2026-08-20T10:00:00Z" }, "—"],
    [{ started_at: "2026-08-20T10:00:00Z", finished_at: "2026-08-20T10:00:02.5Z" }, "2.5s"],
    [{ started_at: "2026-08-20T10:00:02Z", finished_at: "2026-08-20T10:00:00Z" }, "—"],
  ])("%j -> %s", (patch, expected) => {
    expect(runDuration({ ...run, ...patch })).toBe(expected);
  });
});
