import { QueryClient } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import type { ApiClient, Task } from "@/lib/api";
import { moveTaskToHost } from "./taskDrawerMove";

const toast = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn() }));
const calls = vi.hoisted(() => [] as string[]);
const destination = vi.hoisted(() => ({
  importTask: vi.fn(),
  getTask: vi.fn(),
  updateTask: vi.fn(),
}));
vi.mock("sonner", () => ({ toast }));
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  createApi: vi.fn(() => ({ target: { id: "far" }, tasks: destination })),
}));

const task: Task = {
  key: "TEST-1",
  queue: "TEST",
  parent_key: "",
  position: 0,
  priority: "P1",
  title: "Ship the drawer",
  description: "",
  status: "open",
  category: "open",
  pull_request: "",
  author: "user:owner",
  customer: "user:owner",
  group: "",
  assignee: "agent:worker",
  manual_block_reason: "",
  blocked: false,
  revision: 2,
  created_at: "2026-07-31T10:00:00Z",
  updated_at: "2026-07-31T10:00:00Z",
  completed_at: "",
};
const remote = { value: "far:agent:dev", label: "dev", assignee: "agent:dev", hostId: "far" };
const source = {
  addTaskComment: vi.fn(async () => void calls.push("source.comment")),
  exportTask: vi.fn(async () => {
    calls.push("source.export");
    return {
      root_key: "TEST-1",
      tasks: [
        { key: "TEST-1", assignee: "agent:worker" },
        { key: "TEST-2", assignee: "agent:worker" },
      ],
    };
  }),
  getTask: vi.fn(async () => ({ task: { ...task, revision: 3 } })),
  updateTask: vi.fn(async (_key: string, body: object) => {
    calls.push(`source.update ${JSON.stringify(body)}`);
    return task;
  }),
};
// The source client is a stub too: createApi is mocked to build the destination.
const api = { target: null, tasks: source } as unknown as ApiClient;

beforeEach(() => {
  vi.clearAllMocks();
  calls.length = 0;
  destination.importTask.mockImplementation(async () => {
    calls.push("destination.import");
    return { ...task, assignee: "" };
  });
  destination.getTask.mockResolvedValue({ task: { ...task, revision: 7 } });
  destination.updateTask.mockImplementation(async (_key: string, body: object) => {
    calls.push(`destination.update ${JSON.stringify(body)}`);
    return { ...task, assignee: "agent:dev" };
  });
});

const input = {
  revision: 2,
  title: "Renamed",
  description: "",
  pull_request: "",
  priority: "P1" as const,
};

it("saves edits, imports the tree unassigned, retires the source, then assigns", async () => {
  const onMoved = vi.fn();
  const result = await moveTaskToHost({
    api,
    client: new QueryClient(),
    task,
    input: { ...input, assignee: "far:agent:dev" },
    remote,
    onMoved,
  });
  expect(calls).toEqual([
    `source.update ${JSON.stringify({ ...input, assignee: "agent:worker" })}`,
    "source.export",
    "destination.import",
    "source.comment",
    `source.update ${JSON.stringify({ status: "cancelled", revision: 3 })}`,
    `destination.update ${JSON.stringify({ assignee: "agent:dev", revision: 7 })}`,
  ]);
  expect(destination.importTask).toHaveBeenCalledWith({
    root_key: "TEST-1",
    tasks: [
      { key: "TEST-1", assignee: "" },
      { key: "TEST-2", assignee: "agent:worker" },
    ],
  });
  expect(source.addTaskComment).toHaveBeenCalledWith(
    "TEST-1",
    "Moved to far as TEST-1.",
    expect.any(String),
  );
  expect(result.assignee).toBe("agent:dev");
  expect(toast.success).toHaveBeenCalledWith("TEST-1 moved to far");
  expect(onMoved).toHaveBeenCalledWith("far");
});

it("reports a failure after the import and still follows the task", async () => {
  destination.getTask.mockRejectedValue(new Error("gone"));
  const onMoved = vi.fn();
  const result = await moveTaskToHost({
    api,
    client: new QueryClient(),
    task,
    input: { ...input, title: task.title },
    remote: { ...remote, hostLabel: "Far server" },
    onMoved,
  });
  expect(calls[0]).toBe("source.export");
  expect(result.assignee).toBe("");
  expect(toast.error).toHaveBeenCalledWith(
    "TEST-1 moved to Far server, but the move did not finish: gone",
  );
  expect(onMoved).toHaveBeenCalledWith("far");
});
