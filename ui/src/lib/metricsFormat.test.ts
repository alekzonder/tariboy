import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api";
import {
  SMALL_SAMPLE_TASKS,
  avgTokens,
  byVersionDesc,
  fmtAvg,
  fmtDuration,
  fmtMoney,
  fmtTokens,
  metricsErrorKind,
  tokenSum,
  tokenTitle,
  versionLabel,
  workflowLabel,
} from "./metricsFormat";

describe("metrics values", () => {
  it("shows a time like the CLI", () => {
    expect(fmtDuration(3 * 86400 + 4 * 3600 + 59)).toBe("3d 4h");
    expect(fmtDuration(12 * 3600 + 5 * 60)).toBe("12h 05m");
    expect(fmtDuration(42 * 60 + 30)).toBe("42m");
    expect(fmtDuration(59)).toBe("<1m");
    expect(fmtDuration(0)).toBe("<1m");
  });

  it("shows a cost in dollars and never rounds a small cost to zero", () => {
    expect(fmtMoney(12.4)).toBe("$12.40");
    expect(fmtMoney(0.004)).toBe("<$0.01");
    expect(fmtMoney(0.01)).toBe("$0.01");
    expect(fmtMoney(0)).toBe("$0.00");
  });

  it("shows a per-task average with one decimal", () => {
    expect(fmtAvg(2.31)).toBe("2.3");
    expect(fmtAvg(0)).toBe("0.0");
  });

  it("shows tokens short, with the four kinds in a hint", () => {
    expect(fmtTokens(1_200_000)).toBe("1.2M");
    expect(fmtTokens(340_400)).toBe("340k");
    expect(fmtTokens(900)).toBe("900");
    expect(fmtTokens(0)).toBe("0");
    const tokens = { input: 1000, output: 2000, cache_write: 30, cache_read: 4 };
    expect(tokenSum(tokens)).toBe(3034);
    expect(tokenTitle(tokens)).toBe("Input 1,000 · Output 2,000 · Cache write 30 · Cache read 4");
    expect(
      avgTokens({
        input_tokens: 1,
        output_tokens: 2,
        cache_write_tokens: 3,
        cache_read_tokens: 4,
      }),
    ).toEqual({ input: 1, output: 2, cache_write: 3, cache_read: 4 });
  });

  it("orders versions by their numbers, newest first, unknown last", () => {
    expect(["0.9.0", "unknown", "0.10.0", "0.2.1", "1.0.0"].sort(byVersionDesc)).toEqual([
      "1.0.0",
      "0.10.0",
      "0.9.0",
      "0.2.1",
      "unknown",
    ]);
  });

  it("names the special versions and workflows", () => {
    expect(versionLabel("unknown")).toBe("unknown version");
    expect(versionLabel("0.3.0")).toBe("0.3.0");
    expect(workflowLabel("none")).toBe("No workflow");
    expect(workflowLabel("unknown")).toBe("Unknown workflow");
    expect(workflowLabel("development")).toBe("development");
  });

  it("names the small sample threshold", () => {
    expect(SMALL_SAMPLE_TASKS).toBe(5);
  });
});

describe("metricsErrorKind", () => {
  it("tells each error the screen shows its own way", () => {
    expect(metricsErrorKind(new ApiError(400, "bad_time", "since is not RFC 3339"))).toBe(
      "bad_time",
    );
    expect(metricsErrorKind(new ApiError(400, "invalid_param", "bad image"))).toBe(
      "invalid_param",
    );
    expect(metricsErrorKind(new ApiError(404, "not_found", "unknown route GET /api/metrics"))).toBe(
      "old_daemon",
    );
    expect(metricsErrorKind(new ApiError(0, "network_error", "request timed out"))).toBe("timeout");
    expect(metricsErrorKind(new ApiError(404, "not_found", "task not found"))).toBe("other");
    expect(metricsErrorKind(new ApiError(0, "network_error", "network error: refused"))).toBe(
      "other",
    );
    expect(metricsErrorKind(new Error("boom"))).toBe("other");
  });
});
