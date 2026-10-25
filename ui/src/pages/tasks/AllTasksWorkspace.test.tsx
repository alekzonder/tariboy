import { act, render, screen, waitFor, within } from "@/test/queryRender";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import type { HostAgents } from "@/lib/aggregate";
import type { Task } from "@/lib/api";
import AllTasksWorkspace from "./AllTasksWorkspace";
import { resetTaskColumns } from "./taskColumnLayout";

const api = vi.hoisted(() => ({
  createTask: vi.fn(),
  getTask: vi.fn(),
  listTaskEvents: vi.fn(),
  listTaskPrincipals: vi.fn(),
  listTaskQueues: vi.fn(),
  listTasks: vi.fn(),
  addTaskComment: vi.fn(),
  exportTask: vi.fn(),
  importTask: vi.fn(),
  updateTask: vi.fn(),
}));
// Where each request takes the host it goes to in the mocks: every client the
// workspace creates passes its own host there, so an assertion that names a
// host proves the request went through that host's client.
const hostArgument = vi.hoisted((): Record<string, number> => ({
  createTask: 1,
  getTask: 1,
  listTaskEvents: 3,
  listTaskPrincipals: 0,
  listTaskQueues: 0,
  listTasks: 1,
  addTaskComment: 2,
  exportTask: 1,
  importTask: 1,
  updateTask: 2,
}));
vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  return {
    ...actual,
    createApi: (target: Parameters<typeof actual.createApi>[0]) => {
      const client = actual.createApi(target);
      const tasks: Record<string, unknown> = { ...client.tasks };
      for (const [name, at] of Object.entries(hostArgument)) {
        tasks[name] = (...args: unknown[]) => {
          args.splice(at, 0, client.target);
          return (api as Record<string, (...values: unknown[]) => unknown>)[name](...args);
        };
      }
      return { ...client, tasks };
    },
  };
});
vi.mock("@/lib/terminalsHost", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/terminalsHost")>()),
  targetFor: (id: string) =>
    id === "" ? null : { id, label: id, baseURL: `https://${id}`, token: "t" },
}));
vi.mock("@/hooks/useTasksSocket", () => ({ useTasksSocket: vi.fn(() => "open") }));
const toast = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn() }));
vi.mock("sonner", () => ({ toast }));

function task(key: string, extra: Partial<Task> = {}): Task {
  return {
    key,
    queue: key.split("-")[0],
    parent_key: "",
    position: 0,
    priority: "P2",
    title: `Title ${key}`,
    description: "",
    status: "open",
    category: "open",
    author: "user:me",
    customer: "user:me",
    group: "",
    assignee: "",
    manual_block_reason: "",
    blocked: false,
    revision: 1,
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
    completed_at: "",
    started_at: "",
    pull_request: "",
    access: "write",
    ...extra,
  };
}

const agent = (name: string) => ({ name }) as HostAgents["agents"][number];
const hosts: HostAgents[] = [
  { host: { id: "", label: "local" }, agents: [agent("jack")] },
  { host: { id: "h2", label: "hetzner-02" }, agents: [agent("builder")] },
  { host: { id: "down", label: "gone" }, agents: [], error: "unreachable" },
];

const tasksByServer: Record<string, Task[]> = {
  "": [
    task("IMPROVE-a1", { assignee: "agent:jack", updated_at: "2026-09-02T10:00:00Z" }),
    task("TASK-b2", { updated_at: "2026-09-01T09:00:00Z" }),
  ],
  h2: [task("IMPROVE-c3", { assignee: "agent:builder", updated_at: "2026-09-03T10:00:00Z" })],
};
const serverOf = (target: { id: string } | null | undefined) => target?.id ?? "";

beforeEach(() => {
  sessionStorage.clear();
  localStorage.clear();
  for (const fn of [...Object.values(api), ...Object.values(toast)]) fn.mockReset();
  api.listTasks.mockImplementation(async (_query: unknown, target: { id: string } | null) => ({
    tasks: tasksByServer[serverOf(target)] ?? [],
    sequence: 1,
  }));
  api.getTask.mockImplementation(async (key: string, target: { id: string } | null) => ({
    task: tasksByServer[serverOf(target)].find((item) => item.key === key),
    comments: [],
    waiting_for: [],
    relations: [],
  }));
  api.listTaskEvents.mockResolvedValue({ events: [], count: 0 });
  api.listTaskPrincipals.mockResolvedValue({ customer: "user:me", agents: [], groups: [] });
  api.listTaskQueues.mockResolvedValue({ queues: [{ prefix: "IMPROVE" }], count: 1 });
});

