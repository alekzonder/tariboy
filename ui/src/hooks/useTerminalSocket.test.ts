import { describe, it, expect, beforeEach, afterEach, vi } from "vitest"
import { renderHook, act } from "@testing-library/react"
import { useTerminalSocket } from "@/hooks/useTerminalSocket"
import { terminalWsUrl } from "./useTerminalSocket"
import { setLocalBaseURL } from "@/lib/api"

/** Minimal fake WebSocket: captures the constructor args, exposes settable
 * on* handlers the test can fire directly, and spies on send()/close() so
 * assertions don't need a real network stack. */
class FakeWebSocket {
  static last: FakeWebSocket | undefined
  static instances: FakeWebSocket[] = []
  static OPEN = 1
  static CONNECTING = 0
  static CLOSED = 3

  url: string
  binaryType = ""
  readyState = FakeWebSocket.CONNECTING
  onopen: ((ev: Event) => void) | null = null
  onmessage: ((ev: MessageEvent) => void) | null = null
  onclose: ((ev: CloseEvent) => void) | null = null
  onerror: ((ev: Event) => void) | null = null
  send = vi.fn()
  close = vi.fn(() => {
    this.readyState = FakeWebSocket.CLOSED
  })

  constructor(url: string) {
    this.url = url
    FakeWebSocket.last = this
    FakeWebSocket.instances.push(this)
  }
}

beforeEach(() => {
  FakeWebSocket.last = undefined
  FakeWebSocket.instances = []
  vi.stubGlobal("WebSocket", FakeWebSocket)
})

function open(ws: FakeWebSocket) {
  ws.readyState = FakeWebSocket.OPEN
  ws.onopen?.(new Event("open"))
}

