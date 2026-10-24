import { QueryClientProvider, keepPreviousData } from "@tanstack/react-query"
import { act, render, screen, waitFor } from "@/test/queryRender"
import { expect, it, vi } from "vitest"
import { ApiProvider } from "@/components/ApiProvider"
import { useApi } from "@/components/apiContext"
import { ApiError, createApi, setActiveDaemon, type ApiTarget } from "@/lib/api"
import { addDaemon, resolveDaemon, setDaemonToken } from "@/lib/daemons"
import { queryClient, apiServerKey } from "@/lib/queryClient"
import { useApiQuery } from "./useApiQuery"

const host = { id: "remote", label: "Remote", baseURL: "https://first.test", token: "secret-one" }

type ResourceProps = { filter?: string; read: (target: ApiTarget, signal: AbortSignal) => Promise<string> }

function Reader({ read, filter }: ResourceProps) {
  const api = useApi()
  const result = useApiQuery({ queryKey: [...apiServerKey(api.target), "example", ...(filter ? [filter] : [])], queryFn: ({ signal }) => read(api.target, signal), placeholderData: keepPreviousData })
  return <div data-placeholder={result.isPlaceholderData}>{result.data ?? result.error?.message ?? "Loading"}</div>
}

// Without a target the resource reads through the surrounding provider's client.
function Resource({ target, ...props }: ResourceProps & { target?: ApiTarget }) {
  if (target === undefined) return <Reader {...props} />
  return <ApiProvider target={target}><Reader {...props} /></ApiProvider>
}

it("keeps server identity across a new port and cancels the old endpoint's read", async () => {
  const requests: { target: ApiTarget; signal: AbortSignal; resolve: (value: string) => void }[] = []
  const read = (target: ApiTarget, signal: AbortSignal) => new Promise<string>((resolve) => requests.push({ target, signal, resolve }))
  const mounted = render(<Resource target={host} read={read} />)
  await act(async () => requests[0].resolve("Cached"))
  expect(await screen.findByText("Cached")).toBeInTheDocument()
  await act(async () => { void mounted.client.invalidateQueries() })
  expect(requests).toHaveLength(2)
  mounted.rerender(<Resource target={{ ...host, baseURL: "" }} read={read} />)
  expect(requests[1].signal.aborted).toBe(true)
  expect(screen.getByText("Cached")).toBeInTheDocument()
  expect(requests).toHaveLength(2)
  const reconnected = { ...host, baseURL: "https://second.test" }
  mounted.rerender(<Resource target={reconnected} read={read} />)
  await waitFor(() => expect(requests).toHaveLength(3))
  expect(requests[2].target).toEqual(reconnected)
  await act(async () => requests[1].resolve("Obsolete"))
  expect(screen.queryByText("Obsolete")).not.toBeInTheDocument()
  await act(async () => requests[2].resolve("Recovered"))
  expect(await screen.findByText("Recovered")).toBeInTheDocument()
})

it("separates explicit local and active remote requests without putting credentials in keys", async () => {
  setActiveDaemon(host)
  const read = vi.fn(async (target: ApiTarget) => target ? "Remote" : "Local")
  try {
    const mounted = render(<><Resource read={read} /><Resource target={null} read={read} /></>, { api: createApi(undefined) })
    expect(await screen.findByText("Remote")).toBeInTheDocument()
    expect(await screen.findByText("Local")).toBeInTheDocument()
    expect(read.mock.calls.map(([target]) => target)).toEqual([host, null])
    const keys = mounted.client.getQueryCache().getAll().map((query) => query.queryKey)
    expect(JSON.stringify(keys)).not.toContain("secret-one")
    expect(keys).toEqual([["server", "remote", "example"], ["server", "local", "example"]])
  } finally { setActiveDaemon(null) }
})

it("clears cached data on credential rotation before a new read completes", async () => {
  const read = vi.fn().mockResolvedValueOnce("Private").mockImplementation(() => new Promise(() => {}))
  const mounted = render(<Resource target={host} read={read} />)
  await screen.findByText("Private")
  mounted.rerender(<Resource target={{ ...host, token: "secret-two" }} read={read} />)
  expect(screen.queryByText("Private")).not.toBeInTheDocument()
  await waitFor(() => expect(read).toHaveBeenCalledTimes(2))
  expect(mounted.client.getQueryData(["server", "remote", "example"])).toBeUndefined()
  mounted.rerender(<Resource target={{ ...host, token: "secret-two" }} read={read} />)
  expect(screen.queryByText("Private")).not.toBeInTheDocument()
})

