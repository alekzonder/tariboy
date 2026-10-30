import { describe, expect, it } from "vitest";
import { activityWindow, closestBucket } from "./activityPeriod";

const now = new Date("2026-10-18T12:00:00Z");

describe("activityWindow", () => {
  it("defaults to the last day", () => {
    expect(activityWindow(new URLSearchParams(), now)).toEqual({
      period: "day",
      since: "2026-10-17T12:00:00.000Z",
      until: "2026-10-18T12:00:00.000Z",
      customSince: "",
      customUntil: "",
      error: "",
    });
  });

  it("slides hour and week windows back from now", () => {
    expect(activityWindow(new URLSearchParams("period=hour"), now).since).toBe(
      "2026-10-18T11:00:00.000Z",
    );
    expect(activityWindow(new URLSearchParams("period=week"), now).since).toBe(
      "2026-10-11T12:00:00.000Z",
    );
  });

  it("reads a custom range as local wall-clock times and runs an open end to now", () => {
    const local = "2026-10-01T09:30";
    const window = activityWindow(new URLSearchParams(`period=custom&since=${local}`), now);
    expect(window).toMatchObject({
      period: "custom",
      since: new Date(local).toISOString(),
      until: now.toISOString(),
      customSince: local,
    });
  });

  it("reports an invalid or reversed custom range", () => {
    expect(activityWindow(new URLSearchParams("period=custom&since=nope"), now).error).toBe(
      "Enter a valid start time.",
    );
    expect(
      activityWindow(
        new URLSearchParams("period=custom&since=2026-10-02T00:00&until=2026-10-01T00:00"),
        now,
      ).error,
    ).toBe("The start must be before the end.");
  });

  it("falls back to the day for an unknown period", () => {
    expect(activityWindow(new URLSearchParams("period=year"), now).period).toBe("day");
  });

  it("runs today from local midnight until now", () => {
    const midnight = new Date(now);
    midnight.setHours(0, 0, 0, 0);
    expect(activityWindow(new URLSearchParams("period=today"), now)).toMatchObject({
      period: "today",
      since: midnight.toISOString(),
      until: now.toISOString(),
    });
  });

  it("slides the month window back 30 days", () => {
    expect(activityWindow(new URLSearchParams("period=month"), now).since).toBe(
      "2026-09-18T12:00:00.000Z",
    );
  });

  it("uses the caller's default period and still reads hour and day links", () => {
    expect(activityWindow(new URLSearchParams(), now, "today").period).toBe("today");
    expect(activityWindow(new URLSearchParams("period=year"), now, "today").period).toBe("today");
    expect(activityWindow(new URLSearchParams("period=hour"), now, "today").period).toBe("hour");
    expect(activityWindow(new URLSearchParams("period=day"), now, "today").since).toBe(
      "2026-10-17T12:00:00.000Z",
    );
  });
});

describe("closestBucket", () => {
  it("widens the bucket with the span", () => {
    expect(closestBucket(3600e3)).toBe("5m");
    expect(closestBucket(24 * 3600e3)).toBe("1h");
    expect(closestBucket(7 * 24 * 3600e3)).toBe("1d");
  });
});
