import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { readRememberedQueue, rememberQueue } from "./taskQueueMemory";

beforeEach(() => localStorage.clear());
afterEach(() => vi.restoreAllMocks());

describe("taskQueueMemory", () => {
  it("keeps one queue per server", () => {
    rememberQueue("local", "DEV");
    rememberQueue("mac-mini", "OPS");
    expect(readRememberedQueue("local")).toBe("DEV");
    expect(readRememberedQueue("mac-mini")).toBe("OPS");
    expect(readRememberedQueue("other")).toBe("");
    expect(JSON.parse(localStorage.getItem("tasks:new-task-queue:v1") ?? "{}")).toEqual({
      local: "DEV",
      "mac-mini": "OPS",
    });
  });

  it("reads a broken record as nothing remembered", () => {
    localStorage.setItem("tasks:new-task-queue:v1", "{broken");
    expect(readRememberedQueue("local")).toBe("");
    rememberQueue("local", "DEV");
    expect(readRememberedQueue("local")).toBe("DEV");
    localStorage.setItem("tasks:new-task-queue:v1", JSON.stringify({ local: 7 }));
    expect(readRememberedQueue("local")).toBe("");
  });

  it("ignores unavailable storage", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("denied");
    });
    expect(() => rememberQueue("local", "DEV")).not.toThrow();
    expect(readRememberedQueue("local")).toBe("");
  });
});
