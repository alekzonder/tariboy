import type { ReactElement } from "react"
import { act, fireEvent, render as renderWith, screen, waitFor, within } from "@/test/queryRender"
import userEvent from "@testing-library/user-event"
import { beforeEach, expect, it, vi } from "vitest"
import { ApiProvider } from "@/components/ApiProvider"
import { ApiError, type ApiClient, type Task, type TaskDetail as Detail } from "@/lib/api"
import type { Daemon } from "@/lib/daemons"
import { fakeApi } from "@/test/api"
import TaskDrawer from "./TaskDrawer"
import { customerView, workflowTask } from "./workflowFixtures"

const api = vi.hoisted(() => ({
  addTaskComment: vi.fn(),
  getTask: vi.fn(),
  getTaskWorkflow: vi.fn(),
  listTaskEvents: vi.fn(),
  listTaskPrincipals: vi.fn(),
  listTasks: vi.fn(),
  updateTask: vi.fn(),
}))
const toast = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn() }))
const taskSocket = vi.hoisted(() => ({
  options: undefined as {
    after?: number
    enabled?: boolean
    onHint?: (hint: { sequence: number; task_key?: string }) => void
    onReset?: (sequence: number) => void
  } | undefined,
}))

// One fake client per host identity, each with the same mocks: a test that
// switches hosts sees the drawer get a new client, as ApiProvider would give it.
const clients = new Map<string, ApiClient>()
function hostApi(target: Daemon | null): ApiClient {
  const identity = JSON.stringify(target)
  if (!clients.has(identity)) {
    const { getTaskWorkflow, ...tasks } = api
    clients.set(identity, fakeApi({ tasks, workflows: { getTaskWorkflow } }, target))
  }
  return clients.get(identity)!
}
const render = (ui: ReactElement) => renderWith(ui, { api: hostApi(null) })
const onHost = (target: Daemon, ui: ReactElement) => <ApiProvider client={hostApi(target)}>{ui}</ApiProvider>
vi.mock("@/hooks/useTasksSocket", () => ({
  useTasksSocket: vi.fn((options) => {
    taskSocket.options = options
    return "open"
  }),
}))
vi.mock("sonner", () => ({ toast }))

const task: Task = {
  key: "TEST-1",
  queue: "TEST",
  parent_key: "",
  position: 0,
  priority: "P1",
  title: "Ship the drawer",
  description: "Open a task from chat",
  status: "open",
  category: "open",
  pull_request: "",
  author: "user:owner",
  customer: "user:owner",
  group: "",
  assignee: "agent:worker",
  manual_block_reason: "",
  blocked: false,
  revision: 2,
  created_at: "2026-07-31T10:00:00Z",
  updated_at: "2026-07-31T10:00:00Z",
  completed_at: "",
}
const detail: Detail = { task, comments: [], waiting_for: [], relations: [] }

it("does not show one server's detail while the same key loads on another", async () => {
  const first = { id: "first", label: "First", baseURL: "https://first.test", token: "" }
  const second = { ...first, id: "second", baseURL: "https://second.test" }
  const mounted = render(onHost(first, <TaskDrawer taskKey="TEST-1" onClose={vi.fn()} />))
  await screen.findByText("Ship the drawer")
  api.getTask.mockReturnValue(new Promise(() => {}))
  mounted.rerender(onHost(second, <TaskDrawer taskKey="TEST-1" onClose={vi.fn()} />))
  expect(screen.queryByText("Ship the drawer")).not.toBeInTheDocument()
  expect(screen.getByText("Loading task…")).toBeInTheDocument()
})

it("shares an in-flight detail read between two consumers on the same server", async () => {
  let finish!: (value: Detail) => void
  api.getTask.mockReturnValue(new Promise<Detail>((resolve) => { finish = resolve }))
  render(<><TaskDrawer taskKey="TEST-1" onClose={vi.fn()} /><TaskDrawer taskKey="TEST-1" onClose={vi.fn()} /></>)
  await waitFor(() => expect(api.getTask).toHaveBeenCalledTimes(1))
  await act(async () => finish(detail))
  expect(await screen.findAllByText("Ship the drawer")).toHaveLength(2)
})

