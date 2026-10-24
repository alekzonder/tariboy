import type { ReactElement } from "react"
import { act, fireEvent, render as renderWith, screen, within } from "@/test/queryRender"
import { afterEach, beforeEach, expect, it, vi } from "vitest"
import type { WorkflowScriptRun, TransitionRequest, WorkflowView } from "@/lib/api"
import WorkflowProblem from "./WorkflowProblem"
import { customerView, poolView, remoteTarget } from "@/test/fixtures/workflowFixtures"
import { fakeApi } from "@/test/api"

const api = vi.hoisted(() => ({ getTransitionRequest: vi.fn(), retryTransitionRequest: vi.fn(), getTaskScriptRunLog: vi.fn() }))
// The mocks are methods of a client bound to the remote host, so each call
// they record went to that host.
const remoteApi = fakeApi({ workflows: { getTransitionRequest: api.getTransitionRequest, retryTransitionRequest: api.retryTransitionRequest, getTaskScriptRunLog: api.getTaskScriptRunLog } }, remoteTarget)
const render = (ui: ReactElement) => renderWith(ui, { api: remoteApi })

const request = (state: TransitionRequest["state"], extra: Partial<TransitionRequest> = {}): TransitionRequest => ({
  id: 11, task_key: "REL-1", outcome: "approve", actor: "user:owner", state, created_at: "2026-10-01T10:06:00Z", ...extra,
})
const failedRun: WorkflowScriptRun = { id: 8, task_key: "REL-1", kind: "check", script: "./checks/notes.sh", run_as: "queue", state: "finished",
  verdict: "failure", exit_code: 3, message: "the script failed with exit 3", request_id: 11,
  created_at: "2026-10-01T10:06:00Z", started_at: "2026-10-01T10:06:01Z", finished_at: "2026-10-01T10:06:02Z", log_path: "/l" }

beforeEach(() => {
  vi.clearAllMocks()
  api.getTaskScriptRunLog.mockResolvedValue({ text: "Traceback: boom", truncated: false })
})
afterEach(() => { vi.useRealTimers() })

function renderProblem(view: WorkflowView) {
  const onChanged = vi.fn()
  const onRefresh = vi.fn()
  const element = (next: WorkflowView) => <WorkflowProblem taskKey="REL-1" view={next} onChanged={onChanged} onRefresh={onRefresh} />
  const { rerender, unmount } = render(element(view))
  return { onChanged, onRefresh, unmount, rerender: (next: WorkflowView) => rerender(element(next)) }
}

it("explains a failed request with its run's log open and a Retry", async () => {
  await act(async () => { renderProblem({ ...customerView, runs: [failedRun], last_request: request("failed", { result_message: "the script failed with exit 3\nlog: /x/run.log" }) }) })
  const problem = screen.getByRole("alert")
  expect(problem).toHaveAttribute("data-problem", "failed")
  expect(problem).toHaveTextContent("A check could not run: approve")
  expect(within(problem).getByText("the script failed with exit 3")).toBeInTheDocument()
  expect(problem).not.toHaveTextContent("log: /x/run.log")
  expect(problem).toHaveTextContent(/then Retry/)
  expect(api.getTaskScriptRunLog).toHaveBeenCalledWith("REL-1", 8, undefined)
  expect(problem.querySelector("pre")!.textContent).toContain("the script failed with exit 3")
  expect(within(problem).getByRole("listitem").querySelector("pre")!.textContent).toBe("Traceback: boom")
  expect(within(problem).getByRole("button", { name: "Retry" })).toBeEnabled()
})

it("retries the last failed request, even in a pool status, and follows it like an advance", async () => {
  vi.useFakeTimers()
  api.retryTransitionRequest.mockResolvedValue(request("pending", { id: 12, wait_seconds: 30 }))
  api.getTransitionRequest.mockResolvedValueOnce(request("applied", { id: 12 }))
  const { onChanged } = renderProblem({ ...poolView, last_request: request("failed", { result_message: "GitHub answered HTTP 401" }) })
  await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Retry" })) })
  expect(api.retryTransitionRequest).toHaveBeenCalledWith("REL-1", 11)
  expect(screen.getByRole("button", { name: "Checking…" })).toBeDisabled()
  await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
  expect(api.getTransitionRequest).toHaveBeenCalledWith("REL-1", 12)
  expect(onChanged).toHaveBeenCalledTimes(1)
})

