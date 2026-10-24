import { useCallback, useEffect, useState } from "react"
import { useQueryClient, type QueryClient } from "@tanstack/react-query"
import { useApi } from "@/components/apiContext"
import { useTasksSocket } from "@/hooks/useTasksSocket"
import type { ApiClient } from "@/lib/api"
import { taskKeys } from "./taskQueries"

// `api` is the newest client for the server: a reconnect replaces it while the cycle stays.
type Cycle = { running: boolean; pending: boolean; scheduled: boolean; subscribers: number; sequence: number; api: ApiClient | null }
const cycles = new WeakMap<QueryClient, Map<string, Cycle>>()

function serverCycle(client: QueryClient, server: string, api: ApiClient | null) {
  let servers = cycles.get(client)
  if (!servers) { servers = new Map(); cycles.set(client, servers) }
  let state = servers.get(server)
  if (!state) {
    state = { running: false, pending: false, scheduled: false, subscribers: 0, sequence: 0, api }
    servers.set(server, state)
  }
  return state
}

/** All subscribers on a server share one cycle and deduplicate event sequences. */
export function useTaskQueryEvents(initialSequence: number | undefined, taskKey?: string) {
  const api = useApi()
  const target = api.target
  const client = useQueryClient()
  const server = target?.id ?? "local"
  const [sequence, setSequence] = useState(0)
  useEffect(() => {
    const state = serverCycle(client, server, null)
    if (!state.subscribers) state.sequence = 0
    state.subscribers++
    return () => { state.subscribers-- }
  }, [client, server])
  useEffect(() => { serverCycle(client, server, api).api = api }, [client, server, api])
  const refresh = useCallback(() => {
    const state = serverCycle(client, server, api)
    if (state.running) { state.pending = true; return }
    if (state.scheduled) return
    state.scheduled = true
    queueMicrotask(async () => {
      state.scheduled = false
      if (!state.subscribers) return
      state.running = true
      const queryKey = taskKeys.server((state.api ?? api).target)
      try {
        do {
          state.pending = false
          // A hint during a pre-existing read needs a read after that snapshot.
          await Promise.allSettled(client.getQueryCache().findAll({ queryKey })
            .filter((query) => query.state.fetchStatus === "fetching").map((query) => query.promise))
          if (state.subscribers && (state.api ?? api).ready) await client.invalidateQueries({ queryKey }, { cancelRefetch: false })
        } while (state.pending && state.subscribers)
      } finally { state.running = false }
    })
  }, [client, server, api])
  useTasksSocket({
    target,
    after: Math.max(sequence, initialSequence ?? 0),
    enabled: initialSequence !== undefined && api.ready,
    onHint: (hint) => {
      const state = serverCycle(client, server, api)
      setSequence(hint.sequence)
      if ((taskKey && hint.task_key && hint.task_key !== taskKey) || hint.sequence <= state.sequence) return
      state.sequence = hint.sequence
      refresh()
    },
    onReset: (value) => {
      const state = serverCycle(client, server, api)
      setSequence(Math.max(0, value))
      state.sequence = Math.max(0, value)
      refresh()
    },
  })
}
