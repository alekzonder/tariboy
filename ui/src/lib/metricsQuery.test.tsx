import { describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import type { PropsWithChildren } from "react";
import {
  METRICS_PERIODS,
  agentMetricsQuery,
  compareQuery,
  defaultPair,
  metricsFilters,
  metricsView,
  metricsWindow,
  olderVersion,
  parseCompare,
  periodParams,
  shorterThanQuarter,
  useMetricsWindow,
} from "./metricsQuery";

const NOW = new Date("2026-11-02T14:05:00.000Z");
const q = (s: string) => new URLSearchParams(s);
const DAY = 24 * 3600e3;

describe("metrics period", () => {
  it("offers Week, Month, Quarter and Custom", () => {
    expect(METRICS_PERIODS.map((p) => p.label)).toEqual(["Week", "Month", "Quarter", "Custom"]);
  });

  it("defaults to the 30 days before now and sends RFC 3339 bounds", () => {
    const w = metricsWindow(q(""), NOW);
    expect(w.period).toBe("month");
    expect(w.until).toBe("2026-11-02T14:05:00.000Z");
    expect(new Date(w.until).getTime() - new Date(w.since).getTime()).toBe(30 * DAY);
  });

  it("reads week and quarter", () => {
    const week = metricsWindow(q("period=week"), NOW);
    expect(new Date(week.until).getTime() - new Date(week.since).getTime()).toBe(7 * DAY);
    const quarter = metricsWindow(q("period=quarter"), NOW);
    expect(quarter.period).toBe("quarter");
    expect(new Date(quarter.until).getTime() - new Date(quarter.since).getTime()).toBe(90 * DAY);
  });

  it("falls back to Month for a period the Metrics bar does not offer", () => {
    expect(metricsWindow(q("period=hour"), NOW).period).toBe("month");
  });

  it("reads a custom range and reports an unusable one without bounds", () => {
    const ok = metricsWindow(q("period=custom&since=2026-10-01T10:00"), NOW);
    expect(ok.since).toBe(new Date("2026-10-01T10:00").toISOString());
    expect(ok.until).toBe(NOW.toISOString());
    const missing = metricsWindow(q("period=custom"), NOW);
    expect(missing).toMatchObject({ since: "", until: "", error: "Enter a valid start time." });
    const reversed = metricsWindow(
      q("period=custom&since=2026-10-02T10:00&until=2026-10-01T10:00"),
      NOW,
    );
    expect(reversed.error).toBe("The start must be before the end.");
  });

  it("knows when a period is shorter than a quarter", () => {
    expect(shorterThanQuarter(metricsWindow(q("period=month"), NOW))).toBe(true);
    expect(shorterThanQuarter(metricsWindow(q("period=quarter"), NOW))).toBe(false);
  });

  it("keeps the window still until the period changes or refresh is asked", () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(NOW);
    const wrapper = ({ children }: PropsWithChildren) => (
      <MemoryRouter initialEntries={["/metrics?period=week&agent=a"]}>{children}</MemoryRouter>
    );
    const { result, rerender } = renderHook(
      () => ({ ...useMetricsWindow(), search: useLocation().search }),
      { wrapper },
    );
    expect(result.current.window.until).toBe(NOW.toISOString());
    const later = new Date(NOW.getTime() + 5 * 60e3);
    vi.setSystemTime(later);
    rerender();
    expect(result.current.window.until).toBe(NOW.toISOString());
    act(() => result.current.refresh());
    expect(result.current.window.until).toBe(later.toISOString());
    act(() => result.current.setPeriod("quarter"));
    expect(result.current.search).toBe("?period=quarter&agent=a");
    expect(result.current.window.period).toBe("quarter");
    act(() => result.current.setCustom("since", "2026-10-01T10:00"));
    expect(result.current.search).toContain("since=2026-10-01T10%3A00");
    vi.useRealTimers();
  });
});

describe("metrics query", () => {
  it("reads the view, with Agents by default", () => {
    expect(metricsView(q(""))).toBe("agents");
    expect(metricsView(q("view=images"))).toBe("images");
    expect(metricsView(q("view=workflows"))).toBe("workflows");
    expect(metricsView(q("view=nope"))).toBe("agents");
  });

  it("reads the filters as the URL gives them", () => {
    expect(metricsFilters(q("agent=a&image=dev:1.0&workflow=none"))).toEqual({
      agent: "a",
      image: "dev:1.0",
      workflow: "none",
    });
    expect(metricsFilters(q(""))).toEqual({ agent: "", image: "", workflow: "" });
  });

  it("keeps the host and period in links to another metrics screen", () => {
    const w = metricsWindow(q("period=custom&since=2026-10-01T10:00"), NOW);
    expect(periodParams(q("host=h1&agent=x"), w).toString()).toBe(
      "host=h1&period=custom&since=2026-10-01T10%3A00",
    );
    expect(periodParams(q(""), metricsWindow(q(""), NOW)).toString()).toBe("period=month");
  });

  it("opens an agent's Metrics view on the same window", () => {
    const month = metricsWindow(q("period=month"), NOW);
    expect(agentMetricsQuery(month)).toBe("view=metrics&period=month");
    const quarter = metricsWindow(q("period=quarter"), NOW);
    expect(agentMetricsQuery(quarter)).toBe(
      `view=metrics&period=custom&since=${encodeURIComponent(quarter.since)}&until=${encodeURIComponent(quarter.until)}`,
    );
  });
});

describe("compare query", () => {
  it("needs a kind and a name", () => {
    expect(parseCompare(q("kind=image&name=dev&a=1&b=2"))).toEqual({
      kind: "image",
      name: "dev",
      a: "1",
      b: "2",
    });
    expect(parseCompare(q("kind=workflow&name=w"))).toEqual({
      kind: "workflow",
      name: "w",
      a: "",
      b: "",
    });
    expect(parseCompare(q("name=dev"))).toBeNull();
    expect(parseCompare(q("kind=image"))).toBeNull();
    expect(parseCompare(q("kind=other&name=x"))).toBeNull();
  });

  it("builds a compare link with the period and host kept", () => {
    expect(compareQuery(q("host=h&period=week"), "image", "dev", "0.1.0", "0.2.0")).toBe(
      "host=h&period=week&kind=image&name=dev&a=0.1.0&b=0.2.0",
    );
  });

  it("picks the newest version as B and the one before as A, never unknown", () => {
    expect(defaultPair(["0.9.0", "unknown", "0.10.0"])).toEqual({ a: "0.9.0", b: "0.10.0" });
    expect(defaultPair(["unknown", "0.1.0"])).toEqual({ a: "", b: "0.1.0" });
    expect(defaultPair([])).toEqual({ a: "", b: "" });
  });

  it("finds the nearest older version for the row button", () => {
    const versions = ["0.10.0", "0.9.0", "unknown", "0.2.0"];
    expect(olderVersion(versions, "0.10.0")).toBe("0.9.0");
    expect(olderVersion(versions, "0.2.0")).toBe("");
    expect(olderVersion(versions, "0.9.0")).toBe("0.2.0");
  });
});
