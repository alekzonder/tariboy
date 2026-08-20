import { useCallback, useEffect, useRef, useState } from "react"
import type { Terminal } from "@xterm/xterm"
import type { Daemon } from "@/lib/daemons"
import { getLocalBaseURL } from "@/lib/api"

/** Connection lifecycle exposed to the Terminal tab. "connecting" covers both
 * the initial dial and any backoff-driven reconnect attempt. */
export type TerminalSocketStatus = "connecting" | "open" | "closed"

/** Everything the Terminal tab needs to drive a live xterm.js session over
 * `GET /api/agents/<name>/terminal`: connection status, whether the daemon
 * reports no interactive session at all, byte/resize senders, and a way to
 * hand the hook the mounted `Terminal` instance so incoming frames can be
 * written to it directly. */
export interface TerminalSocketController {
  status: TerminalSocketStatus
  absent: boolean
  send: (data: string | Uint8Array) => void
  sendResize: (cols: number, rows: number) => void
  attachTerm: (t: Terminal) => void
  name: string
  /** Force the connect effect to tear down and re-dial immediately (backoff
   * reset to the minimum delay), clearing `absent` first. Intended for a
   * caller that just took an action which may have made a session appear
   * again (e.g. restarting the agent from the "Session not running" panel).
   * The fresh lifecycle includes the same bounded startup grace as an initial
   * connection; it never turns into indefinite no-session polling. */
  reconnect: () => void
}

const INITIAL_COLS = 80
const INITIAL_ROWS = 24
const RECONNECT_MIN_MS = 250
const RECONNECT_MAX_MS = 2000
const ABSENT_GRACE_MS = 5000
// The daemon closes with this code when there is no interactive session for
// the agent (not running, not a tmux/interactive agent, or the freshly queued
// interactive iteration has not published its shim socket yet). It is
// provisional during the bounded startup grace below.
const CLOSE_CODE_ABSENT = 4404
// Once an attached PTY reaches EOF, the daemon closes the websocket normally
// with this reason. That means the tmux session ended; retrying would only
// attach to a missing session and print the same diagnostic into xterm.
const CLOSE_REASON_EOF = "eof"

/** Build the terminal websocket URL. `target` undefined/null keeps today's
 * same-origin, no-token URL; a `Daemon` target dials that host's baseURL
 * (http(s) mapped to ws(s)) with the bearer token riding as a `?token=` query
 * param — a browser WebSocket cannot set an Authorization header, so this
 * mirrors the cross-origin SSE `?token=` convention (same TLS caveat). */
export function terminalWsUrl(name: string, cols: number, rows: number, target?: Daemon | null): string {
  const path = `/api/agents/${encodeURIComponent(name)}/terminal?cols=${cols}&rows=${rows}`
  const tok = target?.token ? `&token=${encodeURIComponent(target.token)}` : ""
  if (target && !target.baseURL) {
    throw new Error(`host ${target.label || target.id} is not ready`)
  }
  if (target) {
    return target.baseURL.replace(/\/+$/, "").replace(/^http/, "ws") + path + tok
  }
  // The desktop app's page origin (tauri://localhost) is not a valid WS origin,
  // so the local daemon's explicit http://127.0.0.1:PORT is mapped to ws://.
  // In a browser this is "" and the page location is used, as before.
  const local = getLocalBaseURL()
  if (local) return local.replace(/^http/, "ws") + path + tok
  const scheme = location.protocol === "https:" ? "wss:" : "ws:"
  return scheme + "//" + location.host + path + tok
}

/** Open and maintain a raw-byte websocket to an agent's interactive terminal.
 * Binary frames stream both directions (PTY bytes); a JSON text frame
 * `{cols,rows}` requests a resize. A 4404 close means the daemon has no
 * interactive session to attach to yet, so it is retried for a bounded startup
 * grace before surfacing `absent`. A normal `1000/eof` close means an attached
 * PTY ended and stops immediately. Transport failures are retried with capped
 * exponential backoff while `enabled` stays true and the component is mounted.
 */
