import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import type { ApiTarget } from "@/lib/api"
import { listTasks, type Task, type TaskFilters, type TaskStatusView } from "@/lib/tasks"
import { targetFor } from "@/lib/terminalsHost"

// listAllTasks follows the cursor to the end and reports the newest event
// sequence seen, which is where a live socket for the same list resumes.
export async function listAllTasks(
  filters: TaskFilters,
  target?: ApiTarget,
): Promise<{ tasks: Task[]; sequence: number }> {
  const tasks: Task[] = []
  let after = ""
  let sequence = 0
  const seenCursors = new Set<string>()
  do {
    const page = await listTasks({ ...filters, limit: 500, after }, target)
    tasks.push(...(page.tasks ?? []))
    sequence = Math.max(sequence, page.sequence ?? 0)
    after = page.next_cursor ?? ""
    if (after && seenCursors.has(after)) throw new Error("task pagination cursor repeated")
    if (after) seenCursors.add(after)
  } while (after)
  return { tasks, sequence }
}

/** A server as the sidebar knows it; `error` means the agent poll could not
 *  reach it, so there is nothing to ask for tasks either. */
export interface TaskServer {
  id: string
  label: string
  error?: string
}

/** A task and the server it lives on. Keys are unique only within a server —
 *  a transferred task keeps its key and leaves a cancelled copy behind — so a
 *  row is `serverId` + `key`. */
export type ServerTask = Task & { serverId: string; serverName: string }

type ServerState = { tasks?: Task[]; sequence?: number; error?: string }

/**
 * Every server's tasks at once: each reachable server is read in parallel with
 * its own token, and a failure stays that server's error instead of the
 * list's. `reload` re-reads one server — the Retry link and that server's live
 * socket both use it.
 */
export function useAllServersTasks(
  servers: readonly TaskServer[],
  filters: { text: string; statusView: TaskStatusView },
) {
  const [state, setState] = useState<Record<string, ServerState>>({})
  const requests = useRef(new Map<string, number>())
  // The sidebar poll hands over a fresh array every few seconds; only a change
  // of servers or of their reachability is a reason to read again.
  const serversKey = servers.map((server) => `${server.id}\u0001${server.label}\u0001${server.error ?? ""}`).join("\u0000")
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const stableServers = useMemo(() => servers, [serversKey])
  const { text, statusView } = filters

  // Servers with a read in flight; `true` means a live hint arrived meanwhile
  // and one more read follows it.
  const inflight = useRef(new Map<string, boolean>())

  const reload = useCallback(function read(id: string) {
    const request = (requests.current.get(id) ?? 0) + 1
    requests.current.set(id, request)
    inflight.current.set(id, false)
    const current = () => requests.current.get(id) === request
    listAllTasks({ text, status_view: statusView }, targetFor(id))
      .then(({ tasks, sequence }) => {
        if (current()) setState((all) => ({ ...all, [id]: { tasks, sequence } }))
      })
      .catch((error: unknown) => {
        if (!current()) return
        // The rows it had may belong to another filter; the error says why
        // they are gone.
        const message = error instanceof Error ? error.message : String(error)
        setState((all) => ({ ...all, [id]: { error: message || "unavailable" } }))
      })
      .finally(() => {
        if (!current()) return
        const again = inflight.current.get(id)
        inflight.current.delete(id)
        if (again) read(id)
      })
  }, [statusView, text])

  // A live hint only says something changed; a busy server sends many, and
  // each would otherwise re-read every page.
  const refresh = useCallback((id: string) => {
    if (inflight.current.has(id)) inflight.current.set(id, true)
    else reload(id)
  }, [reload])

  useEffect(() => {
    for (const server of stableServers) {
      if (!server.error) reload(server.id)
    }
  }, [reload, stableServers])

  return useMemo(() => {
    const tasks: ServerTask[] = []
    const errors: Record<string, string> = {}
    const sequences: Record<string, number> = {}
    let answered = false
    for (const server of stableServers) {
      const entry = state[server.id]
      // A server the sidebar cannot reach is not asked, but a Retry that got an
      // answer wins over that older verdict.
      const error = entry?.error ?? (entry?.tasks ? undefined : server.error)
      if (error) errors[server.id] = error
      if (entry) answered = true
      if (!entry?.tasks) continue
      if (entry.sequence !== undefined) sequences[server.id] = entry.sequence
      for (const task of entry.tasks) tasks.push({ ...task, serverId: server.id, serverName: server.label })
    }
    // Before the first sidebar poll there is no server to wait for yet.
    const loading = !answered && (stableServers.length === 0 || stableServers.some((server) => !server.error))
    return { tasks, errors, sequences, loading, reload, refresh }
  }, [refresh, reload, state, stableServers])
}