describe("useTerminalSocket", () => {
  it("opens a websocket and reports status open", () => {
    const { result } = renderHook(() => useTerminalSocket("agent-a", true))
    expect(FakeWebSocket.last).toBeDefined()
    expect(FakeWebSocket.last!.binaryType).toBe("arraybuffer")
    expect(FakeWebSocket.last!.url).toContain("/api/agents/agent-a/terminal")

    act(() => open(FakeWebSocket.last!))
    expect(result.current.status).toBe("open")
  })

  it("writes incoming binary frames to the attached terminal", () => {
    const { result } = renderHook(() => useTerminalSocket("agent-a", true))
    const write = vi.fn()
    act(() => result.current.attachTerm({ write } as unknown as import("@xterm/xterm").Terminal))
    act(() => open(FakeWebSocket.last!))

    const bytes = new Uint8Array([1, 2, 3]).buffer
    act(() => FakeWebSocket.last!.onmessage?.({ data: bytes } as MessageEvent))

    expect(write).toHaveBeenCalledTimes(1)
    const written = write.mock.calls[0][0] as Uint8Array
    expect(Array.from(written)).toEqual([1, 2, 3])
  })

  it("send() forwards typed text as a BINARY frame, not a text frame (C1)", () => {
    const { result } = renderHook(() => useTerminalSocket("agent-a", true))
    act(() => open(FakeWebSocket.last!))
    act(() => result.current.send("ls"))

    expect(FakeWebSocket.last!.send).toHaveBeenCalledTimes(1)
    const sent = FakeWebSocket.last!.send.mock.calls[0][0]
    // Must NOT be a string — a string transmits a TEXT frame, which the daemon
    // treats as a resize control message and drops, so keyboard input vanishes.
    // (TextEncoder yields a Uint8Array from another realm, so assert via
    // ArrayBuffer.isView rather than a realm-bound instanceof.)
    expect(typeof sent).not.toBe("string")
    expect(ArrayBuffer.isView(sent)).toBe(true)
    expect(new TextDecoder().decode(sent as Uint8Array)).toBe("ls")
  })

  it("send() encodes a control-byte hotkey string to raw bytes (C1)", () => {
    const { result } = renderHook(() => useTerminalSocket("agent-a", true))
    act(() => open(FakeWebSocket.last!))
    act(() => result.current.send("\x03")) // Ctrl-C

    const sent = FakeWebSocket.last!.send.mock.calls[0][0]
    expect(typeof sent).not.toBe("string")
    expect(ArrayBuffer.isView(sent)).toBe(true)
    expect(Array.from(sent as Uint8Array)).toEqual([3])
  })

  it("send() passes a Uint8Array through unchanged as binary", () => {
    const { result } = renderHook(() => useTerminalSocket("agent-a", true))
    act(() => open(FakeWebSocket.last!))
    const bytes = new Uint8Array([27, 91, 65]) // ESC [ A
    act(() => result.current.send(bytes))

    const sent = FakeWebSocket.last!.send.mock.calls[0][0]
    expect(sent).toBe(bytes)
    expect(Array.from(sent as Uint8Array)).toEqual([27, 91, 65])
  })

  it("sendResize() posts a JSON TEXT frame (resize stays the text channel)", () => {
    const { result } = renderHook(() => useTerminalSocket("agent-a", true))
    act(() => open(FakeWebSocket.last!))
    act(() => result.current.sendResize(100, 30))

    const sent = FakeWebSocket.last!.send.mock.calls[0][0]
    expect(typeof sent).toBe("string")
    expect(JSON.parse(sent as string)).toEqual({ cols: 100, rows: 30 })
  })

  it("emits the attached terminal's size as a text frame on open (I2)", () => {
    const { result } = renderHook(() => useTerminalSocket("agent-a", true))
    act(() =>
      result.current.attachTerm({
        cols: 120,
        rows: 40,
        write: vi.fn(),
      } as unknown as import("@xterm/xterm").Terminal),
    )
    act(() => open(FakeWebSocket.last!))

    const sent = FakeWebSocket.last!.send.mock.calls[0][0]
    expect(typeof sent).toBe("string")
    expect(JSON.parse(sent as string)).toEqual({ cols: 120, rows: 40 })
  })

  it("does not emit a resize on open when no term is attached", () => {
    renderHook(() => useTerminalSocket("agent-a", true))
    act(() => open(FakeWebSocket.last!))
    expect(FakeWebSocket.last!.send).not.toHaveBeenCalled()
  })

  it("does not emit a resize on open when the term has a zero size", () => {
    const { result } = renderHook(() => useTerminalSocket("agent-a", true))
    act(() =>
      result.current.attachTerm({
        cols: 0,
        rows: 0,
        write: vi.fn(),
      } as unknown as import("@xterm/xterm").Terminal),
    )
    act(() => open(FakeWebSocket.last!))
    expect(FakeWebSocket.last!.send).not.toHaveBeenCalled()
  })

  it("dials at the default 80x24 before any resize is known", () => {
    renderHook(() => useTerminalSocket("agent-a", true))
    expect(FakeWebSocket.last!.url).toContain("cols=80&rows=24")
  })

  it("dials the NEXT socket at the last requested size, not 80x24 (agent switch)", () => {
    // The daemon starts the `tmux attach` client at the URL's size and tmux
    // runs `window-size latest`, so re-dialing at 80x24 shrinks the whole
    // session's window — the visible bug when switching between agents.
    const { result, rerender } = renderHook(
      ({ n }: { n: string }) => useTerminalSocket(n, true),
      { initialProps: { n: "agent-a" } },
    )
    act(() => open(FakeWebSocket.last!))
    act(() => result.current.sendResize(203, 51))

    rerender({ n: "agent-b" })
    expect(FakeWebSocket.last!.url).toContain("/api/agents/agent-b/terminal")
    expect(FakeWebSocket.last!.url).toContain("cols=203&rows=51")
  })

  it("remembers a resize computed while the socket is down", () => {
    // fit() can land while the ws is CONNECTING (its frame is dropped) — the
    // size must still be carried into the next dial.
    const { result, rerender } = renderHook(
      ({ n }: { n: string }) => useTerminalSocket(n, true),
      { initialProps: { n: "agent-a" } },
    )
    act(() => result.current.sendResize(150, 45)) // never opened: nothing sent
    expect(FakeWebSocket.last!.send).not.toHaveBeenCalled()

    rerender({ n: "agent-b" })
    expect(FakeWebSocket.last!.url).toContain("cols=150&rows=45")
  })

  it("does not open a socket when disabled", () => {
    renderHook(() => useTerminalSocket("agent-a", false))
    expect(FakeWebSocket.last).toBeUndefined()
  })

  describe("reconnect backoff", () => {
    beforeEach(() => {
      vi.useFakeTimers()
    })

    afterEach(() => {
      vi.useRealTimers()
    })

    it("recovers when a 4404 startup race is followed by a live session", () => {
      const { result } = renderHook(() => useTerminalSocket("agent-a", true))

      act(() => {
        FakeWebSocket.instances[0].onclose?.({ code: 4404 } as CloseEvent)
      })
      expect(result.current.absent).toBe(false)

      act(() => {
        vi.advanceTimersByTime(250)
      })
      expect(FakeWebSocket.instances.length).toBe(2)

      act(() => open(FakeWebSocket.instances[1]))
      expect(result.current.status).toBe("open")
      expect(result.current.absent).toBe(false)
    })

    it("reconnects with growing backoff on non-4404 closes", () => {
      renderHook(() => useTerminalSocket("agent-a", true))
      expect(FakeWebSocket.instances.length).toBe(1)

      // First close: delay should be the initial 250ms.
      act(() => {
        FakeWebSocket.instances[0].onclose?.({ code: 1006 } as CloseEvent)
      })
      act(() => {
        vi.advanceTimersByTime(249)
      })
      expect(FakeWebSocket.instances.length).toBe(1)
      act(() => {
        vi.advanceTimersByTime(1)
      })
      expect(FakeWebSocket.instances.length).toBe(2)

      // Second close: delay should have doubled to 500ms.
      act(() => {
        FakeWebSocket.instances[1].onclose?.({ code: 1006 } as CloseEvent)
      })
      act(() => {
        vi.advanceTimersByTime(250)
      })
      expect(FakeWebSocket.instances.length).toBe(2)
      act(() => {
        vi.advanceTimersByTime(250)
      })
      expect(FakeWebSocket.instances.length).toBe(3)
    })

    it("caps backoff at RECONNECT_MAX_MS (2000ms)", () => {
      renderHook(() => useTerminalSocket("agent-a", true))

      // Drive several consecutive closes: 250 -> 500 -> 1000 -> 2000 -> 2000 (capped).
      const delays = [250, 500, 1000, 2000, 2000]
      for (let i = 0; i < delays.length; i++) {
        const ws = FakeWebSocket.instances[FakeWebSocket.instances.length - 1]
        act(() => {
          ws.onclose?.({ code: 1006 } as CloseEvent)
        })
        act(() => {
          vi.advanceTimersByTime(delays[i])
        })
        expect(FakeWebSocket.instances.length).toBe(i + 2)
      }

      // One more round at the cap: still reconnects at exactly 2000ms, no runaway growth.
      const ws = FakeWebSocket.instances[FakeWebSocket.instances.length - 1]
      act(() => {
        ws.onclose?.({ code: 1006 } as CloseEvent)
      })
      act(() => {
        vi.advanceTimersByTime(1999)
      })
      expect(FakeWebSocket.instances.length).toBe(delays.length + 1)
      act(() => {
        vi.advanceTimersByTime(1)
      })
      expect(FakeWebSocket.instances.length).toBe(delays.length + 2)
    })

    it("resets backoff to the minimum after a successful reconnect", () => {
      renderHook(() => useTerminalSocket("agent-a", true))

      // First close/reconnect at 250ms.
      act(() => {
        FakeWebSocket.instances[0].onclose?.({ code: 1006 } as CloseEvent)
      })
      act(() => {
        vi.advanceTimersByTime(250)
      })
      expect(FakeWebSocket.instances.length).toBe(2)

      // The reconnected socket opens successfully, which should reset the backoff.
      act(() => open(FakeWebSocket.instances[1]))

      // Close again: delay should be back at the 250ms minimum, not the grown 500ms.
      act(() => {
        FakeWebSocket.instances[1].onclose?.({ code: 1006 } as CloseEvent)
      })
      act(() => {
        vi.advanceTimersByTime(249)
      })
      expect(FakeWebSocket.instances.length).toBe(2)
      act(() => {
        vi.advanceTimersByTime(1)
      })
      expect(FakeWebSocket.instances.length).toBe(3)
    })

    it("does not reconnect after unmount and closes the socket", () => {
      const consoleError = vi.spyOn(console, "error").mockImplementation(() => {})
      const { unmount } = renderHook(() => useTerminalSocket("agent-a", true))
      const ws = FakeWebSocket.instances[0]
      act(() => open(ws))

      unmount()
      expect(ws.close).toHaveBeenCalledTimes(1)

      // Fire a close event after unmount, as a real socket might during teardown.
      act(() => {
        ws.onclose?.({ code: 1006 } as CloseEvent)
      })
      act(() => {
        vi.advanceTimersByTime(5000)
      })

      expect(FakeWebSocket.instances.length).toBe(1)
      const stateWarnings = consoleError.mock.calls.filter((c) =>
        String(c[0]).includes("state update"),
      )
      expect(stateWarnings.length).toBe(0)
      consoleError.mockRestore()
    })

    it("marks absent and stops after 4404 persists through the startup grace", () => {
      const { result } = renderHook(() => useTerminalSocket("agent-a", true))

      act(() => {
        FakeWebSocket.instances[0].onclose?.({ code: 4404 } as CloseEvent)
      })
      act(() => {
        vi.advanceTimersByTime(250)
      })
      expect(FakeWebSocket.instances.length).toBe(2)
      act(() => {
        FakeWebSocket.instances[1].onclose?.({ code: 4404 } as CloseEvent)
        vi.advanceTimersByTime(500)
      })
      expect(FakeWebSocket.instances.length).toBe(3)
      act(() => {
        FakeWebSocket.instances[2].onclose?.({ code: 4404 } as CloseEvent)
        vi.advanceTimersByTime(1000)
      })
      expect(FakeWebSocket.instances.length).toBe(4)
      act(() => {
        FakeWebSocket.instances[3].onclose?.({ code: 4404 } as CloseEvent)
        vi.advanceTimersByTime(2000)
      })
      expect(FakeWebSocket.instances.length).toBe(5)
      act(() => {
        FakeWebSocket.instances[4].onclose?.({ code: 4404 } as CloseEvent)
        vi.advanceTimersByTime(1250)
      })
      expect(FakeWebSocket.instances.length).toBe(6)
      act(() => {
        FakeWebSocket.instances[5].onclose?.({ code: 4404 } as CloseEvent)
      })

      expect(result.current.absent).toBe(true)
      expect(result.current.status).toBe("closed")
      act(() => {
        vi.advanceTimersByTime(5000)
      })
      expect(FakeWebSocket.instances.length).toBe(6)
    })

    it("does not reconnect after the terminal PTY reaches normal EOF", () => {
      const { result } = renderHook(() => useTerminalSocket("agent-a", true))
      const ws = FakeWebSocket.instances[0]
      act(() => open(ws))
      act(() => {
        ws.onclose?.({ code: 1000, reason: "eof" } as CloseEvent)
      })

      act(() => {
        vi.advanceTimersByTime(5000)
      })

      expect(FakeWebSocket.instances.length).toBe(1)
      expect(result.current.absent).toBe(true)
      expect(result.current.status).toBe("closed")
    })

    it("still reconnects after a normal close without the terminal EOF reason", () => {
      const { result } = renderHook(() => useTerminalSocket("agent-a", true))
      const ws = FakeWebSocket.instances[0]
      act(() => open(ws))
      act(() => {
        ws.onclose?.({ code: 1000, reason: "" } as CloseEvent)
      })

      act(() => {
        vi.advanceTimersByTime(250)
      })

      expect(FakeWebSocket.instances.length).toBe(2)
      expect(result.current.absent).toBe(false)
      expect(result.current.status).toBe("connecting")
    })

    it("reconnect() clears absent and dials a new socket after a 4404 close", () => {
      const { result } = renderHook(() => useTerminalSocket("agent-a", true))
      const ws = FakeWebSocket.instances[0]
      act(() => open(ws))
      act(() => {
        // Let the startup grace expire while the socket is otherwise open,
        // then model the daemon reporting that the session never appeared.
        vi.advanceTimersByTime(5000)
        ws.onclose?.({ code: 4404 } as CloseEvent)
      })
      expect(result.current.absent).toBe(true)
      expect(FakeWebSocket.instances.length).toBe(1)

      act(() => {
        result.current.reconnect()
      })

      expect(result.current.absent).toBe(false)
      expect(FakeWebSocket.instances.length).toBe(2)
      expect(FakeWebSocket.instances[1]).not.toBe(ws)
    })

    it("reconnect() opens no socket while disabled", () => {
      const { result } = renderHook(() => useTerminalSocket("agent-a", false))
      expect(FakeWebSocket.instances.length).toBe(0)

      act(() => {
        result.current.reconnect()
      })

      expect(FakeWebSocket.instances.length).toBe(0)
    })

    it("re-emits the terminal size on open after a backoff reconnect (I2)", () => {
      const { result } = renderHook(() => useTerminalSocket("agent-a", true))
      act(() =>
        result.current.attachTerm({
          cols: 120,
          rows: 40,
          write: vi.fn(),
        } as unknown as import("@xterm/xterm").Terminal),
      )

      // First socket opens and emits the size.
      act(() => open(FakeWebSocket.instances[0]))
      expect(FakeWebSocket.instances[0].send).toHaveBeenCalledTimes(1)
      expect(JSON.parse(FakeWebSocket.instances[0].send.mock.calls[0][0] as string)).toEqual({
        cols: 120,
        rows: 40,
      })

      // Transient (non-4404) close triggers a backoff reconnect.
      act(() => {
        FakeWebSocket.instances[0].onclose?.({ code: 1006 } as CloseEvent)
      })
      act(() => {
        vi.advanceTimersByTime(250)
      })
      expect(FakeWebSocket.instances.length).toBe(2)

      // The new socket must re-send the size on its own open — the mount effect
      // never re-runs, so this is the only path that keeps tmux in sync.
      act(() => open(FakeWebSocket.instances[1]))
      expect(FakeWebSocket.instances[1].send).toHaveBeenCalledTimes(1)
      expect(JSON.parse(FakeWebSocket.instances[1].send.mock.calls[0][0] as string)).toEqual({
        cols: 120,
        rows: 40,
      })
    })

    it("keeps input working when the OLD socket's close lands after the switch", () => {
      // Agent switch closes socket A and dials socket B in the same commit, but
      // A's close event only fires a tick later — after wsRef already points at
      // B. A stale close handler must not clear the live socket, or every
      // keystroke is silently dropped while output keeps streaming in.
      const { result, rerender } = renderHook(({ name }) => useTerminalSocket(name, true), {
        initialProps: { name: "agent-a" },
      })
      const first = FakeWebSocket.instances[0]
      act(() => open(first))

      rerender({ name: "agent-b" })
      const second = FakeWebSocket.instances[1]
      act(() => open(second))

      // Now the old socket's close finally arrives.
      act(() => {
        first.onclose?.({ code: 1006 } as CloseEvent)
      })

      act(() => result.current.send("ls"))
      expect(second.send).toHaveBeenCalledTimes(1)
      const sent = second.send.mock.calls[0][0]
      expect(new TextDecoder().decode(sent as Uint8Array)).toBe("ls")
    })

    it("tears down the old socket and opens a new one when name changes", () => {
      const { rerender } = renderHook(({ name }) => useTerminalSocket(name, true), {
        initialProps: { name: "agent-a" },
      })
      const first = FakeWebSocket.instances[0]
      act(() => open(first))
      expect(first.url).toContain("/api/agents/agent-a/terminal")

      rerender({ name: "agent-b" })

      expect(first.close).toHaveBeenCalledTimes(1)
      expect(FakeWebSocket.instances.length).toBe(2)
      const second = FakeWebSocket.instances[1]
      expect(second.url).toContain("/api/agents/agent-b/terminal")
    })
  })
})

