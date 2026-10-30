import { describe, expect, it, vi } from "vitest";
import type { Task, TasksApi } from "@/lib/api";
import { listAllTasks } from "./listAllTasks";

const task = (key: string) => ({ key }) as Task;

describe("listAllTasks", () => {
  it("follows the cursor to the end and keeps the newest sequence", async () => {
    const listTasks = vi.fn(async ({ after }: { after?: string }) =>
      after === "c1"
        ? { tasks: [task("A-2")], next_cursor: "", sequence: 4 }
        : { tasks: [task("A-1")], next_cursor: "c1", sequence: 5 },
    );
    const result = await listAllTasks({ status_view: "active" }, {
      listTasks,
    } as unknown as TasksApi);
    expect(result.tasks.map((item) => item.key)).toEqual(["A-1", "A-2"]);
    expect(result.sequence).toBe(5);
    expect(listTasks).toHaveBeenCalledWith({ status_view: "active", limit: 500, after: "c1" });
  });

  it("stops on a repeated cursor", async () => {
    const listTasks = vi.fn(async () => ({ tasks: [], next_cursor: "same", sequence: 0 }));
    await expect(listAllTasks({}, { listTasks } as unknown as TasksApi)).rejects.toThrow(
      "task pagination cursor repeated",
    );
  });
});
