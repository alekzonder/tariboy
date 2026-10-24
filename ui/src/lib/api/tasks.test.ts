import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import type { Daemon } from "../daemons"
import { ApiError, createApi, setActiveDaemon, transferTask } from "./index"

type Reply = { status?: number; result?: unknown; error?: { code: string; message: string } }
type Sent = { url: string; method: string; body: unknown; auth?: string; signal?: AbortSignal }

const remote: Daemon = { id: "remote", label: "Remote", baseURL: "https://remote.test", token: "secret" }
const other: Daemon = { id: "other", label: "Other", baseURL: "https://other.test", token: "other-secret" }

let sent: Sent[] = []
let replies: Reply[] = []
let fallback: Reply = { result: {} }

/** Answers each request with the next queued reply, or `fallback`. */
const reply = (...next: Reply[]) => { replies.push(...next) }
const answer = (result: unknown) => { fallback = { result } }

beforeEach(() => {
  sent = []
  replies = []
  fallback = { result: {} }
  setActiveDaemon(null)
  vi.stubGlobal("fetch", vi.fn(async (url: string, init: RequestInit) => {
    const headers = init.headers as Record<string, string> | undefined
    sent.push({
      url,
      method: init.method ?? "GET",
      body: typeof init.body === "string" ? JSON.parse(init.body) : init.body,
      auth: headers?.Authorization,
      signal: init.signal ?? undefined,
    })
    const { status = 200, result, error } = replies.shift() ?? fallback
    const envelope = error ? { ok: false, error } : { ok: true, result }
    return { ok: status < 300, status, text: async () => JSON.stringify(envelope), json: async () => envelope } as Response
  }))
})
afterEach(() => {
  vi.unstubAllGlobals()
  setActiveDaemon(null)
})

const last = () => sent.at(-1)!
/** The last request went to `host` with its token, as `method path [body]`. */
function expectLast(method: string, path: string, body?: unknown, host: Daemon = remote) {
  expect(last()).toMatchObject({ url: `${host.baseURL}${path}`, method, body, auth: `Bearer ${host.token}` })
}

describe("task client", () => {
  it("forwards the query cancellation signal to the HTTP transport", async () => {
    const signal = new AbortController().signal
    await createApi(null).tasks.getTask("DEV-1", signal)
    expect(last()).toMatchObject({ url: "/api/tasks/DEV-1", method: "GET", body: undefined, auth: undefined, signal })
  })

  it("uses revisioned explicit pool mutations", async () => {
    await createApi(undefined).tasks.rebindAgentPool("DEV", "developers", ["dev-a", "dev-b"], 4, "pool-4")
    expect(last()).toMatchObject({
      url: "/api/task-queues/DEV/pools/developers",
      method: "PATCH",
      body: { agents: ["dev-a", "dev-b"], revision: 4, idempotency_key: "pool-4" },
    })
  })

  it("lists the agent pools of a queue", async () => {
    await createApi(null).tasks.listAgentPools("DEV")
    expect(sent.map(({ method, url }) => [method, url])).toEqual([["GET", "/api/task-queues/DEV/pools"]])
  })

  it("keeps wait-customer, pull request, revision, and the client's host in task updates", async () => {
    await createApi(remote).tasks.updateTask("TARI-43", {
      status: "wait_customer",
      pull_request: "https://example.test/pull/7",
      revision: 7,
    })
    expectLast("PATCH", "/api/tasks/TARI-43", {
      status: "wait_customer",
      pull_request: "https://example.test/pull/7",
      revision: 7,
    })
  })
})