export function useTerminalSocket(name: string, enabled: boolean, target?: Daemon | null): TerminalSocketController {
  const [status, setStatus] = useState<TerminalSocketStatus>("connecting")
  const [absent, setAbsent] = useState(false)
  const wsRef = useRef<WebSocket | null>(null)
  const termRef = useRef<Terminal | null>(null)
  const reconnectDelayRef = useRef(RECONNECT_MIN_MS)
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  // Bumping this forces the connect effect below to tear down and re-dial;
  // see `reconnect()`.
  const [retryNonce, setRetryNonce] = useState(0)
  // Last size the UI asked for, remembered even while the socket is down. Every
  // dial (agent switch, backoff reconnect, redial) starts from it instead of the
  // 80x24 default: the daemon attaches a fresh `tmux attach` client at the URL's
  // size and tmux runs `window-size latest`, so dialing at 80x24 visibly shrinks
  // the whole session's window until a later resize frame lands.
  const sizeRef = useRef({ cols: INITIAL_COLS, rows: INITIAL_ROWS })

  const attachTerm = useCallback((t: Terminal) => {
    termRef.current = t
  }, [])

  const reconnect = useCallback(() => {
    reconnectDelayRef.current = RECONNECT_MIN_MS
    setAbsent(false)
    setRetryNonce((n) => n + 1)
  }, [])

  // Serialize the target so the connect effect keys off its VALUE (host +
  // token), not object identity — a caller re-rendering with a fresh but
  // equivalent Daemon object must not force a re-dial.
  const targetKey = JSON.stringify(target ?? null)

  useEffect(() => {
    if (!enabled) return
    let alive = true
    const absentDeadline = Date.now() + ABSENT_GRACE_MS
    let absentDelay = RECONNECT_MIN_MS
    const socketTarget = JSON.parse(targetKey) as Daemon | null
    if (socketTarget && !socketTarget.baseURL) {
      queueMicrotask(() => {
        if (!alive) return
        setAbsent(false)
        setStatus("closed")
      })
      return () => {
        alive = false
      }
    }
    queueMicrotask(() => {
      if (alive) setAbsent(false)
    })

    const connect = () => {
      if (!alive) return
      setStatus("connecting")
      const ws = new WebSocket(terminalWsUrl(name, sizeRef.current.cols, sizeRef.current.rows, socketTarget))
      ws.binaryType = "arraybuffer"
      wsRef.current = ws

      ws.onopen = () => {
        if (!alive) return
        reconnectDelayRef.current = RECONNECT_MIN_MS
        setStatus("open")
        // Emit the current terminal size on EVERY (re)connect. The mount-effect's
        // fit() runs while the ws is still CONNECTING (dropped), and it never
        // re-runs on a backoff reconnect — so without this the fresh `tmux
        // attach` client stays at the URL's default 80x24 while xterm shows the
        // fitted size, clipping full-screen apps until a manual resize. Reuse
        // the resize text-frame channel; guard against an unattached/zero-sized
        // term (the dial URL already carried `sizeRef`, and the ResizeObserver
        // will follow up once layout settles).
        const term = termRef.current
        if (term && term.cols > 0 && term.rows > 0) {
          ws.send(JSON.stringify({ cols: term.cols, rows: term.rows }))
        }
      }

      ws.onmessage = (ev) => {
        if (!alive) return
        if (ev.data instanceof ArrayBuffer) {
          termRef.current?.write(new Uint8Array(ev.data))
        }
      }

      ws.onclose = (ev) => {
        // Only disown the socket if it is still the current one. On an agent
        // switch the effect closes socket A and dials socket B in the same
        // commit, but A's close event arrives a tick later — by then wsRef
        // holds B, and clearing it would make send() drop every keystroke
        // while output kept streaming in over B.
        if (wsRef.current === ws) wsRef.current = null
        if (!alive) return
        if (ev.code === CLOSE_CODE_ABSENT) {
          const remaining = absentDeadline - Date.now()
          if (remaining > 0) {
            setStatus("connecting")
            const delay = Math.min(absentDelay, remaining)
            absentDelay = Math.min(absentDelay * 2, RECONNECT_MAX_MS)
            reconnectTimerRef.current = setTimeout(connect, delay)
            return
          }
          setAbsent(true)
          setStatus("closed")
          return
        }
        if (ev.code === 1000 && ev.reason === CLOSE_REASON_EOF) {
          setAbsent(true)
          setStatus("closed")
          return
        }
        setStatus("closed")
        const delay = reconnectDelayRef.current
        reconnectDelayRef.current = Math.min(delay * 2, RECONNECT_MAX_MS)
        reconnectTimerRef.current = setTimeout(connect, delay)
      }
    }

    connect()

    return () => {
      alive = false
      if (reconnectTimerRef.current !== undefined) clearTimeout(reconnectTimerRef.current)
      reconnectDelayRef.current = RECONNECT_MIN_MS
      wsRef.current?.close()
      wsRef.current = null
    }
  }, [name, enabled, retryNonce, targetKey])

  const send = useCallback((data: string | Uint8Array) => {
    const ws = wsRef.current
    if (!ws || ws.readyState !== WebSocket.OPEN) return
    // Stdin MUST go out as a BINARY frame: the daemon writes binary frames to
    // the PTY and treats every TEXT frame as a {cols,rows} resize control
    // message (discarding it if it isn't valid resize JSON). ws.send(aString)
    // transmits a TEXT frame, so a raw string would silently vanish into the
    // resize branch. Encode strings to UTF-8 bytes first — correct for typed
    // text, control sequences ("\x03"→[3]), and xterm's default SGR mouse mode.
    // Cast to BufferSource: Uint8Array's default ArrayBufferLike generic doesn't
    // structurally match WebSocket.send's param under strict lib.dom typings,
    // but the runtime accepts any typed-array view.
    ws.send(
      (typeof data === "string" ? new TextEncoder().encode(data) : data) as string | BufferSource,
    )
  }, [])

  // Record the size unconditionally — a resize computed while the socket is
  // connecting/closed is exactly the one the next dial must start from — then
  // push it over the wire if there is an open socket to push it on.
  const sendResize = useCallback((cols: number, rows: number) => {
    if (cols > 0 && rows > 0) sizeRef.current = { cols, rows }
    const ws = wsRef.current
    if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ cols, rows }))
  }, [])

  return { status, absent, send, sendResize, attachTerm, name, reconnect }
}
