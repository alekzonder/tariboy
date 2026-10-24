import { describe, expect, it } from "vitest";
import { usageParams } from "./usageParams";

const now = Date.parse("2026-07-12T12:00:00Z");
const base = {
  preset: "24h" as const,
  customSince: "",
  customUntil: "",
  groupBy: "task" as const,
  now,
};

describe("usageParams", () => {
  it("uses the controlled period and derives the bucket from its span", () => {
    expect(
      usageParams({
        ...base,
        period: { since: "2026-07-12T11:00:00Z", until: "2026-07-12T12:00:00Z" },
      }),
    ).toEqual({
      since: "2026-07-12T11:00:00Z",
      until: "2026-07-12T12:00:00Z",
      group_by: "task",
      bucket: "5m",
    });
  });

  it("treats an open controlled period as ending now", () => {
    expect(usageParams({ ...base, period: { since: "2026-07-10T12:00:00Z", until: "" } })).toEqual({
      since: "2026-07-10T12:00:00Z",
      until: "",
      group_by: "task",
      bucket: "1d",
    });
  });

  it("falls back to an hourly bucket for a controlled period without a start", () => {
    expect(usageParams({ ...base, period: { since: "", until: "" } }).bucket).toBe("1h");
  });

  it("converts a custom range to UTC and buckets by its span", () => {
    const since = "2026-07-12T08:00";
    const until = "2026-07-12T10:00";
    expect(
      usageParams({ ...base, preset: "custom", customSince: since, customUntil: until }),
    ).toEqual({
      since: new Date(since).toISOString(),
      until: new Date(until).toISOString(),
      group_by: "task",
      bucket: "5m",
    });
  });

  it("treats an open custom end as now", () => {
    const since = new Date(now - 3 * 24 * 3600e3);
    const local = new Date(since.getTime() - since.getTimezoneOffset() * 60e3)
      .toISOString()
      .slice(0, 16);
    expect(usageParams({ ...base, preset: "custom", customSince: local }).bucket).toBe("1d");
  });

  it("keeps the hourly bucket for an empty custom range", () => {
    expect(usageParams({ ...base, preset: "custom" })).toEqual({
      since: "",
      until: "",
      group_by: "task",
      bucket: "1h",
    });
  });

  it.each([
    ["1h", 3600e3, "5m"],
    ["5h", 5 * 3600e3, "15m"],
    ["24h", 24 * 3600e3, "1h"],
    ["7d", 7 * 24 * 3600e3, "1d"],
  ] as const)("slides the %s preset back from now", (preset, ms, bucket) => {
    expect(usageParams({ ...base, preset })).toEqual({
      since: new Date(now - ms).toISOString(),
      group_by: "task",
      bucket,
    });
  });
});
