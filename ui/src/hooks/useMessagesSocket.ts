import { useEffect, useRef, useState } from "react"
import { getLocalBaseURL, resolveTarget, type ApiTarget } from "@/lib/api"
import type { Daemon } from "@/lib/daemons"

const RECONNECT_MIN_MS = 250
const RECONNECT_MAX_MS = 5000

/** One live publication on this host. It is a hint, not the message: the client
 *  refetches over HTTP, which stays authoritative. */
export interface MessageHint {
  agent: string
  /** The chat the publication landed in, absent for a message on a channel no
   *  chat owns. A client that knows it refetches only that conversation. */
  chat?: string
  id: string
  channel: string
  type: string
  from: string
  ts: string
}

/** Something that can change an agent's state happened: an iteration started
 *  or ended, or a lifecycle event was recorded. The client refetches. */
export interface AgentStateHint {
  agent: string
  type: string
  ts: string
}

export type MessagesSocketStatus = "connecting" | "open" | "closed"

function hintWsUrl(target: Daemon | null, path: string): string {
  if (target && !target.baseURL) {
    throw new Error(`host ${target.label || target.id} is not ready`)
  }
  const toWs = (url: string) =>
    url.replace(/\/+$/, "").replace(/^http:/, "ws:").replace(/^https:/, "wss:")
  let base: string
  if (target) {
    base = toWs(target.baseURL)
  } else {
    const local = getLocalBaseURL()
    base = local
      ? toWs(local)
      : `${location.protocol === "https:" ? "wss:" : "ws:"}//${location.host}`
  }
  const query = new URLSearchParams()
  if (target?.token) query.set("token", target.token)
  const qs = query.toString()
  return `${base}${path}${qs ? `?${qs}` : ""}`
}

export function messagesWsUrl(target: Daemon | null): string {
  return hintWsUrl(target, "/api/messages/ws")
}

type HintSocketOptions<T> = {
  target?: ApiTarget
  enabled?: boolean
  onHint: (hint: T) => void
  onOpen?: () => void
}

/**
 * Watch every chat on one host over a single socket. The socket replays
 * nothing, so the caller refetches on mount and on every reconnect; that, not a
 * cursor, is what makes a missed frame harmless.
 */
export function useMessagesSocket(options: HintSocketOptions<MessageHint>): MessagesSocketStatus {
  return useHintSocket("/api/messages/ws", (parsed) => {
    const hint = parsed as Partial<MessageHint>
    // A shared chat belongs to no single agent, so a frame that names its
    // chat is addressed even without one.
    if (!hint || typeof hint.id !== "string") return null
    if (typeof hint.agent !== "string" && typeof hint.chat !== "string") return null
    return hint as MessageHint
  }, options)
}

/**
 * Watch every agent's state on one host. While it is open, the agents list
 * and the agent status can poll slowly and refetch on a hint instead. A host
 * without this socket keeps it closed, and the caller keeps its short poll.
 */
export function useAgentStatesSocket(options: HintSocketOptions<AgentStateHint>): MessagesSocketStatus {
  return useHintSocket("/api/agent-states/ws", (parsed) => {
    const hint = parsed as Partial<AgentStateHint>
    return hint && typeof hint.agent === "string" ? (hint as AgentStateHint) : null
  }, options)
}

function useHintSocket<T>(
  path: string,
  accept: (parsed: unknown) => T | null,
  { target, enabled = true, onHint, onOpen }: HintSocketOptions<T>,
): MessagesSocketStatus {
  const [status, setStatus] = useState<MessagesSocketStatus>(enabled ? "connecting" : "closed")
  const onHintRef = useRef(onHint)
  const onOpenRef = useRef(onOpen)
  const acceptRef = useRef(accept)
  useEffect(() => {
    onHintRef.current = onHint
    onOpenRef.current = onOpen
    acceptRef.current = accept
  }, [onHint, onOpen, accept])

  const targetKey = JSON.stringify(resolveTarget(target))

  useEffect(() => {
    if (!enabled) {
      queueMicrotask(() => setStatus("closed"))
      return
    }
    let alive = true
    let socket: WebSocket | null = null
    let timer: ReturnType<typeof setTimeout> | undefined
    let reconnectDelay = RECONNECT_MIN_MS
    const socketTarget = JSON.parse(targetKey) as Daemon | null

    const connect = () => {
      if (!alive) return
      let url: string
      try {
        url = hintWsUrl(socketTarget, path)
      } catch {
        setStatus("closed")
        return
      }
      setStatus("connecting")
      const current = new WebSocket(url)
      socket = current
      current.onopen = () => {
        if (!alive || socket !== current) return
        reconnectDelay = RECONNECT_MIN_MS
        setStatus("open")
        onOpenRef.current?.()
      }
      current.onmessage = (event) => {
        if (!alive || typeof event.data !== "string") return
        let parsed: unknown
        try {
          parsed = JSON.parse(event.data)
        } catch {
          return
        }
        const hint = acceptRef.current(parsed)
        if (hint !== null) onHintRef.current(hint)
      }
      current.onclose = () => {
        if (socket === current) socket = null
        if (!alive) return
        setStatus("closed")
        const wait = reconnectDelay
        reconnectDelay = Math.min(reconnectDelay * 2, RECONNECT_MAX_MS)
        timer = setTimeout(connect, wait)
      }
    }

    connect()
    return () => {
      alive = false
      if (timer !== undefined) clearTimeout(timer)
      socket?.close()
      socket = null
    }
  }, [enabled, targetKey, path])

  return status
}