it("shows an initial network failure in a closable detail panel", async () => {
  api.getTask.mockRejectedValue(new ApiError(0, "network_error", "Server unavailable"))
  const onClose = vi.fn()
  render(<TaskDrawer taskKey="TEST-1" onClose={onClose} />)
  expect(await screen.findByRole("alert")).toHaveTextContent("Server unavailable")
  fireEvent.click(screen.getByRole("button", { name: "Close task detail" }))
  expect(onClose).toHaveBeenCalledOnce()
})

it("aborts a detail read when its last consumer closes", async () => {
  api.getTask.mockReturnValue(new Promise(() => {}))
  const mounted = render(<TaskDrawer taskKey="TEST-1" onClose={vi.fn()} />)
  await waitFor(() => expect(api.getTask).toHaveBeenCalledTimes(1))
  const signal = api.getTask.mock.calls[0][1] as AbortSignal | undefined
  mounted.unmount()
  expect(signal?.aborted).toBe(true)
})

it("does not read an unresolved host or fall back to the local server", async () => {
  render(onHost({ id: "offline", label: "Offline", baseURL: "", token: "" }, <TaskDrawer taskKey="TEST-1" onClose={vi.fn()} />))
  await act(async () => {})
  expect(api.getTask).not.toHaveBeenCalled()
  expect(api.listTasks).not.toHaveBeenCalled()
  expect(screen.getByText("Loading task…")).toBeInTheDocument()
})

it("clears the old account's detail before reading with changed credentials", async () => {
  const host = { id: "remote", label: "Remote", baseURL: "https://remote.test", token: "old" }
  const mounted = render(onHost(host, <TaskDrawer taskKey="TEST-1" onClose={vi.fn()} />))
  await screen.findByText("Ship the drawer")
  api.getTask.mockReturnValue(new Promise(() => {}))
  mounted.rerender(onHost({ ...host, token: "new" }, <TaskDrawer taskKey="TEST-1" onClose={vi.fn()} />))
  expect(screen.queryByText("Ship the drawer")).not.toBeInTheDocument()
})

it("removes a loaded detail when access is denied during refresh", async () => {
  render(<TaskDrawer taskKey="TEST-1" onClose={vi.fn()} />)
  await screen.findByText("Ship the drawer")
  api.getTask.mockRejectedValue(new ApiError(403, "forbidden", "Access denied"))
  await act(async () => { taskSocket.options?.onHint?.({ sequence: 420 }) })
  await waitFor(() => expect(screen.queryByText("Ship the drawer")).not.toBeInTheDocument())
  expect(await screen.findByText("Access denied")).toBeInTheDocument()
})

beforeEach(() => {
  vi.clearAllMocks()
  clients.clear()
  localStorage.clear()
  taskSocket.options = undefined
  api.getTask.mockResolvedValue(detail)
  api.listTaskEvents.mockResolvedValue({ events: [], count: 0 })
  api.listTasks.mockResolvedValue({ tasks: [], sequence: 412 })
  api.listTaskPrincipals.mockResolvedValue({ customer: "user:owner", agents: ["worker"], groups: [] })
  api.updateTask.mockResolvedValue({ ...task, status: "in_progress", category: "in_progress", revision: 3 })
  api.addTaskComment.mockResolvedValue({ comment: null, created_waits: [], resolved_waits: [] })
})

it("loads the task it was given and closes without touching the tasks list", async () => {
  const onClose = vi.fn()
  render(<TaskDrawer taskKey="TEST-1" onClose={onClose} />)
  expect(await screen.findByText("Ship the drawer")).toBeInTheDocument()
  expect(api.getTask).toHaveBeenCalledWith("TEST-1", expect.any(AbortSignal))
  await userEvent.click(screen.getByRole("button", { name: "Close task detail" }))
  expect(onClose).toHaveBeenCalled()
})

it("saves task edits from the drawer itself", async () => {
  render(<TaskDrawer taskKey="TEST-1" onClose={vi.fn()} />)
  await screen.findByText("Ship the drawer")
  await userEvent.selectOptions(screen.getByLabelText("Status"), "in_progress")
  await userEvent.click(screen.getByRole("button", { name: "Save task" }))
  await waitFor(() => expect(api.updateTask).toHaveBeenCalledWith(
    "TEST-1",
    expect.objectContaining({ revision: 2, status: "in_progress" }),
  ))
})

