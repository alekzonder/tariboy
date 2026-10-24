import type { PropsWithChildren } from "react"
import { ApiProvider } from "@/components/ApiProvider"
import { createApi, type ApiClient } from "@/lib/api"
import type { Daemon } from "@/lib/daemons"

type Domain = Exclude<{ [K in keyof ApiClient]: ApiClient[K] extends (...args: never[]) => unknown ? never : ApiClient[K] extends object ? K : never }[keyof ApiClient], undefined>
export type ApiOverrides = { [K in Domain]?: Partial<ApiClient[K]> } & Partial<Pick<ApiClient, "get" | "post" | "put" | "delete" | "request" | "raw" | "agentGet" | "agentPost" | "agentPut" | "agentDelete">>

/**
 * fakeApi builds a real client for `target` (local by default) and replaces the
 * given methods, so a test mocks exactly the calls it cares about; every other
 * method still goes through fetch.
 */
export function fakeApi(overrides: ApiOverrides = {}, target: Daemon | null = null): ApiClient {
  const client = createApi(target) as unknown as Record<string, unknown>
  const result: Record<string, unknown> = { ...client }
  for (const [key, value] of Object.entries(overrides)) {
    const base = client[key]
    result[key] = typeof value === "function" || typeof base === "function" ? value : { ...(base as object), ...(value as object) }
  }
  return result as unknown as ApiClient
}

/** A render wrapper that provides `client` (or a local client) to the tree. */
export function apiWrapper(client: ApiClient = createApi(null)) {
  return function ApiWrapper({ children }: PropsWithChildren) {
    return <ApiProvider client={client}>{children}</ApiProvider>
  }
}