describe("terminalWsUrl", () => {
  it("same-origin without target: relative host, no token", () => {
    const url = terminalWsUrl("a b", 80, 24)
    expect(url).toContain("/api/agents/a%20b/terminal?cols=80&rows=24")
    expect(url).not.toContain("token=")
  })
  it("cross-origin target: ws(s) scheme from baseURL + token query", () => {
    const url = terminalWsUrl("bob", 120, 40, {
      id: "d1", label: "prod", baseURL: "https://prod:8765", token: "s3kret",
    })
    expect(url).toBe("wss://prod:8765/api/agents/bob/terminal?cols=120&rows=40&token=s3kret")
  })
  it("http baseURL maps to ws", () => {
    const url = terminalWsUrl("bob", 80, 24, {
      id: "d1", label: "lan", baseURL: "http://10.0.0.2:9990", token: "t",
    })
    expect(url.startsWith("ws://10.0.0.2:9990/")).toBe(true)
  })
  it("target without token omits the query token", () => {
    const url = terminalWsUrl("bob", 80, 24, { id: "d", label: "x", baseURL: "https://h", token: "" })
    expect(url).not.toContain("token=")
  })

  it("fails closed instead of dialing local for an unresolved remote target", () => {
    expect(() =>
      terminalWsUrl("bob", 80, 24, { id: "missing", label: "missing", baseURL: "", token: "" }),
    ).toThrow(/missing.*not ready/)
  })

  it("dials the configured local origin when there is no explicit target", () => {
    setLocalBaseURL("http://127.0.0.1:9993")
    try {
      expect(terminalWsUrl("bob", 80, 24)).toBe(
        "ws://127.0.0.1:9993/api/agents/bob/terminal?cols=80&rows=24",
      )
    } finally {
      setLocalBaseURL("")
    }
  })

  it("falls back to the page location when no local origin is set", () => {
    setLocalBaseURL("")
    expect(terminalWsUrl("bob", 80, 24)).toContain("/api/agents/bob/terminal")
    expect(terminalWsUrl("bob", 80, 24).startsWith("ws")).toBe(true)
  })
})
