import { act, render, waitFor } from "@/test/queryRender";
import { beforeEach, expect, it, vi } from "vitest";
import { useTasksSocket } from "@/hooks/useTasksSocket";
import { useApiQuery } from "@/hooks/useApiQuery";
import { createApi } from "@/lib/api";
import { taskKeys } from "./taskQueries";
import { useTaskQueryEvents } from "./useTaskQueryEvents";

vi.mock("@/hooks/useTasksSocket", () => ({ useTasksSocket: vi.fn() }));
beforeEach(() => vi.clearAllMocks());

it("coalesces staggered copies of one event across consumers and retains a trailing cycle", async () => {
  let finish!: () => void;
  const read = vi.fn().mockResolvedValue("initial");
  function Reader() {
    useApiQuery({ queryKey: taskKeys.notifications(null), queryFn: () => read() });
    useTaskQueryEvents(10);
    return null;
  }
  const mounted = render(
    <>
      <Reader />
      <Reader />
    </>,
  );
  await waitFor(() => expect(read).toHaveBeenCalledTimes(1));
  const [first, second] = vi
    .mocked(useTasksSocket)
    .mock.calls.slice(0, 2)
    .map(([options]) => options);
  read.mockImplementationOnce(
    () =>
      new Promise<string>((resolve) => {
        finish = () => resolve("fresh");
      }),
  );
  await act(async () =>
    first.onHint?.({ type: "event", sequence: 11, kind: "task.updated", task_key: "DEV-1" }),
  );
  await waitFor(() => expect(read).toHaveBeenCalledTimes(2));
  await act(async () =>
    second.onHint?.({ type: "event", sequence: 11, kind: "task.updated", task_key: "DEV-1" }),
  );
  await act(async () => finish());
  await waitFor(() => expect(mounted.client.isFetching()).toBe(0));
  expect(read).toHaveBeenCalledTimes(2);
  read.mockImplementationOnce(
    () =>
      new Promise<string>((resolve) => {
        finish = () => resolve("next");
      }),
  );
  await act(async () =>
    first.onHint?.({ type: "event", sequence: 12, kind: "task.updated", task_key: "DEV-1" }),
  );
  await waitFor(() => expect(read).toHaveBeenCalledTimes(3));
  await act(async () =>
    second.onHint?.({ type: "event", sequence: 13, kind: "task.updated", task_key: "DEV-1" }),
  );
  await act(async () => finish());
  await waitFor(() => expect(read).toHaveBeenCalledTimes(4));
});

it("rereads tasks after a read that never answered times out", async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  try {
    const api = createApi(null);
    let hang = true;
    const fetchMock = vi.fn((_url: string, init: RequestInit) =>
      hang
        ? new Promise<Response>((_resolve, reject) =>
            init.signal?.addEventListener("abort", () => reject(new Error("aborted"))),
          )
        : Promise.resolve({
            ok: true,
            status: 200,
            text: async () => JSON.stringify({ ok: true, result: "fresh" }),
          } as Response),
    );
    vi.stubGlobal("fetch", fetchMock);
    function Reader() {
      useApiQuery({
        queryKey: taskKeys.notifications(null),
        queryFn: ({ signal }) => api.get<string>("/api/tasks/notifications", signal),
      });
      useTaskQueryEvents(10);
      return null;
    }
    render(<Reader />, { api });
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const [options] = vi.mocked(useTasksSocket).mock.calls[0];
    await act(async () =>
      options.onHint?.({ type: "event", sequence: 11, kind: "task.updated", task_key: "DEV-1" }),
    );
    hang = false;
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
  } finally {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  }
});