function rowKeys() {
  return screen
    .getAllByTestId(/^task-row-/)
    .map((row) => row.getAttribute("data-testid")!.replace("task-row-", ""));
}

it("lists every reachable server's tasks, newest first, with a Server column", async () => {
  render(<AllTasksWorkspace hosts={hosts} />);
  await waitFor(() => expect(rowKeys()).toEqual(["IMPROVE-c3", "IMPROVE-a1", "TASK-b2"]));

  const header = screen.getByTestId("task-table-header");
  expect(within(header).getByText("Server")).toBeInTheDocument();
  expect(within(header).getByText("Agent")).toBeInTheDocument();
  expect(
    within(screen.getByTestId("task-row-IMPROVE-c3")).getByText("hetzner-02"),
  ).toBeInTheDocument();
  expect(
    within(screen.getByTestId("task-row-IMPROVE-c3")).getByText("builder"),
  ).toBeInTheDocument();
  expect(screen.queryByRole("checkbox")).toBeNull();
});

it("reorders the every-server table from its Columns menu", async () => {
  render(<AllTasksWorkspace hosts={hosts} />);
  await waitFor(() => expect(rowKeys()).toHaveLength(3));

  await userEvent.click(screen.getByRole("button", { name: "Columns" }));
  await userEvent.click(screen.getByRole("menuitem", { name: "Move Server left" }));

  const order = (scope: HTMLElement) =>
    [...scope.querySelectorAll("[data-column]")].map((cell) => cell.getAttribute("data-column"));
  const header = screen.getByTestId("task-table-header");
  expect(order(header)).toEqual([
    "priority",
    "key",
    "task",
    "server",
    "agent",
    "status",
    "updated",
  ]);
  expect(order(screen.getByTestId("task-row-IMPROVE-c3"))).toEqual(order(header));
  act(() => resetTaskColumns());
});

it("reports an unavailable server above the list and retries only it", async () => {
  render(<AllTasksWorkspace hosts={hosts} />);
  const warning = await screen.findByText(/gone unavailable/);
  api.listTasks.mockClear();
  await userEvent.click(
    within(warning.closest("[data-testid=server-warning]") as HTMLElement).getByRole("button", {
      name: "Retry",
    }),
  );
  await waitFor(() => expect(api.listTasks).toHaveBeenCalledTimes(1));
  expect(api.listTasks).toHaveBeenCalledWith(
    expect.anything(),
    expect.objectContaining({ id: "down" }),
  );
});

it("merges queues by prefix across servers and narrows by server", async () => {
  render(<AllTasksWorkspace hosts={hosts} />);
  await waitFor(() => expect(rowKeys()).toHaveLength(3));

  await userEvent.click(screen.getByRole("button", { name: "Queue: all" }));
  const improve = await screen.findByRole("menuitem", { name: /IMPROVE/ });
  expect(improve).toHaveTextContent("2");
  await userEvent.click(improve);
  expect(rowKeys()).toEqual(["IMPROVE-c3", "IMPROVE-a1"]);

  await userEvent.click(await screen.findByRole("button", { name: "Server: all" }));
  const gone = await screen.findByRole("menuitem", { name: /gone/ });
  expect(gone).toHaveAttribute("aria-disabled", "true");
  await userEvent.click(screen.getByRole("menuitem", { name: /hetzner-02/ }));
  expect(rowKeys()).toEqual(["IMPROVE-c3"]);
});

