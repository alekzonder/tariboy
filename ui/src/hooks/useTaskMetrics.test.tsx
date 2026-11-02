import { describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import type { TaskMetrics } from "@/lib/api";
import { apiWrapper, fakeApi } from "@/test/api";
import { taskMetrics } from "@/test/fixtures/apiFixtures";
import { useTaskMetrics } from "./useTaskMetrics";

type Call = { key: string; signal?: AbortSignal; resolve: (m: TaskMetrics) => void };

function setup(initial: { taskKey: string; open: boolean }) {
  const calls: Call[] = [];
  const getTaskMetrics = vi.fn(
    (key: string, signal?: AbortSignal) =>
      new Promise<TaskMetrics>((resolve) => calls.push({ key, signal, resolve })),
  );
  const client = fakeApi({ metrics: { getTaskMetrics } });
  const hook = renderHook(
    (props: { taskKey: string; open: boolean }) => useTaskMetrics(props.taskKey, props.open),
    { initialProps: initial, wrapper: apiWrapper(client) },
  );
  return { calls, ...hook };
}

describe("useTaskMetrics", () => {
  it("asks for nothing while the section is closed", () => {
    const { calls, result } = setup({ taskKey: "DEV-1", open: false });
    expect(calls).toHaveLength(0);
    expect(result.current).toMatchObject({ metrics: null, loading: false, updating: false });
  });

  it("loads the task's metrics once the section opens", async () => {
    const { calls, result, rerender } = setup({ taskKey: "DEV-1", open: false });
    rerender({ taskKey: "DEV-1", open: true });
    expect(calls.map((c) => c.key)).toEqual(["DEV-1"]);
    expect(result.current.loading).toBe(true);
    await act(async () => calls[0].resolve(taskMetrics()));
    expect(result.current.metrics?.key).toBe("DEV-1");
    expect(result.current.updatedAt).toBeInstanceOf(Date);
  });

  it("cancels the request of the previous task and never shows its late answer", async () => {
    const { calls, result, rerender } = setup({ taskKey: "DEV-1", open: true });
    rerender({ taskKey: "DEV-2", open: true });
    expect(calls[0].signal?.aborted).toBe(true);
    await act(async () => calls[0].resolve(taskMetrics({ key: "DEV-1" })));
    expect(result.current.metrics).toBeNull();
    await act(async () => calls[1].resolve(taskMetrics({ key: "DEV-2" })));
    expect(result.current.metrics?.key).toBe("DEV-2");
  });

  it("does not show the previous task's metrics while the next one loads", async () => {
    const { calls, result, rerender } = setup({ taskKey: "DEV-1", open: true });
    await act(async () => calls[0].resolve(taskMetrics({ key: "DEV-1" })));
    rerender({ taskKey: "DEV-2", open: true });
    expect(result.current).toMatchObject({ metrics: null, loading: true, updating: false });
  });

  it("keeps the metrics, marked as updating, while a refresh loads", async () => {
    const { calls, result } = setup({ taskKey: "DEV-1", open: true });
    await act(async () => calls[0].resolve(taskMetrics({ title: "first" })));
    act(() => result.current.retry());
    expect(calls).toHaveLength(2);
    expect(result.current).toMatchObject({ updating: true, loading: false });
    expect(result.current.metrics?.title).toBe("first");
    await act(async () => calls[1].resolve(taskMetrics({ title: "second" })));
    expect(result.current.metrics?.title).toBe("second");
    expect(result.current.updating).toBe(false);
  });
});
