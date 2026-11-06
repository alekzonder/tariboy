import { describe, expect, it } from "vitest";
import type { Queue, Task } from "./api";
import { countTasksByQueue } from "./tasks";
import {
  buildTaskForest,
  canReorderTaskBeside,
  canDropTaskInside,
  filterTasksByQueue,
  flattenVisible,
  parentQueueLabel,
  parentQueueOf,
  queueMarkerLabel,
  queueOfKey,
  resolveTaskDrop,
} from "./taskTree";

function task(
  key: string,
  parentKey = "",
  position = 0,
  priority: "P0" | "P1" | "P2" | "P3" = "P2",
): Task {
  return {
    key,
    category: "open",
    queue: "TEST",
    parent_key: parentKey,
    position,
    priority,
    title: key,
    description: "",
    status: "open",
    author: "user:owner",
    customer: "user:owner",
    group: "",
    assignee: "",
    manual_block_reason: "",
    blocked: false,
    budget_usd: 0,
    revision: 1,
    created_at: "2026-07-31T10:00:00Z",
    updated_at: "2026-07-31T10:00:00Z",
    completed_at: "",
    started_at: "",
    pull_request: "",
  };
}

describe("task tree model", () => {
  it("keeps arbitrary-depth descendants in a five-level forest", () => {
    const rows = [
      task("TEST-5", "TEST-4"),
      task("TEST-3", "TEST-2"),
      task("TEST-1"),
      task("TEST-4", "TEST-3"),
      task("TEST-2", "TEST-1"),
    ];

    const forest = buildTaskForest(rows);

    expect(forest).toHaveLength(1);
    expect(forest[0].children[0].children[0].children[0].children[0].task.key).toBe("TEST-5");
  });

  it("keeps context ancestors and promotes an orphan with a missing parent", () => {
    const forest = buildTaskForest([
      { ...task("TEST-1"), access: "context" },
      task("TEST-2", "TEST-1"),
      task("TEST-9", "TEST-404"),
    ]);

    expect(forest.map((node) => node.task.key)).toEqual(["TEST-1", "TEST-9"]);
    expect(forest[0].children[0].task.key).toBe("TEST-2");
    expect(forest[1].orphaned).toBe(true);
  });

  it("sorts root and nested siblings by priority, position, and key", () => {
    const forest = buildTaskForest([
      task("TEST-4", "", 1, "P3"),
      task("TEST-3", "", 20, "P0"),
      task("TEST-2", "", 10, "P0"),
      task("TEST-1", "", 1, "P2"),
      task("TEST-8", "TEST-1", 1, "P2"),
      task("TEST-7", "TEST-1", 20, "P1"),
      task("TEST-6", "TEST-1", 10, "P1"),
      task("TEST-5", "TEST-1", 1, "P0"),
    ]);

    expect(forest.map((node) => node.task.key)).toEqual(["TEST-2", "TEST-3", "TEST-1", "TEST-4"]);
    expect(forest[2].children.map((node) => node.task.key)).toEqual([
      "TEST-5",
      "TEST-6",
      "TEST-7",
      "TEST-8",
    ]);
  });

  it("only reorders siblings within the same priority", () => {
    expect(canReorderTaskBeside(task("TEST-1", "", 1, "P0"), task("TEST-2", "", 2, "P0"))).toBe(
      true,
    );
    expect(canReorderTaskBeside(task("TEST-1", "", 1, "P3"), task("TEST-2", "", 2, "P0"))).toBe(
      false,
    );
    expect(canReorderTaskBeside(undefined, task("TEST-2"))).toBe(false);
  });

  it("flattens only expanded branches", () => {
    const forest = buildTaskForest([
      task("TEST-1"),
      task("TEST-2", "TEST-1"),
      task("TEST-3", "TEST-2"),
    ]);

    expect(flattenVisible(forest, new Set()).map((row) => row.task.key)).toEqual(["TEST-1"]);
    expect(
      flattenVisible(forest, new Set(["TEST-1"])).map((row) => [row.task.key, row.depth]),
    ).toEqual([
      ["TEST-1", 0],
      ["TEST-2", 1],
    ]);
  });

  it("rejects an inside drop onto the dragged task or its own subtree", () => {
    const forest = buildTaskForest([
      task("TEST-1"),
      task("TEST-2", "TEST-1"),
      task("TEST-3", "TEST-2"),
      task("TEST-4"),
    ]);

    expect(canDropTaskInside(forest, "TEST-1", "TEST-1")).toBe(false);
    expect(canDropTaskInside(forest, "TEST-1", "TEST-3")).toBe(false);
    expect(canDropTaskInside(forest, "TEST-1", "TEST-4")).toBe(true);
  });

  describe("resolveTaskDrop", () => {
    const rows = [
      task("TEST-1", "", 1),
      task("TEST-2", "", 2),
      task("TEST-3", "", 3),
      task("TEST-4", "TEST-1", 1),
      task("TEST-5", "TEST-4", 1),
      task("TEST-6", "", 4, "P0"),
      { ...task("OTHER-1", "", 5), queue: "OTHER" },
    ];
    const resolve = (activeKey: string, overId: string) =>
      resolveTaskDrop({ tasks: rows, forest: buildTaskForest(rows), activeKey, overId });

    it("moves a task to the root", () => {
      expect(resolve("TEST-4", "__root__")).toEqual({
        key: "TEST-4",
        parentKey: "",
        beforeKey: "",
      });
    });

    it("places a task before or after a sibling", () => {
      expect(resolve("TEST-3", "before:TEST-2")).toEqual({
        key: "TEST-3",
        parentKey: "",
        beforeKey: "TEST-2",
      });
      expect(resolve("TEST-1", "after:TEST-2")).toEqual({
        key: "TEST-1",
        parentKey: "",
        beforeKey: "TEST-3",
      });
      expect(resolve("TEST-1", "after:OTHER-1")).toEqual({
        key: "TEST-1",
        parentKey: "",
        beforeKey: "",
      });
    });

    it("rejects a reorder beside a task of another priority", () => {
      expect(resolve("TEST-1", "before:TEST-6")).toBeNull();
    });

    it("nests a task inside another one", () => {
      expect(resolve("TEST-2", "inside:TEST-5")).toEqual({
        key: "TEST-2",
        parentKey: "TEST-5",
        beforeKey: "",
      });
    });

    it("rejects a drop into another queue, onto itself, into its subtree or onto nothing", () => {
      expect(resolve("TEST-2", "inside:OTHER-1")).toBeNull();
      expect(resolve("TEST-2", "TEST-2")).toBeNull();
      expect(resolve("TEST-2", "inside:TEST-2")).toBeNull();
      expect(resolve("TEST-1", "inside:TEST-5")).toBeNull();
      expect(resolve("TEST-1", "inside:TEST-404")).toBeNull();
    });
  });
});