it("does not retry a failed read and keeps prior data only for network failures", async () => {
  const read = vi.fn().mockResolvedValueOnce("Snapshot").mockRejectedValue(new ApiError(0, "network_error", "Offline"))
  const mounted = render(<Resource target={host} read={read} />)
  await screen.findByText("Snapshot")
  await act(async () => { await mounted.client.invalidateQueries() })
  expect(screen.getByText("Snapshot")).toBeInTheDocument()
  expect(read).toHaveBeenCalledTimes(2)
  read.mockRejectedValue(new ApiError(401, "unauthorized", "Sign in"))
  await act(async () => { await mounted.client.invalidateQueries() })
  expect(await screen.findByText("Sign in")).toBeInTheDocument()
  expect(mounted.client.getQueryData(["server", "remote", "example"])).toBeUndefined()
  expect(read).toHaveBeenCalledTimes(3)
})

it("does not restore an observer snapshot after registry credential replacement", async () => {
  const meta = await addDaemon({ label: "Account", baseURL: "https://account.test", token: "old" })
  const original = await resolveDaemon(meta.id)
  const read = vi.fn().mockResolvedValueOnce("Old account").mockImplementation(() => new Promise(() => {}))
  const view = (target: ApiTarget) => <QueryClientProvider client={queryClient}><Resource target={target} read={read} /></QueryClientProvider>
  const mounted = render(view(original))
  try {
    await screen.findByText("Old account")
    await act(async () => { await setDaemonToken(meta.id, "new") })
    const changed = await resolveDaemon(meta.id)
    mounted.rerender(view(changed))
    await waitFor(() => expect(read.mock.calls.length).toBeGreaterThan(1))
    mounted.rerender(view(changed))
    expect(screen.queryByText("Old account")).not.toBeInTheDocument()
    expect(queryClient.getQueryData([...apiServerKey(changed!), "example"])).toBeUndefined()
  } finally { mounted.unmount(); queryClient.clear() }
})

it("restarts the new account read after credentials change while unavailable", async () => {
  const reads: { signal: AbortSignal; resolve: (value: string) => void }[] = []
  const read = (_target: ApiTarget, signal: AbortSignal) => new Promise<string>((resolve) => reads.push({ signal, resolve }))
  const mounted = render(<Resource target={host} read={read} />)
  await act(async () => reads[0].resolve("Old"))
  await screen.findByText("Old")
  mounted.rerender(<Resource target={{ ...host, baseURL: "" }} read={read} />)
  mounted.rerender(<Resource target={{ ...host, token: "new" }} read={read} />)
  await waitFor(() => expect(reads.length).toBeGreaterThan(1))
  await waitFor(() => expect(reads.at(-1)!.signal.aborted).toBe(false))
  await act(async () => reads.at(-1)!.resolve("New"))
  expect(await screen.findByText("New")).toBeInTheDocument()
})

it.each(["registry", "direct"] as const)("clears an already active filter placeholder on %s credential rotation", async (path) => {
  const meta = await addDaemon({ label: "Placeholder account", baseURL: "https://placeholder.test", token: "old" })
  const original = (await resolveDaemon(meta.id))!
  const read = vi.fn().mockResolvedValueOnce("Private previous account").mockImplementation(() => new Promise(() => {}))
  const view = (target: ApiTarget, filter?: string) => <QueryClientProvider client={queryClient}><Resource target={target} read={read} filter={filter} /></QueryClientProvider>
  const mounted = render(view(original))
  try {
    await screen.findByText("Private previous account")
    mounted.rerender(view(original, "pending-filter"))
    await waitFor(() => expect(read).toHaveBeenCalledTimes(2))
    expect(screen.getByText("Private previous account")).toHaveAttribute("data-placeholder", "true")
    mounted.rerender(view({ ...original, baseURL: "" }, "pending-filter"))
    expect(screen.getByText("Private previous account")).toHaveAttribute("data-placeholder", "true")
    mounted.rerender(view(original, "pending-filter"))
    await waitFor(() => expect(read.mock.calls.length).toBeGreaterThan(2))
    expect(screen.getByText("Private previous account")).toHaveAttribute("data-placeholder", "true")
    if (path === "registry") await act(async () => { await setDaemonToken(meta.id, "new") })
    const changed = path === "registry" ? (await resolveDaemon(meta.id))! : { ...original, token: "new" }
    mounted.rerender(view(changed, "pending-filter"))
    await act(async () => {})
    for (let render = 0; render < 3; render++) {
      mounted.rerender(view(changed, "pending-filter"))
      await act(async () => {})
      expect(screen.queryByText("Private previous account")).not.toBeInTheDocument()
    }
    expect(queryClient.getQueryData([...apiServerKey(changed), "example", "pending-filter"])).toBeUndefined()
  } finally { mounted.unmount(); queryClient.clear() }
})
