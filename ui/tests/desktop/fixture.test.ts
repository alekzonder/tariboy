import { EventEmitter } from "node:events";
import type { ChildProcess } from "node:child_process";

import { describe, expect, it } from "vitest";

import { stopChild, TailBuffer } from "./fixture";

describe("Desktop E2E diagnostics", () => {
  it("retains only the configured log tail", () => {
    const log = new TailBuffer(8);

    log.append("prefix-");
    log.append("diagnostic");

    expect(log.toString()).toBe("agnostic");
    expect(log.bytes()).toHaveLength(8);
  });

  it("does not miss a synchronous child exit while stopping", async () => {
    const child = new EventEmitter() as ChildProcess;
    Object.defineProperty(child, "exitCode", { value: null, writable: true });
    child.kill = () => {
      Object.defineProperty(child, "exitCode", { value: 0, writable: true });
      child.emit("exit", 0, null);
      return true;
    };

    await expect(Promise.race([
      stopChild(child),
      new Promise((_, reject) => setTimeout(() => reject(new Error("stopChild hung")), 200)),
    ])).resolves.toBeUndefined();
  });
});