it("shows a retry that settled without applying and refetches the view", async () => {
  api.retryTransitionRequest.mockResolvedValue(request("failed", { id: 12, result_message: "still 401" }))
  const { onChanged, onRefresh } = renderProblem({ ...poolView, last_request: request("failed", { result_message: "GitHub answered HTTP 401" }) })
  await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Retry" })) })
  expect(screen.getByRole("status")).toHaveTextContent("failed: still 401")
  expect(onRefresh).toHaveBeenCalled()
  expect(onChanged).not.toHaveBeenCalled()
})

it("explains a rejection by who fixes it, without a retry, and offers the run's log on demand", () => {
  renderProblem({ ...poolView, runs: [{ ...failedRun, verdict: "reject", exit_code: 112, message: "no heading" }],
    last_request: request("rejected", { result_message: "no heading" }) })
  const problem = screen.getByRole("alert")
  expect(problem).toHaveAttribute("data-problem", "rejected")
  expect(problem).toHaveTextContent("A check rejected the request: approve")
  expect(problem).toHaveTextContent(/agent sees this in its goal/)
  expect(screen.queryByRole("button", { name: "Retry" })).not.toBeInTheDocument()
  expect(api.getTaskScriptRunLog).not.toHaveBeenCalled()
  expect(within(problem).getByRole("button", { name: "Log" })).toBeInTheDocument()
})

it("shows a rejected last request only when it belongs to the current visit", () => {
  // Made at 10:03, before the open visit was entered at 10:04:30.
  const earlier = request("rejected", { result_message: "old rejection", created_at: "2026-10-01T10:03:00Z" })
  const { rerender } = renderProblem({ ...customerView, last_request: earlier })
  expect(screen.queryByRole("alert")).not.toBeInTheDocument()
  rerender({ ...customerView, last_request: { ...earlier, created_at: "2026-10-01T10:04:30Z" } })
  expect(screen.getByRole("alert")).toHaveTextContent("old rejection")
})

it("says nothing on a closed or paused task, or while a request is checking", () => {
  const failed = request("failed", { result_message: "check timed out" })
  for (const view of [
    { ...customerView, category: "done" as const, owner: "", outcomes: [], last_request: failed },
    { ...customerView, category: "cancelled" as const, owner: "", outcomes: [], last_request: failed },
    { ...poolView, waiting_on: "pause" as const, paused_reason: "script_failures", last_request: failed },
    { ...poolView, last_request: request("pending") },
  ]) {
    const { unmount } = renderProblem(view)
    expect(screen.queryByRole("alert")).not.toBeInTheDocument()
    expect(screen.queryByRole("status")).not.toBeInTheDocument()
    unmount()
  }
})

it("explains a failed watch with its log open and the failures counted", async () => {
  const watch: WorkflowScriptRun = { ...failedRun, id: 9, kind: "watch", script: "./scripts/ci.sh", request_id: undefined, message: "GitHub answered 502" }
  await act(async () => {
    renderProblem({ ...customerView, status: "publish", owner: "script", waiting_on: "script", outcomes: [], runs: [watch],
      visits: [{ id: 3, sequence: 3, status: "publish", entered_at: "2026-10-01T10:05:00Z", entered_by: "u", script_failures: 2 }] })
  })
  const problem = screen.getByRole("alert")
  expect(problem).toHaveAttribute("data-problem", "watch_failed")
  expect(problem).toHaveTextContent("The watch script failed")
  expect(problem).toHaveTextContent("2 of 3 script failures")
  expect(api.getTaskScriptRunLog).toHaveBeenCalledWith("REL-1", 9, undefined)
})

it("says a pool status waits for an agent, and counts idle iterations quietly", () => {
  const { unmount } = renderProblem({ ...poolView, holder: undefined, category: "open" })
  expect(screen.getByRole("alert")).toHaveTextContent("Waiting for an agent")
  unmount()
  renderProblem({ ...poolView, visits: [{ id: 2, sequence: 2, status: "draft", entered_at: "2026-10-01T10:04:30Z", entered_by: "u", idle_iterations: 2 }] })
  expect(screen.queryByRole("alert")).not.toBeInTheDocument()
  expect(screen.getByRole("status")).toHaveTextContent("2 of 3 idle iterations")
})
