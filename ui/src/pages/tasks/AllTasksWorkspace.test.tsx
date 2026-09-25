import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, expect, it, vi } from "vitest"
import type { HostAgents } from "@/lib/aggregate"
import type { Task } from "@/lib/tasks"
import AllTasksWorkspace from "./AllTasksWorkspace"

const api = vi.hoisted(() => ({
  createTask: vi.fn(),
  getTask: vi.fn(),
  getTaskWorkflow: vi.fn(),
  listTaskEvents: vi.fn(),
  listTaskPrincipals: vi.fn(),
  listTaskQueues: vi.fn(),
  listTasks: vi.fn(),
  transferTask: vi.fn(),
  updateTask: vi.fn(),
}))
vi.mock("@/lib/tasks", async (importOriginal) => ({
  ...await importOriginal<typeof import("@/lib/tasks")>(),
  ...api,
}))
vi.mock("@/lib/terminalsHost", async (importOriginal) => ({
  ...await importOriginal<typeof import("@/lib/terminalsHost")>(),
  targetFor: (id: string) => (id === "" ? null : { id, label: id, baseURL: `https://${id}`, token: "t" }),
}))
vi.mock("@/hooks/useTasksSocket", () => ({ useTasksSocket: vi.fn(() => "open") }))
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

function task(key: string, extra: Partial<Task> = {}): Task {
  return {
    key, queue: key.split("-")[0], parent_key: "", position: 0, priority: "P2", title: `Title ${key}`,
    description: "", status: "open", author: "user:me", customer: "user:me", group: "", assignee: "",
    manual_block_reason: "", blocked: false, revision: 1, created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z", completed_at: "", access: "write", ...extra,
  }
}

const agent = (name: string) => ({ name }) as HostAgents["agents"][number]
const hosts: HostAgents[] = [
  { host: { id: "", label: "local" }, agents: [agent("jack")] },
  { host: { id: "h2", label: "hetzner-02" }, agents: [agent("builder")] },
  { host: { id: "down", label: "gone" }, agents: [], error: "unreachable" },
]

const tasksByServer: Record<string, Task[]> = {
  "": [
    task("IMPROVE-a1", { assignee: "agent:jack", updated_at: "2026-09-02T10:00:00Z" }),
    task("TASK-b2", { updated_at: "2026-09-01T09:00:00Z" }),
  ],
  h2: [task("IMPROVE-c3", { assignee: "agent:builder", updated_at: "2026-09-03T10:00:00Z" })],
}
const serverOf = (target: { id: string } | null | undefined) => target?.id ?? ""

beforeEach(() => {
  sessionStorage.clear()
  for (const fn of Object.values(api)) fn.mockReset()
  api.listTasks.mockImplementation(async (_query: unknown, target: { id: string } | null) =>
    ({ tasks: tasksByServer[serverOf(target)] ?? [], sequence: 1 }))
  api.getTask.mockImplementation(async (key: string, target: { id: string } | null) => ({
    task: tasksByServer[serverOf(target)].find((item) => item.key === key),
    comments: [], waiting_for: [], relations: [],
  }))
  api.listTaskEvents.mockResolvedValue({ events: [], count: 0 })
  api.listTaskPrincipals.mockResolvedValue({ customer: "user:me", agents: [], groups: [] })
  api.listTaskQueues.mockResolvedValue({ queues: [{ prefix: "IMPROVE" }], count: 1 })
})

function rowKeys() {
  return screen.getAllByTestId(/^task-row-/).map((row) => row.getAttribute("data-testid")!.replace("task-row-", ""))
}

it("lists every reachable server's tasks, newest first, with a Server column", async () => {
  render(<AllTasksWorkspace hosts={hosts} />)
  await waitFor(() => expect(rowKeys()).toEqual(["IMPROVE-c3", "IMPROVE-a1", "TASK-b2"]))

  const header = screen.getByTestId("task-table-header")
  expect(within(header).getByText("Server")).toBeInTheDocument()
  expect(within(header).getByText("Agent")).toBeInTheDocument()
  expect(within(screen.getByTestId("task-row-IMPROVE-c3")).getByText("hetzner-02")).toBeInTheDocument()
  expect(within(screen.getByTestId("task-row-IMPROVE-c3")).getByText("builder")).toBeInTheDocument()
  expect(screen.queryByRole("checkbox")).toBeNull()
})

it("reports an unavailable server above the list and retries only it", async () => {
  render(<AllTasksWorkspace hosts={hosts} />)
  const warning = await screen.findByText(/gone unavailable/)
  api.listTasks.mockClear()
  await userEvent.click(within(warning.closest("[data-testid=server-warning]") as HTMLElement)
    .getByRole("button", { name: "Retry" }))
  await waitFor(() => expect(api.listTasks).toHaveBeenCalledTimes(1))
  expect(api.listTasks).toHaveBeenCalledWith(expect.anything(), expect.objectContaining({ id: "down" }))
})

