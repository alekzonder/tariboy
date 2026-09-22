import { act, renderHook } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { setLocalBaseURL } from "@/lib/api"
import type { Daemon } from "@/lib/daemons"
import { messagesWsUrl, useMessagesSocket } from "./useMessagesSocket"

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  readonly url: string
  onopen: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null
  close = vi.fn()
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

describe("messages websocket", () => {
  it("builds local and remote URLs with the existing token convention", () => {
    expect(messagesWsUrl(null)).toMatch(/^ws:\/\/.*\/api\/messages\/ws$/)
    const remote: Daemon = {
      id: "prod",
      label: "Production",
      baseURL: "https://chat.example.test/",
      token: "secret token",
    }
    expect(messagesWsUrl(remote)).toBe(
      "wss://chat.example.test/api/messages/ws?token=secret+token",
    )
  })

  it("carries the chat id and keeps a chat frame that names no agent", () => {
    const onHint = vi.fn()
    renderHook(() => useMessagesSocket({ target: null, onHint }))
    const socket = FakeWebSocket.instances[0]
    act(() => {
      socket.onmessage?.(new MessageEvent("message", {
        data: JSON.stringify({ agent: "worker", chat: "tasks:worker", id: "m1", channel: "chat:tasks:worker", type: "task.assigned", from: "system:tasks", ts: "t" }),
      }))
      // A shared chat belongs to no single agent, so the frame names only the chat.
      socket.onmessage?.(new MessageEvent("message", {
        data: JSON.stringify({ chat: "release-team", id: "m2", channel: "chat:release-team", type: "message", from: "user:customer", ts: "t" }),
      }))
    })
    expect(onHint).toHaveBeenCalledTimes(2)
    expect(onHint.mock.calls[0][0].chat).toBe("tasks:worker")
    expect(onHint.mock.calls[1][0].chat).toBe("release-team")
  })

  it("delivers hints and refetches on every (re)connect instead of replaying", () => {
    vi.useFakeTimers()
    const onHint = vi.fn()
    const onOpen = vi.fn()
    renderHook(() => useMessagesSocket({ target: null, onHint, onOpen }))
    const first = FakeWebSocket.instances[0]

    act(() => { first.onopen?.(new Event("open")) })
    expect(onOpen).toHaveBeenCalledTimes(1)

    act(() => {
      first.onmessage?.(new MessageEvent("message", {
        data: JSON.stringify({ agent: "worker", id: "m1", channel: "user:customer", type: "message", from: "agent:worker", ts: "t" }),
      }))
      first.onmessage?.(new MessageEvent("message", { data: "not json" }))
      first.onmessage?.(new MessageEvent("message", { data: JSON.stringify({ id: "m2" }) }))
    })
    expect(onHint).toHaveBeenCalledTimes(1)
    expect(onHint.mock.calls[0][0].agent).toBe("worker")

    act(() => { first.onclose?.(new CloseEvent("close")) })
    act(() => { vi.advanceTimersByTime(300) })
    const second = FakeWebSocket.instances[1]
    expect(second).toBeDefined()
    act(() => { second.onopen?.(new Event("open")) })
    expect(onOpen).toHaveBeenCalledTimes(2)
  })
})