it("posts a comment and reloads the task", async () => {
  render(<TaskDrawer taskKey="TEST-1" onClose={vi.fn()} />)
  await screen.findByText("Ship the drawer")
  const comments = screen.getByText("Comments").closest("section")!
  await userEvent.click(within(comments).getByRole("button", { name: "Markdown" }))
  await userEvent.type(screen.getByLabelText("Comment"), "on it")
  await userEvent.click(screen.getByRole("button", { name: "Send comment" }))
  await waitFor(() => expect(api.addTaskComment).toHaveBeenCalledWith(
    "TEST-1", expect.stringContaining("on it"), expect.any(String),
  ))
  await waitFor(() => expect(api.getTask).toHaveBeenCalledTimes(2))
})

it("reloads the task when the tasks socket hints a change", async () => {
  render(<TaskDrawer taskKey="TEST-1" onClose={vi.fn()} />)
  await screen.findByText("Ship the drawer")
  expect(api.getTask).toHaveBeenCalledTimes(1)
  taskSocket.options?.onHint?.({ sequence: 7 })
  await waitFor(() => expect(api.getTask).toHaveBeenCalledTimes(2))
})

it("stays open with the loaded task when a reload cannot reach the host", async () => {
  const onClose = vi.fn()
  render(<TaskDrawer taskKey="TEST-1" onClose={onClose} />)
  await screen.findByText("Ship the drawer")
  api.getTask.mockRejectedValue(new ApiError(0, "network_error", "network error: Failed to fetch"))
  taskSocket.options?.onHint?.({ sequence: 7 })
  await waitFor(() => expect(api.getTask).toHaveBeenCalledTimes(2))
  await act(async () => {})

  expect(onClose).not.toHaveBeenCalled()
  expect(screen.getByText("Ship the drawer")).toBeInTheDocument()
  expect(toast.error).not.toHaveBeenCalled()
})

it("refetches a workflow task's panel on a hint for this task, and not for another", async () => {
  // The drawer's history is the oldest page and the revision stays the same:
  // only the hint can tell the panel something happened.
  api.getTask.mockResolvedValue({ ...detail, task: workflowTask })
  api.getTaskWorkflow.mockResolvedValue(customerView)
  render(<TaskDrawer taskKey="REL-1" onClose={vi.fn()} />)
  await screen.findByText("release@1.2.0")
  await waitFor(() => expect(taskSocket.options?.enabled).toBe(true))
  expect(api.getTaskWorkflow).toHaveBeenCalledTimes(1)
  await act(async () => { taskSocket.options?.onHint?.({ sequence: 500, task_key: "OTHER-2" }) })
  await waitFor(() => expect(taskSocket.options?.after).toBe(500))
  expect(api.getTaskWorkflow).toHaveBeenCalledTimes(1)
  await act(async () => { taskSocket.options?.onHint?.({ sequence: 501, task_key: "REL-1" }) })
  await waitFor(() => expect(api.getTaskWorkflow).toHaveBeenCalledTimes(2))
})

it("opens at the width the Tasks tab saved and resizes it with the same handle", async () => {
  localStorage.setItem("tasks:workspace:v1", JSON.stringify({ schemaVersion: 1, detailWidth: 520 }))
  render(<TaskDrawer taskKey="TEST-1" onClose={vi.fn()} />)
  await screen.findByText("Ship the drawer")
  expect(screen.getByRole("dialog").style.getPropertyValue("--tasks-detail-width")).toBe("520px")

  const handle = screen.getByRole("separator", { name: "Resize task details" })
  fireEvent.keyDown(handle, { key: "ArrowLeft" })
  expect(screen.getByRole("dialog").style.getPropertyValue("--tasks-detail-width")).toBe("528px")
  expect(JSON.parse(localStorage.getItem("tasks:workspace:v1") ?? "{}"))
    .toEqual({ schemaVersion: 1, detailWidth: 528 })
})

