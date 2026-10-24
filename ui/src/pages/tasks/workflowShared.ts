import { useEffect, useRef, useState } from "react"
import { useApi } from "@/components/apiContext"
import { ApiError, type TransitionRequest } from "@/lib/api"
import type { StatusTone } from "@/lib/statusTone"

/** Helpers shared by the workflow panel and the queue workflow settings. */

/** The message of a failed request, as a person reads it. */
export { errorText } from "@/lib/queryClient"
import { errorText } from "@/lib/queryClient"

// A queue env or secret key: the daemon's rule (internal/tasks/workflow_secrets.go), shared by env vars.
const KEY_PATTERN = /^[A-Za-z_][A-Za-z0-9_]*$/

export function keyProblem(key: string): string {
  if (!KEY_PATTERN.test(key)) return "A key starts with a letter or underscore and holds letters, digits, and underscores."
  if (key.startsWith("TARIBOY_")) return "A key must not start with TARIBOY_."
  return ""
}

/** A ref that is true while the component is mounted, for async callbacks. */
export function useMounted() {
  const mounted = useRef(true)
  useEffect(() => {
    mounted.current = true
    return () => { mounted.current = false }
  }, [])
  return mounted
}

/**
 * The tone of a script run: what went wrong is red, what passed or found
 * something is green, a quiet run is dim, and a run still going draws the eye.
 */
export function runTone(state: string, verdict?: string): StatusTone {
  if (state === "pending" || state === "running") return "attention"
  if (state === "interrupted" || state === "cancelled") return "quiet"
  switch (verdict) {
    case "failure": case "reject": return "danger"
    case "pass": case "outcome": case "items": return "live"
    case "quiet": return "faint"
    default: return "quiet"
  }
}

/** How long to wait for a pending request when the daemon does not say. */
const DEFAULT_WAIT_SECONDS = 90
const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))
/** A refusal that means the view is stale, so the caller refetches it. */
const STALE_CODES = new Set(["status_changed", "transition_pending", "workflow_paused", "artifact_missing"])

/** How following a transition request ended. */
export type FollowResult =
  | { kind: "applied" }
  /** The request settled without applying: rejected, failed, or cancelled. */
  | { kind: "result"; request: TransitionRequest }
  /** The wait ran out; the request is still pending on the daemon. */
  | { kind: "timeout"; requestId: number }
  /** The send or a poll was refused or failed. `stale` says the view should be refetched; `status` is the one a status_changed names. */
  | { kind: "error"; label: string; message: string; stale: boolean; status?: string }

/**
 * Sends a transition request (an advance or a retry) and follows it, polling
 * once a second for as long as its checks may take, then hands the result to
 * `handle`. Nothing is handed over once the component is unmounted. Outcomes,
 * the problem block, and Return to agent all wait for a request the same way.
 */
export function useFollowRequest(taskKey: string) {
  const api = useApi()
  const [checking, setChecking] = useState(false)
  const mountedRef = useMounted()
  const follow = async (send: () => Promise<TransitionRequest>, handle: (result: FollowResult) => void): Promise<void> => {
    setChecking(true)
    const settle = (result: FollowResult) => {
      if (!mountedRef.current) return
      setChecking(false)
      handle(result)
    }
    try {
      let request = await send()
      let polls = Math.ceil(request.wait_seconds ?? DEFAULT_WAIT_SECONDS)
      while (request.state === "pending") {
        if (polls-- <= 0) { settle({ kind: "timeout", requestId: request.id }); return }
        await sleep(1000)
        if (!mountedRef.current) return
        try {
          request = await api.workflows.getTransitionRequest(taskKey, request.id)
        } catch (error) {
          // The request may still be applying; only reading it failed.
          settle({ kind: "error", label: "Could not read the request", message: errorText(error), stale: false })
          return
        }
      }
      settle(request.state === "applied" ? { kind: "applied" } : { kind: "result", request })
    } catch (error) {
      const stale = error instanceof ApiError && STALE_CODES.has(error.code)
      const status = stale && error instanceof ApiError && typeof error.details?.status === "string" ? error.details.status : undefined
      settle({ kind: "error", label: "Refused", message: errorText(error), stale, status })
    }
  }
  return { checking, follow }
}
