import { describe, expect, it } from "vitest";
import type { Task } from "@/lib/api";
import { countTasksByQueue } from "@/lib/tasks";
import { allTasksQueues, filterServerTasks, serverTaskRows } from "./allTasksView";
import type { ServerTask } from "./useAllServersTasks";

const task = (over: Partial<ServerTask>): ServerTask =>
  ({
    key: "T-1",
    queue: "T",
    parent_key: "",
    position: 0,
    priority: "P2",
    updated_at: "2026-07-31T10:00:00Z",
    serverId: "",
    serverName: "local",
    ...over,
  }) as ServerTask;

describe("all tasks view", () => {
  it("counts tasks per queue", () => {
    const tasks = [{ queue: "A" }, { queue: "B" }, { queue: "A" }] as Task[];
    expect([...countTasksByQueue(tasks)]).toEqual([
      ["A", 2],
      ["B", 1],
    ]);
  });

  it("filters by server parameter and queue", () => {
    const tasks = [
      task({ key: "A-1", queue: "A", serverId: "" }),
      task({ key: "B-1", queue: "B", serverId: "" }),
      task({ key: "A-1", queue: "A", serverId: "ssh:box" }),
    ];
    const keys = (list: ServerTask[]) => list.map((item) => `${item.serverId}/${item.key}`);
    expect(keys(filterServerTasks({ tasks, server: "", queue: "" }))).toHaveLength(3);
    expect(keys(filterServerTasks({ tasks, server: "", queue: "A" }))).toEqual([
      "/A-1",
      "ssh:box/A-1",
    ]);
    expect(keys(filterServerTasks({ tasks, server: "local", queue: "" }))).toEqual([
      "/A-1",
      "/B-1",
    ]);
  });

  it("lists counted queues and keeps the selected one", () => {
    const counts = new Map([
      ["B", 1],
      ["A", 2],
    ]);
    expect(allTasksQueues(counts, "")).toEqual([{ prefix: "A" }, { prefix: "B" }]);
    expect(allTasksQueues(counts, "C")).toEqual([
      { prefix: "A" },
      { prefix: "B" },
      { prefix: "C" },
    ]);
  });

  it("merges server roots newest first and expands per server", () => {
    const rows = serverTaskRows(
      [
        task({ key: "A-1", serverId: "", updated_at: "2026-07-31T10:00:00Z" }),
        task({ key: "A-2", parent_key: "A-1", serverId: "" }),
        task({ key: "A-1", serverId: "ssh:box", updated_at: "2026-08-01T10:00:00Z" }),
        task({ key: "A-2", parent_key: "A-1", serverId: "ssh:box" }),
      ],
      new Set(["\u0000A-1"]),
    );
    expect(rows.map((row) => `${row.task.serverId}/${row.task.key}/${row.depth}`)).toEqual([
      "ssh:box/A-1/0",
      "/A-1/0",
      "/A-2/1",
    ]);
  });
});
