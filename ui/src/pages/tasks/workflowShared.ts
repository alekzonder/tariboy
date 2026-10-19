import { useEffect, useRef } from "react"

/** Helpers shared by the workflow panel and the queue workflow settings. */

/** The message of a failed request, as a person reads it. */
export function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

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
