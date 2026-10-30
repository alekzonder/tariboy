import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { readAgentQueue, readRememberedQueue, rememberQueue } from "./taskQueueMemory";

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

  it("keeps one queue per agent of a server and falls back to the server's queue", () => {
    rememberQueue("local", "DEV");
    rememberQueue("local", "OPS", "alpha");
    rememberQueue("local", "TEST", "beta");
    rememberQueue("mac-mini", "QA", "alpha");
    expect(readRememberedQueue("local", "alpha")).toBe("OPS");
    expect(readRememberedQueue("local", "beta")).toBe("TEST");
    expect(readRememberedQueue("mac-mini", "alpha")).toBe("QA");
    // The server's record still follows the last task created there.
    expect(readRememberedQueue("local")).toBe("TEST");
    expect(readRememberedQueue("local", "gamma")).toBe("TEST");
    expect(JSON.parse(localStorage.getItem("tasks:new-task-queue-by-agent:v1") ?? "{}")).toEqual({
      local: { alpha: "OPS", beta: "TEST" },
      "mac-mini": { alpha: "QA" },
    });
  });

  it("reads an agent's own queue without the server's fallback", () => {
    rememberQueue("local", "DEV");
    expect(readAgentQueue("local", "alpha")).toBe("");
    rememberQueue("local", "OPS", "alpha");
    expect(readAgentQueue("local", "alpha")).toBe("OPS");
  });

  it("reads a broken agent record as nothing remembered", () => {
    localStorage.setItem("tasks:new-task-queue-by-agent:v1", "{broken");
    expect(readRememberedQueue("local", "alpha")).toBe("");
    rememberQueue("local", "DEV", "alpha");
    expect(readRememberedQueue("local", "alpha")).toBe("DEV");
    localStorage.setItem(
      "tasks:new-task-queue-by-agent:v1",
      JSON.stringify({ local: { alpha: 7 }, other: "DEV" }),
    );
    expect(readRememberedQueue("local", "alpha")).toBe("DEV");
    rememberQueue("other", "OPS", "alpha");
    expect(readRememberedQueue("other", "alpha")).toBe("OPS");
  });

  it("ignores unavailable storage", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("denied");
    });
    expect(() => rememberQueue("local", "DEV")).not.toThrow();
    expect(() => rememberQueue("local", "DEV", "alpha")).not.toThrow();
    expect(readRememberedQueue("local")).toBe("");
    expect(readRememberedQueue("local", "alpha")).toBe("");
  });
});
