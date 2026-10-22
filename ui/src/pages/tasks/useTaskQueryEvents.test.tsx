import { act, render, waitFor } from "@/test/queryRender"
import { beforeEach, expect, it, vi } from "vitest"
import { useTasksSocket } from "@/hooks/useTasksSocket"
import { useApiQuery } from "@/hooks/useApiQuery"
import { taskKeys } from "./taskQueries"
import { useTaskQueryEvents } from "./useTaskQueryEvents"

vi.mock("@/hooks/useTasksSocket", () => ({ useTasksSocket: vi.fn() }))
beforeEach(() => vi.clearAllMocks())

it("coalesces staggered copies of one event across consumers and retains a trailing cycle", async () => {
  let finish!: () => void
  const read = vi.fn().mockResolvedValue("initial")
  function Reader() {
    useApiQuery({ queryKey: taskKeys.notifications(null), queryFn: () => read() }, null)
    useTaskQueryEvents(null, 10)
    return null
  }
  const mounted = render(<><Reader /><Reader /></>)
  await waitFor(() => expect(read).toHaveBeenCalledTimes(1))
  const [first, second] = vi.mocked(useTasksSocket).mock.calls.slice(0, 2).map(([options]) => options)
  read.mockImplementationOnce(() => new Promise<string>((resolve) => { finish = () => resolve("fresh") }))
  await act(async () => first.onHint?.({ type: "event", sequence: 11, kind: "task.updated", task_key: "DEV-1" }))
  await waitFor(() => expect(read).toHaveBeenCalledTimes(2))
  await act(async () => second.onHint?.({ type: "event", sequence: 11, kind: "task.updated", task_key: "DEV-1" }))
  await act(async () => finish())
  await waitFor(() => expect(mounted.client.isFetching()).toBe(0))
  expect(read).toHaveBeenCalledTimes(2)
  read.mockImplementationOnce(() => new Promise<string>((resolve) => { finish = () => resolve("next") }))
  await act(async () => first.onHint?.({ type: "event", sequence: 12, kind: "task.updated", task_key: "DEV-1" }))
  await waitFor(() => expect(read).toHaveBeenCalledTimes(3))
  await act(async () => second.onHint?.({ type: "event", sequence: 13, kind: "task.updated", task_key: "DEV-1" }))
  await act(async () => finish())
  await waitFor(() => expect(read).toHaveBeenCalledTimes(4))
})
