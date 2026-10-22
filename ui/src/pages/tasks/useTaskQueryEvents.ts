import { useCallback, useEffect, useState } from "react"
import { useQueryClient, type QueryClient } from "@tanstack/react-query"
import { useTasksSocket } from "@/hooks/useTasksSocket"
import { targetReady, type ApiTarget } from "@/lib/api"
import { taskKeys } from "./taskQueries"

type Target = Exclude<ApiTarget, undefined>
type Cycle = { running: boolean; pending: boolean; scheduled: boolean; subscribers: number; sequence: number; target: Target }
const cycles = new WeakMap<QueryClient, Map<string, Cycle>>()

function serverCycle(client: QueryClient, server: string, target: Target) {
  let servers = cycles.get(client)
  if (!servers) { servers = new Map(); cycles.set(client, servers) }
  let state = servers.get(server)
  if (!state) {
    state = { running: false, pending: false, scheduled: false, subscribers: 0, sequence: 0, target }
    servers.set(server, state)
  }
  return state
}

/** All subscribers on a server share one cycle and deduplicate event sequences. */
export function useTaskQueryEvents(target: Target, initialSequence: number | undefined, taskKey?: string) {
  const client = useQueryClient()
  const server = target?.id ?? "local"
  const [sequence, setSequence] = useState(0)
  useEffect(() => {
    const state = serverCycle(client, server, null)
    if (!state.subscribers) state.sequence = 0
    state.subscribers++
    return () => { state.subscribers-- }
  }, [client, server])
  useEffect(() => { serverCycle(client, server, target).target = target }, [client, server, target])
  const refresh = useCallback(() => {
    const state = serverCycle(client, server, target)
    if (state.running) { state.pending = true; return }
    if (state.scheduled) return
    state.scheduled = true
    queueMicrotask(async () => {
      state.scheduled = false
      if (!state.subscribers) return
      state.running = true
      const queryKey = taskKeys.server(state.target)
      try {
        do {
          state.pending = false
          // A hint during a pre-existing read needs a read after that snapshot.
          await Promise.allSettled(client.getQueryCache().findAll({ queryKey })
            .filter((query) => query.state.fetchStatus === "fetching").map((query) => query.promise))
          if (state.subscribers && targetReady(state.target)) await client.invalidateQueries({ queryKey }, { cancelRefetch: false })
        } while (state.pending && state.subscribers)
      } finally { state.running = false }
    })
  }, [client, server, target])
  useTasksSocket({
    target,
    after: Math.max(sequence, initialSequence ?? 0),
    enabled: initialSequence !== undefined && targetReady(target),
    onHint: (hint) => {
      const state = serverCycle(client, server, target)
      setSequence(hint.sequence)
      if ((taskKey && hint.task_key && hint.task_key !== taskKey) || hint.sequence <= state.sequence) return
      state.sequence = hint.sequence
      refresh()
    },
    onReset: (value) => {
      const state = serverCycle(client, server, target)
      setSequence(Math.max(0, value))
      state.sequence = Math.max(0, value)
      refresh()
    },
  })
}
