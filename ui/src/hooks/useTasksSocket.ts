import { useEffect, useRef, useState } from "react"
import {
  getLocalBaseURL,
  resolveTarget,
  type ApiTarget,
} from "@/lib/api"
import type { Daemon } from "@/lib/daemons"
import type { TaskEventHint } from "@/lib/tasks"

const RECONNECT_MIN_MS = 250
const RECONNECT_MAX_MS = 5000

export type TasksSocketStatus = "connecting" | "open" | "closed"

export interface UseTasksSocketOptions {
  target?: ApiTarget
  after?: number
  enabled?: boolean
  onHint: (hint: TaskEventHint) => void
  onReset?: (sequence: number) => void
}

export function tasksWsUrl(target: Daemon | null, after: number): string {
  if (target && !target.baseURL) {
    throw new Error(`host ${target.label || target.id} is not ready`)
  }
  let base: string
  if (target) {
    base = target.baseURL.replace(/\/+$/, "").replace(/^http:/, "ws:").replace(/^https:/, "wss:")
  } else {
    const local = getLocalBaseURL()
    if (local) {
      base = local.replace(/\/+$/, "").replace(/^http:/, "ws:").replace(/^https:/, "wss:")
    } else {
      const scheme = location.protocol === "https:" ? "wss:" : "ws:"
      base = `${scheme}//${location.host}`
    }
  }
  const query = new URLSearchParams({ after: String(after) })
  if (target?.token) query.set("token", target.token)
  return `${base}/api/tasks/ws?${query.toString()}`
}

export function useTasksSocket({
  target,
  after = 0,
  enabled = true,
  onHint,
  onReset,
}: UseTasksSocketOptions): TasksSocketStatus {
  const [status, setStatus] = useState<TasksSocketStatus>(enabled ? "connecting" : "closed")
  const sequenceRef = useRef(after)
  const onHintRef = useRef(onHint)
  const onResetRef = useRef(onReset)
  useEffect(() => {
    onHintRef.current = onHint
    onResetRef.current = onReset
  }, [onHint, onReset])

  const resolved = resolveTarget(target)
  const targetKey = JSON.stringify(resolved)

  useEffect(() => {
    sequenceRef.current = after
  }, [after, targetKey])

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
        url = tasksWsUrl(socketTarget, sequenceRef.current)
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
      }
      current.onmessage = (event) => {
        if (!alive || typeof event.data !== "string") return
        let message: unknown
        try {
          message = JSON.parse(event.data)
        } catch {
          return
        }
        if (!message || typeof message !== "object") return
        const value = message as Record<string, unknown>
        const sequence = typeof value.sequence === "number" ? value.sequence : -1
        if (value.type === "reset") {
          onResetRef.current?.(sequence)
          return
        }
        if (
          value.type !== undefined && value.type !== "event"
          || sequence <= sequenceRef.current
          || typeof value.kind !== "string"
        ) return
        const hint = value as unknown as TaskEventHint
        sequenceRef.current = hint.sequence
        onHintRef.current(hint)
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
