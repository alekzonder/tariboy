import { describe, expect, it } from "vitest";
import { queueMenuItems } from "./taskFilterMenus";

describe("queueMenuItems", () => {
  it("leads with every queue and its total, then each queue with its count", () => {
    const counts = new Map([
      ["OPS", 3],
      ["WEB", 2],
      ["GONE", 4],
    ]);
    expect(
      queueMenuItems([{ prefix: "OPS" }, { prefix: "WEB" }, { prefix: "NEW" }], counts),
    ).toEqual([
      { prefix: "", label: "All queues", count: 9 },
      { prefix: "OPS", label: "OPS", count: 3 },
      { prefix: "WEB", label: "WEB", count: 2 },
      { prefix: "NEW", label: "NEW", count: 0 },
    ]);
  });

  it("lists only the every-queue entry when there are no queues", () => {
    expect(queueMenuItems([], new Map())).toEqual([{ prefix: "", label: "All queues", count: 0 }]);
  });
});
