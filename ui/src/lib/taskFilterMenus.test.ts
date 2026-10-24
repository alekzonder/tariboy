import { describe, expect, it } from "vitest";
import { queueMenuItems, serverMenuItems } from "./taskFilterMenus";

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

describe("serverMenuItems", () => {
  it("leads with every server and the summed count, then each server as given", () => {
    const servers = [
      { id: "a", label: "Alpha", count: 2, unavailable: false },
      { id: "b", label: "Beta", count: 5, unavailable: true },
    ];
    expect(serverMenuItems(servers)).toEqual([
      { id: "", label: "All servers", count: 7, unavailable: false },
      ...servers,
    ]);
  });
});