it("shows an empty state that clears the filters", async () => {
  api.listTasks.mockResolvedValue({ tasks: [], sequence: 1 });
  render(<AllTasksWorkspace hosts={hosts} />);
  await userEvent.type(screen.getByLabelText("Search tasks"), "zzz");
  expect(await screen.findByText("No tasks")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Clear filters" }));
  expect(screen.getByLabelText("Search tasks")).toHaveValue("");
});

it("opens a task on its own server", async () => {
  render(<AllTasksWorkspace hosts={hosts} />);
  await userEvent.click(await screen.findByText("Title IMPROVE-c3"));
  await waitFor(() =>
    expect(api.getTask).toHaveBeenCalledWith(
      "IMPROVE-c3",
      expect.objectContaining({ id: "h2" }),
      expect.any(AbortSignal),
    ),
  );
  expect(await screen.findByRole("button", { name: "Save task" })).toBeInTheDocument();
});

async function pickRemoteAssignee() {
  api.updateTask.mockImplementation(async (key: string, input: Partial<Task>) => ({
    ...task(key),
    ...input,
  }));
  api.exportTask.mockResolvedValue({
    root_key: "TASK-b2",
    queue: "TASK",
    relations: [],
    tasks: [
      { key: "TASK-b2", assignee: "agent:jack" },
      { key: "TASK-child", assignee: "agent:jack" },
    ],
  });
  api.importTask.mockResolvedValue(task("TASK-b2"));
  api.addTaskComment.mockResolvedValue({});
  await userEvent.click(await screen.findByText("Title TASK-b2"));
  const assignee = await screen.findByRole("combobox", { name: "Assignee" });
  expect(within(assignee).getByRole("option", { name: "jack · local" })).toBeInTheDocument();
  await userEvent.selectOptions(
    assignee,
    within(assignee).getByRole("option", { name: "builder · hetzner-02" }),
  );
}

it("moves a task to the server of an agent picked from another server and assigns it there", async () => {
  render(<AllTasksWorkspace hosts={hosts} />);
  await pickRemoteAssignee();
  api.getTask.mockImplementation(async (key: string, target: { id: string } | null) => ({
    task: task(key, { revision: target ? 9 : 4 }),
    comments: [],
    waiting_for: [],
    relations: [],
  }));
  await userEvent.click(screen.getByRole("button", { name: "Save task" }));

  await waitFor(() =>
    expect(api.updateTask).toHaveBeenCalledWith(
      "TASK-b2",
      { assignee: "agent:builder", revision: 9 },
      expect.objectContaining({ id: "h2" }),
    ),
  );
  // The root arrives unassigned, so no same-named agent there can take it
  // before the real assignment; the rest of the tree is untouched.
  expect(api.importTask).toHaveBeenCalledWith(
    expect.objectContaining({
      tasks: [
        { key: "TASK-b2", assignee: "" },
        { key: "TASK-child", assignee: "agent:jack" },
      ],
    }),
    expect.objectContaining({ id: "h2" }),
  );
  expect(api.addTaskComment).toHaveBeenCalledWith(
    "TASK-b2",
    "Moved to hetzner-02 as TASK-b2.",
    null,
    expect.any(String),
  );
  expect(api.updateTask).toHaveBeenCalledWith(
    "TASK-b2",
    { status: "cancelled", revision: 4 },
    null,
  );
  // Nothing else was edited, so the source is not written before the move.
  expect(api.updateTask).toHaveBeenCalledTimes(2);
  // The sheet follows the task.
  await waitFor(() =>
    expect(api.getTask).toHaveBeenLastCalledWith(
      "TASK-b2",
      expect.objectContaining({ id: "h2" }),
      expect.any(AbortSignal),
    ),
  );
});

it("follows the task to its new server even when a step after the import fails", async () => {
  render(<AllTasksWorkspace hosts={hosts} />);
  await pickRemoteAssignee();
  api.getTask.mockImplementation(async (key: string, target: { id: string } | null) => {
    if (target) throw new Error("tunnel dropped");
    return {
      task: task(key, { revision: 4 }),
      comments: [],
      waiting_for: [],
      relations: [],
      descendants: 0,
      active_descendants: 0,
    };
  });
  await userEvent.click(screen.getByRole("button", { name: "Save task" }));

  await waitFor(() =>
    expect(toast.error).toHaveBeenCalledWith(
      "TASK-b2 moved to hetzner-02, but the move did not finish: tunnel dropped",
    ),
  );
  await waitFor(() =>
    expect(api.getTask).toHaveBeenLastCalledWith(
      "TASK-b2",
      expect.objectContaining({ id: "h2" }),
      expect.any(AbortSignal),
    ),
  );
});

it("refuses an assignee whose server dropped out of the list before Save", async () => {
  const { rerender } = render(<AllTasksWorkspace hosts={hosts} />);
  await pickRemoteAssignee();
  rerender(
    <AllTasksWorkspace hosts={[hosts[0], { ...hosts[1], error: "unreachable" }, hosts[2]]} />,
  );
  await userEvent.click(screen.getByRole("button", { name: "Save task" }));

  await waitFor(() =>
    expect(toast.error).toHaveBeenCalledWith(
      "That agent's server is unavailable; pick the assignee again",
    ),
  );
  expect(api.updateTask).not.toHaveBeenCalled();
  expect(api.importTask).not.toHaveBeenCalled();
});

it("creates a task for the chosen agent on that agent's server", async () => {
  api.createTask.mockResolvedValue(task("IMPROVE-new"));
  api.getTask.mockResolvedValue({
    task: task("IMPROVE-new"),
    comments: [],
    waiting_for: [],
    relations: [],
  });
  render(<AllTasksWorkspace hosts={hosts} />);
  await waitFor(() => expect(rowKeys().length).toBeGreaterThan(0));
  await userEvent.click(screen.getByRole("button", { name: "New task" }));
  expect(screen.getByRole("combobox", { name: "Task agent" })).toHaveDisplayValue(
    "Choose an agent",
  );
  await userEvent.selectOptions(
    screen.getByRole("combobox", { name: "Task agent" }),
    "builder · hetzner-02",
  );
  await waitFor(() =>
    expect(screen.getByRole("combobox", { name: "Task queue" })).toHaveDisplayValue("IMPROVE"),
  );
  await userEvent.type(screen.getByRole("textbox", { name: "Task title" }), "Ship it");
  await userEvent.click(screen.getByRole("button", { name: "Create task" }));
  await waitFor(() =>
    expect(api.createTask).toHaveBeenCalledWith(
      { queue: "IMPROVE", title: "Ship it", assignee: "agent:builder" },
      expect.objectContaining({ id: "h2" }),
    ),
  );
});

it("preselects each server's remembered queue and remembers the queue a task was created in", async () => {
  api.listTaskQueues.mockImplementation(async (target: { id: string } | null) => ({
    queues:
      serverOf(target) === ""
        ? [{ prefix: "IMPROVE" }, { prefix: "DEV" }]
        : [{ prefix: "IMPROVE" }, { prefix: "OPS" }],
    count: 2,
  }));
  localStorage.setItem("tasks:new-task-queue:v1", JSON.stringify({ local: "DEV", h2: "OPS" }));
  api.createTask.mockResolvedValue(task("IMPROVE-new"));
  api.getTask.mockResolvedValue({
    task: task("IMPROVE-new"),
    comments: [],
    waiting_for: [],
    relations: [],
  });
  render(<AllTasksWorkspace hosts={hosts} />);
  await waitFor(() => expect(rowKeys().length).toBeGreaterThan(0));
  await userEvent.click(screen.getByRole("button", { name: "New task" }));
  const agentPicker = screen.getByRole("combobox", { name: "Task agent" });
  const queuePicker = () => screen.getByRole("combobox", { name: "Task queue" });

  await userEvent.selectOptions(agentPicker, "jack · local");
  await waitFor(() => expect(queuePicker()).toHaveDisplayValue("DEV"));
  await userEvent.selectOptions(agentPicker, "builder · hetzner-02");
  await waitFor(() => expect(queuePicker()).toHaveDisplayValue("OPS"));

  await userEvent.selectOptions(queuePicker(), "IMPROVE");
  await userEvent.type(screen.getByRole("textbox", { name: "Task title" }), "Ship it");
  await userEvent.click(screen.getByRole("button", { name: "Create task" }));
  await waitFor(() =>
    expect(JSON.parse(localStorage.getItem("tasks:new-task-queue:v1") ?? "{}")).toEqual({
      local: "DEV",
      h2: "IMPROVE",
    }),
  );
});

it("saves a priority on the row server and reloads only that server", async () => {
  render(<AllTasksWorkspace hosts={hosts} />);
  const row = await screen.findByTestId("task-row-IMPROVE-c3");
  api.updateTask.mockResolvedValue(task("IMPROVE-c3", { priority: "P0", revision: 2 }));
  api.listTasks.mockClear();

  await userEvent.click(within(row).getByRole("button", { name: "P2 Normal" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: /P0/ }));

  expect(api.updateTask).toHaveBeenCalledWith(
    "IMPROVE-c3",
    { priority: "P0", revision: 1 },
    expect.objectContaining({ id: "h2" }),
  );
  await waitFor(() => expect(api.listTasks).toHaveBeenCalledTimes(1));
  expect(api.listTasks).toHaveBeenCalledWith(
    expect.anything(),
    expect.objectContaining({ id: "h2" }),
  );
});

it("reports a failed priority change", async () => {
  render(<AllTasksWorkspace hosts={hosts} />);
  const row = await screen.findByTestId("task-row-TASK-b2");
  api.updateTask.mockRejectedValue(new Error("forbidden"));

  await userEvent.click(within(row).getByRole("button", { name: "P2 Normal" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: /P1/ }));

  await waitFor(() => expect(toast.error).toHaveBeenCalledWith("forbidden"));
});
