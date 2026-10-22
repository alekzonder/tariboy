import { useCallback, useEffect, useRef, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { useTasksSocket } from "@/hooks/useTasksSocket"
import { targetReady, type ApiTarget } from "@/lib/api"
import { taskKeys } from "./taskQueries"

/** Socket frames invalidate HTTP queries; bursts share one cycle and one trailing cycle. */
export function useTaskQueryEvents(target: Exclude<ApiTarget, undefined>, initialSequence: number | undefined, taskKey?: string) {
  const client = useQueryClient()
  const [sequence, setSequence] = useState(0)
  const cycle = useRef({ running: false, pending: false, scheduled: false, disposed: false })
  useEffect(() => {
    const state = cycle.current
    state.disposed = false
    return () => { state.disposed = true }
  }, [])
  const refresh = useCallback(() => {
    const state = cycle.current
    if (state.running) { state.pending = true; return }
    if (state.scheduled) return
    state.scheduled = true
    queueMicrotask(async () => {
      state.scheduled = false
      if (state.disposed) return
      state.running = true
      const queryKey = taskKeys.server(target)
      do {
        state.pending = false
        // A hint during a pre-existing read needs a read after that snapshot.
        await Promise.allSettled(client.getQueryCache().findAll({ queryKey })
          .filter((query) => query.state.fetchStatus === "fetching").map((query) => query.promise))
        if (!state.disposed && targetReady(target)) await client.invalidateQueries({ queryKey }, { cancelRefetch: false })
      } while (state.pending && !state.disposed)
      state.running = false
    })
  }, [client, target])
  useTasksSocket({
    target,
    after: Math.max(sequence, initialSequence ?? 0),
    enabled: initialSequence !== undefined && targetReady(target),
    onHint: (hint) => {
      setSequence(hint.sequence)
      if (!taskKey || !hint.task_key || hint.task_key === taskKey) refresh()
    },
    onReset: (value) => { setSequence(Math.max(0, value)); refresh() },
  })
}