it("drags the drawer from the right edge of the window", async () => {
  vi.stubGlobal("innerWidth", 1200)
  render(<TaskDrawer taskKey="TEST-1" onClose={vi.fn()} />)
  await screen.findByText("Ship the drawer")
  const handle = screen.getByRole("separator", { name: "Resize task details" })
  expect(handle).toHaveAttribute("aria-valuemax", "1120")

  fireEvent.pointerDown(handle, { pointerId: 1, button: 0, isPrimary: true })
  fireEvent.pointerMove(window, { pointerId: 1, clientX: 500 })
  fireEvent.pointerUp(window, { pointerId: 1 })
  expect(JSON.parse(localStorage.getItem("tasks:workspace:v1") ?? "{}"))
    .toMatchObject({ detailWidth: 700 })
  vi.unstubAllGlobals()
})

it("starts the tasks socket at the current sequence instead of replaying the whole log", async () => {
  render(<TaskDrawer taskKey="TEST-1" onClose={vi.fn()} />)
  await screen.findByText("Ship the drawer")
  await waitFor(() => expect(taskSocket.options?.enabled).toBe(true))
  expect(taskSocket.options?.after).toBe(412)
})

it("keeps the socket closed until the current sequence is known", async () => {
  let release: (page: { tasks: never[]; sequence: number }) => void = () => {}
  api.listTasks.mockReturnValue(new Promise((resolve) => { release = resolve }))
  render(<TaskDrawer taskKey="TEST-1" onClose={vi.fn()} />)
  await screen.findByText("Ship the drawer")
  expect(taskSocket.options?.enabled).toBe(false)
  release({ tasks: [], sequence: 9 })
  await waitFor(() => expect(taskSocket.options?.enabled).toBe(true))
})

it("ignores a hint about another task", async () => {
  render(<TaskDrawer taskKey="TEST-1" onClose={vi.fn()} />)
  await screen.findByText("Ship the drawer")
  expect(api.getTask).toHaveBeenCalledTimes(1)
  taskSocket.options?.onHint?.({ sequence: 413, task_key: "OTHER-2" })
  await waitFor(() => expect(taskSocket.options?.after).toBe(413))
  expect(api.getTask).toHaveBeenCalledTimes(1)
})

it("reloads on a socket reset without rewinding the stream to the beginning", async () => {
  render(<TaskDrawer taskKey="TEST-1" onClose={vi.fn()} />)
  await screen.findByText("Ship the drawer")
  await waitFor(() => expect(taskSocket.options?.after).toBe(412))
  taskSocket.options?.onReset?.(412)
  await waitFor(() => expect(api.getTask).toHaveBeenCalledTimes(2))
  expect(taskSocket.options?.after).toBe(412)
})

it("opens at once in a loading state and fills in when the task arrives", async () => {
  let resolve!: (value: Detail) => void
  api.getTask.mockReturnValue(new Promise<Detail>((done) => { resolve = done }))
  const onClose = vi.fn()
  render(<TaskDrawer taskKey="TEST-1" onClose={onClose} />)
  const dialog = await screen.findByRole("dialog", { name: "TEST-1" })
  expect(within(dialog).getByText("Loading task…")).toBeInTheDocument()
  expect(screen.queryByText("Ship the drawer")).not.toBeInTheDocument()
  resolve(detail)
  expect(await screen.findByText("Ship the drawer")).toBeInTheDocument()
  expect(screen.queryByText("Loading task…")).not.toBeInTheDocument()
})

it("shows a workflow task by its status label and category tone, and closes it without a draft prompt", async () => {
  api.getTask.mockResolvedValue({
    ...detail,
    task: { ...task, status: "implement", category: "in_progress", workflow_name: "delivery" },
  })
  const onClose = vi.fn()
  render(<TaskDrawer taskKey="TEST-1" onClose={onClose} />)
  const pill = (await screen.findAllByText("Implement"))[0].closest("[data-slot='status-pill']")!
  expect(pill).toHaveAttribute("data-tone", "live")
  await userEvent.click(screen.getByRole("button", { name: "Close task detail" }))
  expect(onClose).toHaveBeenCalled()
})

it("closes from the loading state", async () => {
  api.getTask.mockReturnValue(new Promise<Detail>(() => {}))
  const onClose = vi.fn()
  render(<TaskDrawer taskKey="TEST-1" onClose={onClose} />)
  await screen.findByText("Loading task…")
  await userEvent.click(screen.getByRole("button", { name: "Close task detail" }))
  expect(onClose).toHaveBeenCalled()
})