describe("task transfer", () => {
  it("exports on the source, imports on the destination, then comments on and cancels the source copy", async () => {
    const bundle = { root_key: "DEV-1", queue: "DEV", tasks: [{ key: "DEV-1" }], relations: [] }
    reply(
      { result: bundle },
      { result: { key: "DEV-1" } },
      { result: {} },
      { result: { task: { key: "DEV-1", revision: 5 }, comments: [], waiting_for: [], relations: [] } },
      { result: {} },
    )
    const source = createApi(remote)
    const destination = createApi(other)
    await transferTask("DEV-1", source.tasks, destination.tasks, "Other", "move-1")
    expect(sent.map(({ method, url, auth }) => [method, url, auth])).toEqual([
      ["GET", "https://remote.test/api/tasks/DEV-1/export", "Bearer secret"],
      ["POST", "https://other.test/api/tasks/import", "Bearer other-secret"],
      ["POST", "https://remote.test/api/tasks/DEV-1/comments", "Bearer secret"],
      ["GET", "https://remote.test/api/tasks/DEV-1", "Bearer secret"],
      ["PATCH", "https://remote.test/api/tasks/DEV-1", "Bearer secret"],
    ])
    expect(sent[1].body).toEqual({ bundle })
    expect(sent[2].body).toEqual({ body: "Moved to Other as DEV-1.", idempotency_key: "move-1" })
    expect(sent[4].body).toEqual({ status: "cancelled", revision: 5 })
  })

  it("stops before touching the source copy when the import fails", async () => {
    reply(
      { result: { root_key: "DEV-1", queue: "DEV", tasks: [], relations: [] } },
      { status: 409, error: { code: "task_exists", message: "exists" } },
    )
    await expect(transferTask("DEV-1", createApi(remote).tasks, createApi(other).tasks, "Other")).rejects.toThrow("exists")
    expect(sent.map(({ method, url }) => [method, url])).toEqual([
      ["GET", "https://remote.test/api/tasks/DEV-1/export"],
      ["POST", "https://other.test/api/tasks/import"],
    ])
  })
})

