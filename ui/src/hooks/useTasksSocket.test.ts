import { act, renderHook } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { setLocalBaseURL } from "@/lib/api"
import type { Daemon } from "@/lib/daemons"
import { tasksWsUrl, useTasksSocket } from "./useTasksSocket"

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  static OPEN = 1
  static CONNECTING = 0
  static CLOSED = 3

  readonly url: string
  readyState = FakeWebSocket.CONNECTING
  onopen: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  close = vi.fn(() => {
    this.readyState = FakeWebSocket.CLOSED
  })

  constructor(url: string) {
    this.url = url
    FakeWebSocket.instances.push(this)
  }
}

beforeEach(() => {
  FakeWebSocket.instances = []
  setLocalBaseURL("")
  vi.stubGlobal("WebSocket", FakeWebSocket)
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  setLocalBaseURL("")
})

describe("tasks websocket", () => {
  it("builds local and remote resume URLs using existing token conventions", () => {
    expect(tasksWsUrl(null, 12)).toMatch(/^ws:\/\/.*\/api\/tasks\/ws\?after=12$/)

    const remote: Daemon = {
      id: "prod",
      label: "Production",
      baseURL: "https://tasks.example.test/",
      token: "secret token",
    }
    expect(tasksWsUrl(remote, 7)).toBe(
      "wss://tasks.example.test/api/tasks/ws?after=7&token=secret+token",
    )
  })

  it("delivers valid hints in order and resumes a reconnect after the last sequence", () => {
    vi.useFakeTimers()
    const onHint = vi.fn()
    renderHook(() => useTasksSocket({ target: null, after: 4, onHint }))
    const first = FakeWebSocket.instances[0]

    act(() => {
      first.onmessage?.({ data: JSON.stringify({
        sequence: 5,
        event_id: "event-5",
        kind: "task.updated",
        task_key: "TEST-1",
      }) } as MessageEvent)
      first.onmessage?.({ data: JSON.stringify({
        sequence: 6,
        event_id: "event-6",
        kind: "comment.created",
        task_key: "TEST-1",
      }) } as MessageEvent)
    })

    expect(onHint.mock.calls.map(([hint]) => hint.sequence)).toEqual([5, 6])

    act(() => {
      first.onclose?.({ code: 1006 } as CloseEvent)
      vi.advanceTimersByTime(250)
    })

    expect(FakeWebSocket.instances[1].url).toContain("after=6")
  })

  it("ignores malformed and duplicate hints and exposes a server reset", () => {
    const onHint = vi.fn()
    const onReset = vi.fn()
    renderHook(() => useTasksSocket({ after: 9, onHint, onReset }))
    const socket = FakeWebSocket.instances[0]

    act(() => {
      socket.onmessage?.({ data: "not-json" } as MessageEvent)
      socket.onmessage?.({ data: JSON.stringify({ sequence: 9, kind: "old" }) } as MessageEvent)
      socket.onmessage?.({ data: JSON.stringify({ type: "reset", sequence: 12 }) } as MessageEvent)
    })

    expect(onHint).not.toHaveBeenCalled()
    expect(onReset).toHaveBeenCalledWith(12)
  })

  it("uses capped exponential reconnect and closes cleanly on unmount", () => {
    vi.useFakeTimers()
    const { unmount } = renderHook(() => useTasksSocket({ onHint: vi.fn() }))

    for (const elapsed of [250, 500, 1000, 2000, 4000, 5000]) {
      const current = FakeWebSocket.instances.at(-1)!
      act(() => {
        current.onclose?.({ code: 1006 } as CloseEvent)
        vi.advanceTimersByTime(elapsed)
      })
    }

    expect(FakeWebSocket.instances).toHaveLength(7)
    const last = FakeWebSocket.instances.at(-1)!
    unmount()
    expect(last.close).toHaveBeenCalledOnce()
    act(() => vi.runAllTimers())
    expect(FakeWebSocket.instances).toHaveLength(7)
  })
})
