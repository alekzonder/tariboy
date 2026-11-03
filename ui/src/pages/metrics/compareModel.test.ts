import { describe, expect, it } from "vitest";
import { UNKNOWN_REASON } from "@/lib/metricsFormat";
import { metricsSlice } from "@/test/fixtures/apiFixtures";
import { changeOf, compareRows, fmtCell, smallSample } from "./compareModel";

const slice = (over: Partial<ReturnType<typeof metricsSlice>["tasks"]> = {}, cost = 12.4) => {
  const s = metricsSlice();
  s.tasks = { ...s.tasks, ...over };
  s.usage = { ...s.usage, cost_usd: cost };
  return s;
};

describe("compareRows", () => {
  it("lists every metric with its field and better direction", () => {
    const rows = compareRows(slice(), slice());
    expect(rows.map((r) => [r.label, r.better])).toEqual([
      ["Tasks done", null],
      ["Lead time, median", "lower"],
      ["Lead time, average", "lower"],
      ["Cost per task", "lower"],
      ["Runs per task", "lower"],
      ["Tokens per task", "lower"],
      ["Rework rounds per task", "lower"],
      ["Rejected advances per task", "lower"],
      ["Questions per task", "lower"],
      ["Quality: Image and process", "higher"],
      ["Quality: Role", "higher"],
      ["Quality: Task fit", "higher"],
      ["Runs in period", null],
      ["Cost in period", null],
    ]);
    const values = Object.fromEntries(rows.map((r) => [r.label, fmtCell(r, "a")]));
    expect(values).toEqual({
      "Tasks done": "7",
      "Lead time, median": "12h 30m",
      "Lead time, average": "14h 20m",
      "Cost per task": "$1.37",
      "Runs per task": "2.3",
      "Tokens per task": "1.2M",
      "Rework rounds per task": "0.4",
      "Rejected advances per task": "0.1",
      "Questions per task": "0.6",
      "Quality: Image and process": "82 · 12 scores",
      "Quality: Role": "74 · 11 scores",
      "Quality: Task fit": "90 · 10 scores",
      "Runs in period": "23",
      "Cost in period": "$12.40",
    });
  });

  it("puts one Quality row in place of the axes when neither version has scores", () => {
    const rows = compareRows(slice({ quality: null }), slice({ quality: null }));
    const quality = rows.filter((r) => r.label.startsWith("Quality"));
    expect(quality).toHaveLength(1);
    expect(quality[0]).toMatchObject({ label: "Quality", note: "No quality scores yet" });
  });

  it("leaves a missing version's values unknown", () => {
    const rows = compareRows(slice(), undefined);
    expect(rows.every((r) => r.b === null)).toBe(true);
    expect(changeOf(rows[0])).toBeNull();
  });

  it("gives each unknown value its own reason", () => {
    const a = slice({ lead_time_median_s: null, avg: undefined });
    a.tasks.quality = { ...a.tasks.quality!, role: null };
    const rows = compareRows(a, undefined);
    const reasons = (id: string) => {
      const r = rows.find((row) => row.id === id)!;
      return [r.reasonA, r.reasonB];
    };
    const missing = "No data for this version in this period.";
    expect(reasons("lead_median")).toEqual([UNKNOWN_REASON.leadTime, missing]);
    expect(reasons("cost_task")).toEqual([UNKNOWN_REASON.perTask, missing]);
    expect(reasons("quality_role")).toEqual([UNKNOWN_REASON.qualityAxis, missing]);
    expect(reasons("tasks_done")).toEqual([undefined, missing]);
  });
});

describe("changeOf", () => {
  const row = (label: string, a: number, b: number) =>
    compareRows(slice(), slice())
      .map((r) => (r.label === label ? { ...r, a, b } : r))
      .find((r) => r.label === label)!;

  it("shows B minus A with a percent of A and a verdict", () => {
    expect(changeOf(row("Cost per task", 2.33, 2.75))).toEqual({
      text: "+$0.42 (+18%)",
      direction: "up",
      verdict: "worse",
    });
    expect(changeOf(row("Lead time, median", 54_600, 43_200))).toEqual({
      text: "−3h 10m (−21%)",
      direction: "down",
      verdict: "better",
    });
    expect(changeOf(row("Quality: Role", 70, 80))).toMatchObject({ verdict: "better" });
    expect(changeOf(row("Runs per task", 2, 2))).toEqual({
      text: "0 (0%)",
      direction: "none",
      verdict: "no change",
    });
  });

  it("gives the arrow the sign of B − A, whatever the verdict", () => {
    const pick = (label: string, a: number, b: number) => {
      const change = changeOf(row(label, a, b))!;
      return [change.direction, change.verdict];
    };
    expect(pick("Quality: Role", 70, 80)).toEqual(["up", "better"]);
    expect(pick("Quality: Role", 80, 70)).toEqual(["down", "worse"]);
    expect(pick("Cost per task", 2, 3)).toEqual(["up", "worse"]);
    expect(pick("Cost per task", 3, 2)).toEqual(["down", "better"]);
    expect(pick("Cost per task", 2, 2)).toEqual(["none", "no change"]);
  });

  it("shows a zero cost change as dollars", () => {
    expect(changeOf(row("Cost per task", 1.37, 1.37))).toEqual({
      text: "$0.00 (0%)",
      direction: "none",
      verdict: "no change",
    });
  });

  it("gives no percent from zero and no verdict for a count without direction", () => {
    expect(changeOf(row("Runs per task", 0, 0.5))).toEqual({
      text: "+0.5",
      direction: "up",
      verdict: "worse",
    });
    expect(changeOf(row("Tasks done", 4, 6))).toEqual({
      text: "+2 (+50%)",
      direction: "up",
      verdict: null,
    });
  });
});

describe("smallSample", () => {
  it("warns below five done tasks in either version", () => {
    expect(smallSample(slice({ tasks_done: 2 }), slice({ tasks_done: 3 }))).toBe(
      "Few completed tasks (A: 2, B: 3). The change can be random.",
    );
    expect(smallSample(slice({ tasks_done: 5 }), slice({ tasks_done: 9 }))).toBeNull();
    expect(smallSample(slice({ tasks_done: 5 }), undefined)).toBeNull();
  });
});