it("merges queues by prefix across servers and narrows by server", async () => {
  render(<AllTasksWorkspace hosts={hosts} />)
  await waitFor(() => expect(rowKeys()).toHaveLength(3))

  await userEvent.click(screen.getByRole("button", { name: "Queue: all" }))
  const improve = await screen.findByRole("menuitem", { name: /IMPROVE/ })
  expect(improve).toHaveTextContent("2")
  await userEvent.click(improve)
  expect(rowKeys()).toEqual(["IMPROVE-c3", "IMPROVE-a1"])

  await userEvent.click(await screen.findByRole("button", { name: "Server: all" }))
  const gone = await screen.findByRole("menuitem", { name: /gone/ })
  expect(gone).toHaveAttribute("aria-disabled", "true")
  await userEvent.click(screen.getByRole("menuitem", { name: /hetzner-02/ }))
  expect(rowKeys()).toEqual(["IMPROVE-c3"])
})

it("narrows to the selected agent until the chip is cleared", async () => {
  const onClearAgent = vi.fn()
  render(<AllTasksWorkspace hosts={hosts} agent={{ hostId: "h2", name: "builder" }} onClearAgent={onClearAgent} />)
  await waitFor(() => expect(rowKeys()).toEqual(["IMPROVE-c3"]))
  await userEvent.click(screen.getByRole("button", { name: "Clear agent filter" }))
  expect(onClearAgent).toHaveBeenCalled()
})

it("shows an empty state that clears the filters", async () => {
  const onClearAgent = vi.fn()
  render(<AllTasksWorkspace hosts={hosts} agent={{ hostId: "", name: "nobody" }} onClearAgent={onClearAgent} />)
  expect(await screen.findByText("No tasks")).toBeInTheDocument()
  await userEvent.click(screen.getByRole("button", { name: "Clear filters" }))
  expect(onClearAgent).toHaveBeenCalled()
})

it("opens a task on its own server", async () => {
  render(<AllTasksWorkspace hosts={hosts} />)
  await userEvent.click(await screen.findByText("Title IMPROVE-c3"))
  await waitFor(() => expect(api.getTask).toHaveBeenCalledWith("IMPROVE-c3", expect.objectContaining({ id: "h2" })))
  expect(await screen.findByRole("button", { name: "Save task" })).toBeInTheDocument()
})

it("moves a task to the server of an agent picked from another server", async () => {
  api.transferTask.mockResolvedValue(undefined)
  api.updateTask.mockImplementation(async (key: string, input: Partial<Task>) => ({ ...task(key), ...input }))
  render(<AllTasksWorkspace hosts={hosts} />)
  await userEvent.click(await screen.findByText("Title TASK-b2"))

  const assignee = await screen.findByRole("combobox", { name: "Assignee" })
  expect(within(assignee).getByRole("option", { name: "jack · local" })).toBeInTheDocument()
  await userEvent.selectOptions(assignee, within(assignee).getByRole("option", { name: "builder · hetzner-02" }))
  api.getTask.mockResolvedValueOnce({ task: task("TASK-b2", { revision: 9 }), comments: [], waiting_for: [], relations: [] })
  await userEvent.click(screen.getByRole("button", { name: "Save task" }))

  await waitFor(() => expect(api.transferTask).toHaveBeenCalledWith(
    "TASK-b2", null, expect.objectContaining({ id: "h2" }), "hetzner-02", expect.any(String),
  ))
  await waitFor(() => expect(api.updateTask).toHaveBeenLastCalledWith(
    "TASK-b2", { assignee: "agent:builder", revision: 9 }, expect.objectContaining({ id: "h2" }),
  ))
  // Everything but the assignee is saved on the source before the move.
  expect(api.updateTask.mock.calls[0][2]).toBeNull()
  expect(api.updateTask.mock.calls[0][1]).toMatchObject({ assignee: "" })
})

it("creates a task for the chosen agent on that agent's server", async () => {
  api.createTask.mockResolvedValue(task("IMPROVE-new"))
  render(<AllTasksWorkspace hosts={hosts} agent={{ hostId: "h2", name: "builder" }} onClearAgent={() => {}} />)
  await waitFor(() => expect(rowKeys()).toHaveLength(1))
  await userEvent.click(screen.getByRole("button", { name: "New task" }))
  expect(screen.getByRole("combobox", { name: "Task agent" })).toHaveDisplayValue("builder · hetzner-02")
  await waitFor(() => expect(screen.getByRole("combobox", { name: "Task queue" })).toHaveDisplayValue("IMPROVE"))
  await userEvent.type(screen.getByRole("textbox", { name: "Task title" }), "Ship it")
  await userEvent.click(screen.getByRole("button", { name: "Create task" }))
  await waitFor(() => expect(api.createTask).toHaveBeenCalledWith(
    { queue: "IMPROVE", title: "Ship it", assignee: "agent:builder" }, expect.objectContaining({ id: "h2" }),
  ))
})