describe("workflow task client", () => {
  const workflows = () => createApi(remote).workflows

  it("reads the workflow of a task", async () => {
    const view = { name: "flow", status: "review", statuses: [{ id: "review", owner: "customer", terminal: false }], declared_artifacts: [] }
    answer(view)
    await expect(workflows().getTaskWorkflow("TARI 1")).resolves.toEqual(view)
    expectLast("GET", "/api/tasks/TARI%201/workflow")
  })

  it("advances with and without the status the caller saw", async () => {
    const pending = { id: 4, outcome: "approve", state: "pending", wait_seconds: 30 }
    answer(pending)
    await expect(workflows().advanceTask("TARI-1", "approve", "ok")).resolves.toEqual(pending)
    expectLast("POST", "/api/tasks/TARI-1/advance", { outcome: "approve", message: "ok" })
    const request = await workflows().advanceTask("TARI-1", "approve", "ok", "review")
    expect(request.wait_seconds).toBe(30)
    expectLast("POST", "/api/tasks/TARI-1/advance", { outcome: "approve", message: "ok", from: "review" })
  })

  it("reads a transition request", async () => {
    await workflows().getTransitionRequest("TARI-1", 7)
    expectLast("GET", "/api/tasks/TARI-1/workflow/requests/7")
  })

  it("sets an artifact and reads its history", async () => {
    await workflows().setTaskArtifact("TARI-1", "pr url", "https://x")
    expectLast("PUT", "/api/tasks/TARI-1/artifacts/pr%20url", { value: "https://x" })
    answer({ artifact: { id: 2 }, history: [{ id: 1 }, { id: 2 }] })
    expect(await workflows().getTaskArtifactHistory("TARI-1", "pr url")).toEqual([{ id: 1 }, { id: 2 }])
    expectLast("GET", "/api/tasks/TARI-1/artifacts/pr%20url")
  })

  it("lists script runs and unwraps them", async () => {
    answer({ runs: [{ id: 3 }], count: 1 })
    expect(await workflows().listTaskScriptRuns("TARI-1")).toEqual([{ id: 3 }])
    expectLast("GET", "/api/tasks/TARI-1/workflow/runs")
  })

  it("returns the log text and truncation, with an optional byte limit", async () => {
    answer({ run_id: 3, text: "line", truncated: true })
    expect(await workflows().getTaskScriptRunLog("TARI-1", 3)).toEqual({ text: "line", truncated: true })
    expectLast("GET", "/api/tasks/TARI-1/workflow/runs/3/log")
    await workflows().getTaskScriptRunLog("TARI-1", 3, 4096)
    expectLast("GET", "/api/tasks/TARI-1/workflow/runs/3/log?max_bytes=4096")
  })

  it("moves, cancels, and resumes", async () => {
    await workflows().moveTaskWorkflow("TARI-1", "review", "stuck")
    expectLast("POST", "/api/tasks/TARI-1/workflow/move", { to: "review", reason: "stuck" })
    await workflows().cancelWorkflowTask("TARI-1")
    expectLast("POST", "/api/tasks/TARI-1/cancel")
    await workflows().resumeTaskWorkflow("TARI-1", "release")
    expectLast("POST", "/api/tasks/TARI-1/workflow/resume", { decision: "release" })
    await workflows().reassignTaskWorkflow("TARI-1", "dev-2")
    expectLast("POST", "/api/tasks/TARI-1/workflow/reassign", { agent: "dev-2" })
  })

  it("maps only an unbound queue to null", async () => {
    reply({ status: 404, error: { code: "queue_workflow_not_found", message: "none" } })
    expect(await workflows().getQueueWorkflow("DEV")).toBeNull()
    expectLast("GET", "/api/task-queues/DEV/workflow")
    reply({ status: 500, error: { code: "internal", message: "boom" } })
    const failure = await workflows().getQueueWorkflow("DEV").catch((error: unknown) => error)
    expect(failure).toBeInstanceOf(ApiError)
    expect(failure).toMatchObject({ status: 500, code: "internal", message: "boom" })
    reply({ result: { queue: "DEV", revision: 2 } })
    expect(await workflows().getQueueWorkflow("DEV")).toEqual({ queue: "DEV", revision: 2 })
  })

  it("binds and clears a queue workflow with the revision", async () => {
    await workflows().setQueueWorkflow("DEV", "development:latest", 0)
    expectLast("PUT", "/api/task-queues/DEV/workflow", { ref: "development:latest", revision: 0 })
    await workflows().clearQueueWorkflow("DEV", 3)
    expectLast("DELETE", "/api/task-queues/DEV/workflow?revision=3")
  })

  it("lists, sets, and removes queue secrets without reading values", async () => {
    answer({ secrets: [{ key: "TOKEN", updated_at: "t" }], count: 1, global_env: ["GH_TOKEN"] })
    expect(await workflows().listQueueSecrets("DEV")).toEqual({ secrets: [{ key: "TOKEN", updated_at: "t" }], global_env: ["GH_TOKEN"] })
    expectLast("GET", "/api/task-queues/DEV/secrets")
    answer({ queue: "DEV", key: "A B", updated_at: "t" })
    expect(await workflows().setQueueSecret("DEV", "A B", "s3cret")).toEqual({ queue: "DEV", key: "A B", updated_at: "t" })
    expectLast("PUT", "/api/task-queues/DEV/secrets/A%20B", { value: "s3cret" })
    await workflows().removeQueueSecret("DEV", "A B")
    expectLast("DELETE", "/api/task-queues/DEV/secrets/A%20B")
  })

  it("lists, sets, and removes plain queue env vars with their values", async () => {
    answer({ env: [{ key: "LOG_LEVEL", value: "debug", updated_at: "t" }], count: 1 })
    expect(await workflows().listQueueEnv("DEV")).toEqual([{ key: "LOG_LEVEL", value: "debug", updated_at: "t" }])
    expectLast("GET", "/api/task-queues/DEV/env")
    answer({ queue: "DEV", key: "A B", value: "v", updated_at: "t" })
    await workflows().setQueueEnv("DEV", "A B", "v")
    expectLast("PUT", "/api/task-queues/DEV/env/A%20B", { value: "v" })
    await workflows().removeQueueEnv("DEV", "A B")
    expectLast("DELETE", "/api/task-queues/DEV/env/A%20B")
  })

  it("lists queue sources and reads a source run's log", async () => {
    answer({ sources: [{ name: "prs", runs: [{ id: 4 }] }], count: 1 })
    expect(await workflows().listQueueSources("DEV")).toEqual([{ name: "prs", runs: [{ id: 4 }] }])
    expectLast("GET", "/api/task-queues/DEV/sources")
    answer({ run_id: 4, text: "line", truncated: false })
    expect(await workflows().getQueueSourceRunLog("DEV", 4)).toEqual({ text: "line", truncated: false })
    expectLast("GET", "/api/task-queues/DEV/source-runs/4/log")
    await workflows().getQueueSourceRunLog("DEV", 4, 4096)
    expectLast("GET", "/api/task-queues/DEV/source-runs/4/log?max_bytes=4096")
  })

  it("lists workflow images", async () => {
    answer({ workflows: [{ name: "development", tag: "latest" }], count: 1 })
    expect(await workflows().listWorkflowImages()).toEqual([{ name: "development", tag: "latest" }])
    expectLast("GET", "/api/workflow-images")
  })

  it("reads, removes, validates, and builds workflow images on the client's host", async () => {
    await workflows().getWorkflowImage("dev flow", "1.0.0")
    expectLast("GET", "/api/workflow-images/dev%20flow/1.0.0")
    await workflows().removeWorkflowImage("dev", "latest")
    expectLast("DELETE", "/api/workflow-images/dev/latest")
    await workflows().validateWorkflowDirectory("/src/wf")
    expectLast("POST", "/api/workflow-images/validate", { path: "/src/wf" })
    await workflows().buildWorkflowDirectory("/src/wf")
    expectLast("POST", "/api/workflow-images/build", { path: "/src/wf" })
  })
})