function queued(key: string, queue: string, parentKey = "", position = 0): Task {
  return { ...task(key, parentKey, position), queue };
}

function queue(prefix: string, name: string): Queue {
  return {
    prefix,
    name,
    description: "",
    owners: [],
    responsible_agent: "",
    revision: 1,
    created_at: "2026-07-31T10:00:00Z",
    updated_at: "2026-07-31T10:00:00Z",
  };
}

describe("subtasks from another queue", () => {
  const queues = [queue("TARI", "Tasks"), queue("TARIDESIGN", "Design")];
  // TARI-1 → TARIDESIGN-2 → TARI-5, TARI-1 → TARI-3, and a TARI-9 root.
  const rows = [
    queued("TARI-1", "TARI"),
    queued("TARIDESIGN-2", "TARIDESIGN", "TARI-1", 1),
    queued("TARI-3", "TARI", "TARI-1", 2),
    queued("TARI-5", "TARI", "TARIDESIGN-2"),
    queued("TARI-9", "TARI"),
  ];

  it("takes a parent's queue from its key up to the last dash", () => {
    expect(queueOfKey("TARI-1")).toBe("TARI");
    expect(queueOfKey("TARI-DESIGN-ab12")).toBe("TARI-DESIGN");
    expect(queueOfKey("NODASH")).toBe("NODASH");
  });

  it("gives a new child its parent's queue, loaded or from the key", () => {
    expect(parentQueueOf("TARIDESIGN-2", rows)).toBe("TARIDESIGN");
    expect(parentQueueOf("TARILEAD-3", rows)).toBe("TARILEAD");
  });

  it("names the queue, its workflow and the parent queue in the marker", () => {
    const flexible = queued("TARIDESIGN-2", "TARIDESIGN", "TARI-1");
    const workflow = { ...flexible, workflow_name: "design-flow" };
    expect(queueMarkerLabel(workflow, "TARI", queues)).toBe(
      "Queue TARIDESIGN · Design · workflow design-flow (parent queue TARI)",
    );
    expect(queueMarkerLabel(flexible, "TARI", queues)).toBe(
      "Queue TARIDESIGN · Design · flexible tasks (parent queue TARI)",
    );
    expect(queueMarkerLabel(flexible, "TARI", [])).toBe("Queue TARIDESIGN (parent queue TARI)");
    expect(parentQueueLabel("TARI", queues)).toBe("Parent queue TARI · Tasks");
    expect(parentQueueLabel("GONE", queues)).toBe("Parent queue GONE");
  });

  it("marks only a child whose loaded parent is in another queue", () => {
    const flat = flattenVisible(buildTaskForest(rows), new Set(["TARI-1", "TARIDESIGN-2"]));
    const cross = Object.fromEntries(flat.map((row) => [row.task.key, row.crossQueue]));
    expect(cross).toEqual({
      "TARI-1": false,
      "TARIDESIGN-2": true,
      "TARI-5": true,
      "TARI-3": false,
      "TARI-9": false,
    });
    expect(flat.find((row) => row.task.key === "TARIDESIGN-2")?.parentQueue).toBe("TARI");
  });

  it("links an orphaned root to a parent in another queue, loaded or not", () => {
    const loaded = buildTaskForest([queued("TARIDESIGN-2", "TARIDESIGN", "TARI-1")], rows);
    expect(loaded[0].orphaned).toBe(true);
    expect(loaded[0].crossQueue).toBe(false);
    expect(loaded[0].parentLink).toEqual({ key: "TARI-1", queue: "TARI", canOpen: true });

    const missing = buildTaskForest([queued("TARIDESIGN-2", "TARIDESIGN", "TARI-1")]);
    expect(missing[0].parentLink).toEqual({ key: "TARI-1", queue: "TARI", canOpen: false });

    const sameQueue = buildTaskForest([queued("TARI-3", "TARI", "TARI-1")]);
    expect(sameQueue[0].parentLink).toBeUndefined();
    expect(buildTaskForest([queued("TARI-9", "TARI")])[0].parentLink).toBeUndefined();
  });

  it("keeps the whole subtree of the filtered queue, once", () => {
    const filtered = filterTasksByQueue(rows, "TARI");
    expect(filtered.map((row) => row.key)).toEqual([
      "TARI-1",
      "TARIDESIGN-2",
      "TARI-3",
      "TARI-5",
      "TARI-9",
    ]);
    const forest = buildTaskForest(filtered, rows);
    expect(forest.map((node) => node.task.key)).toEqual(["TARI-1", "TARI-9"]);
    expect(forest[0].children[0].children[0].task.key).toBe("TARI-5");
  });

  it("shows a child queue's task as a root without its parent", () => {
    const filtered = filterTasksByQueue(rows, "TARIDESIGN");
    expect(filtered.map((row) => row.key)).toEqual(["TARIDESIGN-2", "TARI-5"]);
    const forest = buildTaskForest(filtered, rows);
    expect(forest.map((node) => node.task.key)).toEqual(["TARIDESIGN-2"]);
    expect(forest[0].parentLink).toEqual({ key: "TARI-1", queue: "TARI", canOpen: true });
  });

  it("counts each task only in its own queue whatever the filter", () => {
    expect(Object.fromEntries(countTasksByQueue(rows))).toEqual({ TARI: 4, TARIDESIGN: 1 });
  });

  it("does not reorder a child from another queue among its siblings", () => {
    const parent = queued("TARI-1", "TARI");
    const kid = queued("TARIDESIGN-2", "TARIDESIGN", "TARI-1");
    expect(canReorderTaskBeside(kid, queued("TARI-3", "TARI", "TARI-1"), parent)).toBe(false);
    expect(canReorderTaskBeside(queued("TARI-3", "TARI", "TARI-1"), kid, parent)).toBe(true);
    const resolve = (activeKey: string, overId: string) =>
      resolveTaskDrop({ tasks: rows, forest: buildTaskForest(rows), activeKey, overId });
    expect(resolve("TARIDESIGN-2", "before:TARI-3")).toBeNull();
    expect(resolve("TARIDESIGN-2", "inside:TARI-9")).toBeNull();
    expect(resolve("TARIDESIGN-2", "__root__")).toEqual({
      key: "TARIDESIGN-2",
      parentKey: "",
      beforeKey: "",
    });
  });
});
