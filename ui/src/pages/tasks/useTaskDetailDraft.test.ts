import { act, renderHook } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { workflowTask } from "@/test/fixtures/workflowFixtures";
import { useTaskDetailDraft } from "./useTaskDetailDraft";

const task = { ...workflowTask, workflow_digest: undefined, workflow_name: undefined };

it("keeps the draft revision until edited fields are rebased onto the latest task", async () => {
  const save = vi.fn().mockImplementation(async (input) => ({ ...task, ...input }));
  const { result, rerender } = renderHook(({ current }) => useTaskDetailDraft(current, save), {
    initialProps: { current: task },
  });
  act(() => result.current.setTitle("My edit"));
  const current = { ...task, revision: task.revision + 1, description: "Server description" };
  rerender({ current });
  expect(result.current.baseline.revision).toBe(task.revision);
  expect(result.current.title).toBe("My edit");
  act(() => result.current.keepEdits());
  expect(result.current.description).toBe("Server description");
  expect(result.current.title).toBe("My edit");
  await act(() => result.current.save());
  expect(save).toHaveBeenCalledWith(
    expect.objectContaining({
      revision: current.revision,
      title: "My edit",
      description: "Server description",
    }),
  );
});

it("retains edits when saving fails and adopts the server task only after reload", async () => {
  const save = vi.fn().mockRejectedValue(new Error("conflict"));
  const { result } = renderHook(() => useTaskDetailDraft(task, save));
  act(() => result.current.setTitle("Unsaved"));
  await act(() => result.current.save());
  expect(result.current.title).toBe("Unsaved");
  expect(result.current.dirty).toBe(true);
  expect(result.current.saving).toBe(false);
  act(() => result.current.adopt(task));
  expect(result.current.dirty).toBe(false);
});
