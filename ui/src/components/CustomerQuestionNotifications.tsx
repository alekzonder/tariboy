import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react"
import { useNavigate } from "react-router-dom"
import { useDaemons } from "@/components/DaemonProvider"
import { useTasksSocket } from "@/hooks/useTasksSocket"
import { onTaskNotificationActivated, showTaskNotification } from "@/lib/desktop"
import { resolveDaemon, type Daemon } from "@/lib/daemons"
import {
  listTaskNotifications,
  listTasks,
  markTaskNotificationRead,
  type TaskNotification,
} from "@/lib/tasks"
import { serverPath } from "@/lib/terminalsHost"
import {
  CustomerQuestionNotificationsContext,
  type CustomerQuestionNotificationsValue,
} from "./customerQuestionNotificationsContext"
import {
  CUSTOMER_QUESTION_RETRY_MS,
  customerQuestionAttentionKey,
  requestingAgent,
} from "./customerQuestionNotificationModel"

type HostSnapshot = {
  attention: ReadonlySet<string>
}

type FreshQuestion = {
  notification: TaskNotification
  agentName: string
}

const CUSTOMER_QUESTION_ACTIVATION_TIMEOUT_MS = 1000

function withTimeout<T>(operation: Promise<T>, timeoutMs: number): Promise<T> {
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error("customer question notification request timed out")), timeoutMs)
    operation.then(
      (value) => {
        clearTimeout(timeout)
        resolve(value)
      },
      (error: unknown) => {
        clearTimeout(timeout)
        reject(error)
      },
    )
  })
}

export function CustomerQuestionNotifications({ children }: { children: ReactNode }) {
  const { daemons } = useDaemons()
  const navigate = useNavigate()
  const refreshers = useRef(new Map<string, () => Promise<void>>())
  const [snapshots, setSnapshots] = useState(new Map<string, HostSnapshot>())
  const hosts = useMemo(
    () => [{ id: "", label: "This daemon (local)" }, ...daemons.map((host) => ({
      id: host.id,
      label: host.label,
    }))],
    [daemons],
  )

  const receiveSnapshot = useCallback((hostId: string, label: string, attention: ReadonlySet<string>, fresh: FreshQuestion[]) => {
    setSnapshots((current) => {
      const next = new Map(current)
      next.set(hostId, { attention })
      return next
    })
    for (const { notification, agentName } of fresh) {
      void showTaskNotification({
        host_id: hostId,
        notification_id: notification.id,
        task_key: notification.task_key,
        server_label: label,
        agent_name: agentName,
      }).catch(() => {
        // A denied/unavailable native notification cannot change the HTTP snapshot.
      })
    }
  }, [])

  const removeSnapshot = useCallback((hostId: string) => {
    setSnapshots((current) => {
      if (!current.has(hostId)) return current
      const next = new Map(current)
      next.delete(hostId)
      return next
    })
    refreshers.current.delete(hostId)
  }, [])

  const registerRefresher = useCallback((hostId: string, refresh: (() => Promise<void>) | null) => {
    if (refresh) refreshers.current.set(hostId, refresh)
    else refreshers.current.delete(hostId)
  }, [])

  const refreshHost = useCallback(async (hostId: string) => {
    await refreshers.current.get(hostId)?.()
  }, [])

  const attention = useMemo(() => {
    const next = new Set<string>()
    for (const snapshot of snapshots.values()) {
      for (const key of snapshot.attention) next.add(key)
    }
    return next
  }, [snapshots])
  const value = useMemo<CustomerQuestionNotificationsValue>(
    () => ({ attention, refreshHost }),
    [attention, refreshHost],
  )

  useEffect(() => onTaskNotificationActivated((activation) => {
    void (async () => {
      let target: Daemon | null | undefined
      try {
        target = activation.host_id === "" ? null : await withTimeout(
          resolveDaemon(activation.host_id),
          CUSTOMER_QUESTION_ACTIVATION_TIMEOUT_MS,
        )
        if (activation.host_id !== "" && (!target || target.id !== activation.host_id)) return
        await withTimeout(
          markTaskNotificationRead(activation.notification_id, target),
          CUSTOMER_QUESTION_ACTIVATION_TIMEOUT_MS,
        )
      } catch {
        // The route still identifies the original host and task, so the user
        // can answer even when marking the inbox row read was interrupted.
      } finally {
        navigate(`${serverPath(activation.host_id, "tasks")}?task=${encodeURIComponent(activation.task_key)}`)
        void withTimeout(refreshHost(activation.host_id), CUSTOMER_QUESTION_ACTIVATION_TIMEOUT_MS).catch(() => {})
      }
    })()
  }), [navigate, refreshHost])

  return (
    <CustomerQuestionNotificationsContext.Provider value={value}>
      {hosts.map((host) => (
        <HostQuestionWatcher
          key={host.id || "__local__"}
          hostId={host.id}
          label={host.label}
          onSnapshot={receiveSnapshot}
          onRemove={removeSnapshot}
          registerRefresher={registerRefresher}
        />
      ))}
      {children}
    </CustomerQuestionNotificationsContext.Provider>
  )
}

