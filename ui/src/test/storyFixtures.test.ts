import { describe, expect, it } from "vitest";

import * as agents from "@/fixtures/agents";
import * as images from "@/fixtures/images";
import * as metrics from "@/fixtures/metrics";
import * as operations from "@/fixtures/operations";
import * as tasks from "@/fixtures/tasks";
import { NOW, hoursAgo, minutesAgo } from "@/fixtures/time";

import * as apiFixtures from "./fixtures/apiFixtures";

const moved = {
  agentBudget: agents,
  agentRow: agents,
  agentView: agents,
  imageSummary: images,
  operation: operations,
  operationStarted: operations,
  taskItem: tasks,
  metricsSlice: metrics,
  metricsReport: metrics,
  taskMetrics: metrics,
} as const;

type EdgeSet = { empty: unknown[]; one: unknown[]; many: unknown[]; longText: unknown[] };
const sets: Record<string, EdgeSet> = {
  agentRows: agents.agentRows,
  taskItems: tasks.taskItems,
  imageSummaries: images.imageSummaries,
  operations: operations.operations,
  metricsSlices: metrics.metricsSlices,
};

const labelOf = (item: unknown): string => {
  const o = item as { name?: string; title?: string };
  return o.name ?? o.title ?? "";
};

const isoDates = (value: unknown, found: string[] = []): string[] => {
  if (typeof value === "string" && /^\d{4}-\d{2}-\d{2}T/.test(value)) found.push(value);
  else if (Array.isArray(value)) value.forEach((v) => isoDates(v, found));
  else if (value && typeof value === "object")
    Object.values(value).forEach((v) => isoDates(v, found));
  return found;
};

describe("story fixtures", () => {
  it("keeps the factories of apiFixtures", () => {
    for (const [name, module] of Object.entries(moved)) {
      expect(apiFixtures[name as keyof typeof apiFixtures], name).toBe(
        module[name as keyof typeof module],
      );
    }
  });

  it("applies the overrides over complete defaults", () => {
    const base = agents.agentRow();
    const row = agents.agentRow({ name: "builder", state: "running" });
    expect(row.name).toBe("builder");
    expect(row.state).toBe("running");
    expect(Object.keys(row).sort()).toEqual(Object.keys(base).sort());
  });

  it("has empty, one, many and longText sets", () => {
    for (const [name, set] of Object.entries(sets)) {
      expect(set.empty, name).toHaveLength(0);
      expect(set.one, name).toHaveLength(1);
      expect(set.many, name).toHaveLength(50);
      expect(labelOf(set.longText[0]).length, name).toBeGreaterThanOrEqual(60);
    }
  });

  it("takes every date from NOW", () => {
    expect(minutesAgo(5)).toBe("2026-11-05T09:55:00.000Z");
    expect(hoursAgo(2)).toBe("2026-11-05T08:00:00.000Z");
    const defaults = [
      agents.agentRow(),
      agents.agentView(),
      images.imageSummary(),
      operations.operation(),
      tasks.taskItem(),
      metrics.metricsReport(),
      metrics.taskMetrics(),
    ];
    const dates = isoDates(defaults);
    expect(dates.length).toBeGreaterThan(0);
    for (const date of dates) expect(Date.parse(date)).toBeLessThanOrEqual(Date.parse(NOW));
  });
});
