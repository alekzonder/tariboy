import { describe, expect, it, vi } from "vitest";
import { vitestMaxWorkers } from "./vite.workers";

describe("vitestMaxWorkers", () => {
  it.each([undefined, "", " ", "abc"])("uses 4 workers for %j without a warning", (raw) => {
    const warn = vi.fn();
    expect(vitestMaxWorkers(raw, warn)).toBe(4);
    expect(warn).not.toHaveBeenCalled();
  });

  it.each(["1", "0", "-3", "1.5"])("raises %j to 2 workers and warns once", (raw) => {
    const warn = vi.fn();
    expect(vitestMaxWorkers(raw, warn)).toBe(2);
    expect(warn).toHaveBeenCalledTimes(1);
    expect(warn.mock.calls[0][0]).toContain(`TARIBOY_VITEST_MAX_WORKERS=${raw} is below 2`);
    expect(warn.mock.calls[0][0]).toContain("using 2 workers");
  });

  it.each([
    ["2", 2],
    ["4", 4],
    ["6", 6],
  ])("keeps %j without a warning", (raw, want) => {
    const warn = vi.fn();
    expect(vitestMaxWorkers(raw, warn)).toBe(want);
    expect(warn).not.toHaveBeenCalled();
  });
});