function HostQuestionWatcher({
  hostId,
  label,
  onSnapshot,
  onRemove,
  registerRefresher,
}: {
  hostId: string
  label: string
  onSnapshot: (hostId: string, label: string, attention: ReadonlySet<string>, fresh: FreshQuestion[]) => void
  onRemove: (hostId: string) => void
  registerRefresher: (hostId: string, refresh: (() => Promise<void>) | null) => void
}) {
  const [target, setTarget] = useState<Daemon | null | undefined>(undefined)
  const [health, setHealth] = useState<"healthy" | "recovering">("recovering")
  const [sequence, setSequence] = useState(0)
  const [socketEnabled, setSocketEnabled] = useState(false)
  const [retryRequest, setRetryRequest] = useState<{ generation: number; revision: number } | null>(null)
  const observedIds = useRef(new Set<string>())
  const refreshState = useRef({ running: false, pending: false })
  const retryTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const generationRef = useRef(0)
  const labelRef = useRef(label)
  const refreshTargetRef = useRef<Daemon | null | undefined>(undefined)
  const previousSocketStatus = useRef<"connecting" | "open" | "closed">("closed")

  useEffect(() => {
    labelRef.current = label
  }, [label])

  const publish = useCallback((notifications: TaskNotification[], fresh: FreshQuestion[]) => {
    const attention = new Set<string>()
    for (const notification of notifications) {
      const agentName = requestingAgent(notification)
      if (agentName) attention.add(customerQuestionAttentionKey(hostId, agentName))
    }
    onSnapshot(hostId, labelRef.current, attention, fresh)
  }, [hostId, onSnapshot])

  const readAuthoritativeSnapshot = useCallback(async (
    resolved: Daemon | null,
    baseline: boolean,
    generation: number,
  ) => {
    const [inbox, taskPage] = await Promise.all([
      listTaskNotifications(false, resolved),
      listTasks({ limit: 1, status_view: "all" }, resolved),
    ])
    if (generationRef.current !== generation) return false
    if (baseline) refreshTargetRef.current = resolved
    const notifications = inbox.notifications ?? []
    const fresh: FreshQuestion[] = []
    for (const notification of notifications) {
      const isNew = !observedIds.current.has(notification.id)
      observedIds.current.add(notification.id)
      const agentName = requestingAgent(notification)
      if (!baseline && isNew && agentName) fresh.push({ notification, agentName })
    }
    publish(notifications, fresh)
    const nextSequence = taskPage.sequence ?? 0
    if (baseline) setSequence(nextSequence)
    else setSequence((current) => Math.max(current, nextSequence))
    return true
  }, [publish])

  const scheduleBaselineRetry = useCallback((generation: number) => {
    if (generationRef.current !== generation) return
    if (retryTimer.current) clearTimeout(retryTimer.current)
    retryTimer.current = setTimeout(() => {
      setRetryRequest((current) => ({
        generation,
        revision: (current?.revision ?? 0) + 1,
      }))
    }, CUSTOMER_QUESTION_RETRY_MS)
  }, [])

  const refresh = useCallback(async () => {
    const refreshTarget = refreshTargetRef.current
    if (refreshTarget === undefined) return
    if (refreshState.current.running) {
      refreshState.current.pending = true
      return
    }
    refreshState.current.running = true
    const generation = generationRef.current
    try {
      do {
        refreshState.current.pending = false
        if (!await readAuthoritativeSnapshot(refreshTarget, false, generation)) break
      } while (generationRef.current === generation && refreshState.current.pending)
    } catch {
      // Preserve the last successful snapshot, but recover through the same
      // silent baseline used at mount so an outage cannot replay alerts.
      if (generationRef.current === generation) {
        refreshTargetRef.current = undefined
        setHealth("recovering")
        setSocketEnabled(false)
        setTarget(undefined)
        scheduleBaselineRetry(generation)
      }
    } finally {
      refreshState.current.running = false
    }
  }, [readAuthoritativeSnapshot, scheduleBaselineRetry])

  useEffect(() => {
    registerRefresher(hostId, refresh)
    return () => registerRefresher(hostId, null)
  }, [hostId, refresh, registerRefresher])

  const beginBaseline = useCallback(async (generation: number) => {
    try {
      const resolved = hostId ? await resolveDaemon(hostId) : null
      if (hostId && !resolved) throw new Error(`host ${hostId} is unavailable`)
      if (generationRef.current !== generation) return
      setTarget(resolved)
      const applied = await readAuthoritativeSnapshot(resolved, true, generation)
      if (generationRef.current === generation && applied) {
        setHealth("healthy")
        setSocketEnabled(true)
      }
    } catch {
      if (generationRef.current === generation) {
        refreshTargetRef.current = undefined
        setHealth("recovering")
        scheduleBaselineRetry(generation)
      }
    }
  }, [hostId, readAuthoritativeSnapshot, scheduleBaselineRetry])

  useEffect(() => {
    const generation = generationRef.current + 1
    generationRef.current = generation
    void beginBaseline(generation)

    return () => {
      generationRef.current += 1
      refreshTargetRef.current = undefined
      if (retryTimer.current) clearTimeout(retryTimer.current)
      retryTimer.current = null
      onRemove(hostId)
    }
  }, [beginBaseline, hostId, onRemove])

  useEffect(() => {
    if (retryRequest?.generation !== generationRef.current) return
    void beginBaseline(retryRequest.generation)
  }, [beginBaseline, retryRequest])

  const socketStatus = useTasksSocket({
    target,
    after: sequence,
    enabled: socketEnabled,
    onHint: () => void refresh(),
    onReset: (nextSequence) => {
      setSequence((current) => Math.max(current, nextSequence))
      void refresh()
    },
  })

  useEffect(() => {
    const previous = previousSocketStatus.current
    previousSocketStatus.current = socketStatus
    if (previous === "open" && socketStatus === "closed" && health === "healthy" && socketEnabled) {
      void refresh()
    }
  }, [health, refresh, socketEnabled, socketStatus])

  return null
}
