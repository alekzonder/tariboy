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

export type MessagesSocketStatus = "connecting" | "open" | "closed"

export function messagesWsUrl(target: Daemon | null): string {
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
  return `${base}/api/messages/ws${qs ? `?${qs}` : ""}`
}

/**
 * Watch every chat on one host over a single socket. The socket replays
 * nothing, so the caller refetches on mount and on every reconnect; that, not a
 * cursor, is what makes a missed frame harmless.
 */
export function useMessagesSocket({
  target,
  enabled = true,
  onHint,
  onOpen,
}: {
  target?: ApiTarget
  enabled?: boolean
  onHint: (hint: MessageHint) => void
  onOpen?: () => void
}): MessagesSocketStatus {
  const [status, setStatus] = useState<MessagesSocketStatus>(enabled ? "connecting" : "closed")
  const onHintRef = useRef(onHint)
  const onOpenRef = useRef(onOpen)
  useEffect(() => {
    onHintRef.current = onHint
    onOpenRef.current = onOpen
  }, [onHint, onOpen])

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
        url = messagesWsUrl(socketTarget)
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
        const hint = parsed as Partial<MessageHint>
        // A shared chat belongs to no single agent, so a frame that names its
        // chat is addressed even without one.
        if (!hint || typeof hint.id !== "string") return
        if (typeof hint.agent !== "string" && typeof hint.chat !== "string") return
        onHintRef.current(hint as MessageHint)
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
  }, [enabled, targetKey])

  return status
}
