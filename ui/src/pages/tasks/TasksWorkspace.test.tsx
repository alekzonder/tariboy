import { act, fireEvent, render, screen, waitFor, within } from "@/test/queryRender";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, setActiveDaemon, type Task } from "@/lib/api";
import {
  child,
  deferred,
  mockWorkspaceDefaults,
  onHost,
  queue,
  root,
} from "@/test/tasks/tasksWorkspace";
import {
  openManageQueues,
  pickTaskAssignee,
  pickTaskQueue,
  toggleWaitingForMe,
} from "@/test/tasks/taskFilters";
import TasksWorkspace from "./TasksWorkspace";

const api = vi.hoisted(() => ({
  addTaskComment: vi.fn(),
  addTaskRelation: vi.fn(),
  createTask: vi.fn(),
  createTaskQueue: vi.fn(),
  deleteTaskRelation: vi.fn(),
  advanceTask: vi.fn(),
  getTask: vi.fn(),
  getTaskWorkflow: vi.fn(),
  getQueueWorkflow: vi.fn(),
  listAgentPools: vi.fn(),
  rebindAgentPool: vi.fn(),
  mailList: vi.fn(),
  listTaskEvents: vi.fn(),
  listTaskPrincipals: vi.fn(),
  listTaskQueues: vi.fn(),
  listTasks: vi.fn(),
  mailRead: vi.fn(),
  moveTask: vi.fn(),
  updateTask: vi.fn(),
  updateTaskQueue: vi.fn(),
}));
const toast = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn() }));
const daemonContext = vi.hoisted(() => ({ activeId: "" }));
const taskSocket = vi.hoisted(() => ({
  options: undefined as { onHint?: (event: { sequence: number }) => void } | undefined,
}));

// passes its own host there, so an assertion that names a host (null = local)
// proves the request went through that host's client.
const hostArgument = vi.hoisted((): Record<string, number> => ({
  addTaskComment: 2,
  addTaskRelation: 4,
  createTask: 1,
  createTaskQueue: 1,
  deleteTaskRelation: 3,
  advanceTask: 4,
  getTask: 1,
  getTaskWorkflow: 1,
  getQueueWorkflow: 1,
  listAgentPools: 1,
  rebindAgentPool: 5,
  mailList: 2,
  listTaskEvents: 3,
  listTaskPrincipals: 0,
  listTaskQueues: 0,
  listTasks: 1,
  mailRead: 1,
  moveTask: 2,
  updateTask: 2,
  updateTaskQueue: 2,
}));
vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  return {
    ...actual,
    createApi: (target: Parameters<typeof actual.createApi>[0]) => {
      const client = actual.createApi(target);
      const tasks: Record<string, unknown> = { ...client.tasks };
      const workflows: Record<string, unknown> = { ...client.workflows };
      const mail: Record<string, unknown> = { ...client.mail };
      for (const [name, at] of Object.entries(hostArgument)) {
        // mailList and mailRead stand for mail.list and mail.read.
        const inMail = name.startsWith("mail");
        const domain = inMail ? mail : name in tasks ? tasks : workflows;
        domain[inMail ? name.slice(4).toLowerCase() : name] = (...args: unknown[]) => {
          args.splice(at, 0, client.target);
          return (api as Record<string, (...values: unknown[]) => unknown>)[name](...args);
        };
      }
      return { ...client, tasks, workflows, mail };
    },
  };
});
vi.mock("@/hooks/useTasksSocket", () => ({
  useTasksSocket: vi.fn((options) => {
    taskSocket.options = options;
    return "open";
  }),
}));
vi.mock("sonner", () => ({ toast }));
vi.mock("@/components/daemonContext", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/components/daemonContext")>()),
  useOptionalDaemons: () => ({ activeId: daemonContext.activeId }),
}));

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  sessionStorage.clear();
  setActiveDaemon(null);
  daemonContext.activeId = "";
  taskSocket.options = undefined;
  mockWorkspaceDefaults(api);
});

describe("TasksWorkspace", () => {
  it("edits explicit pools in the queue dialog", async () => {
    api.listAgentPools.mockResolvedValue({
      items: [
        {
          id: 4,
          queue: "TEST",
          name: "developers",
          agents: ["dev-a"],
          revision: 2,
          created_at: root.created_at,
          updated_at: root.updated_at,
        },
      ],
      count: 1,
    });
    api.rebindAgentPool.mockResolvedValue({
      id: 4,
      queue: "TEST",
      name: "developers",
      agents: ["dev-a", "dev-b"],
      revision: 3,
      created_at: root.created_at,
      updated_at: root.updated_at,
    });

    render(<TasksWorkspace />);
    await openManageQueues();
    expect(await screen.findByRole("dialog", { name: "Queues" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("tab", { name: "Pools" }));
    const developers = await screen.findByRole("group", { name: "Pool developers" });

    fireEvent.change(within(developers).getByLabelText("Add agent to developers"), {
      target: { value: "dev-b" },
    });
    await userEvent.click(within(developers).getByRole("button", { name: "Add" }));
    await userEvent.click(within(developers).getByRole("button", { name: "Save pool developers" }));
    expect(api.rebindAgentPool).toHaveBeenCalledWith(
      "TEST",
      "developers",
      ["dev-a", "dev-b"],
      2,
      expect.any(String),
      null,
    );
  });

  it("retries a queue whose pools failed to load and refreshes revisions after a conflict", async () => {
    api.listAgentPools
      .mockRejectedValueOnce(Object.assign(new Error("permission denied"), { status: 403 }))
      .mockResolvedValueOnce({
        items: [
          {
            id: 4,
            queue: "TEST",
            name: "developers",
            agents: ["dev-a"],
            revision: 6,
            created_at: root.created_at,
            updated_at: root.updated_at,
          },
        ],
        count: 1,
      })
      .mockResolvedValueOnce({
        items: [
          {
            id: 4,
            queue: "TEST",
            name: "developers",
            agents: ["dev-a"],
            revision: 7,
            created_at: root.created_at,
            updated_at: root.updated_at,
          },
        ],
        count: 1,
      });

    render(<TasksWorkspace />);
    await openManageQueues();
    expect(await screen.findByRole("alert", { name: "Queue TEST error" })).toHaveTextContent(
      "permission denied",
    );
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));
    await userEvent.click(screen.getByRole("tab", { name: "Pools" }));
    let developers = await screen.findByRole("group", { name: "Pool developers" });

    api.rebindAgentPool
      .mockRejectedValueOnce(
        new ApiError(409, "revision_conflict", "agent pool was changed by another actor"),
      )
      .mockResolvedValueOnce({
        id: 4,
        queue: "TEST",
        name: "developers",
        agents: ["dev-a", "dev-b"],
        revision: 8,
        created_at: root.created_at,
        updated_at: root.updated_at,
      });
    fireEvent.change(within(developers).getByLabelText("Add agent to developers"), {
      target: { value: "dev-b" },
    });
    await userEvent.click(within(developers).getByRole("button", { name: "Add" }));
    await userEvent.click(within(developers).getByRole("button", { name: "Save pool developers" }));
    expect(await screen.findByText(/Configuration changed elsewhere/)).toBeInTheDocument();
    expect(api.listAgentPools).toHaveBeenCalledTimes(3);
    developers = await screen.findByRole("group", { name: "Pool developers" });
    fireEvent.change(within(developers).getByLabelText("Add agent to developers"), {
      target: { value: "dev-b" },
    });
    await userEvent.click(within(developers).getByRole("button", { name: "Add" }));
    await userEvent.click(within(developers).getByRole("button", { name: "Save pool developers" }));
    expect(api.rebindAgentPool).toHaveBeenLastCalledWith(
      "TEST",
      "developers",
      ["dev-a", "dev-b"],
      7,
      expect.any(String),
      null,
    );
  });

  it("names a dependency by its title and status, and the parent only once", async () => {
    render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    const row = (await screen.findByText("Child task")).closest("div")!;
    expect(within(row).getByText("TEST-2")).toBeInTheDocument();
    expect(within(row).getByText("In progress")).toBeInTheDocument();
  });

  it("labels nested in-progress tasks visibly and accessibly without changing open rows", async () => {
    api.listTasks.mockResolvedValue({
      tasks: [
        { ...root, status: "open", category: "open" },
        { ...child, status: "in_progress", category: "in_progress" },
      ],
      sequence: 10,
    });

    render(<TasksWorkspace />);

    const parentRow = await screen.findByTestId("task-row-TEST-1");
    await userEvent.click(within(parentRow).getByRole("button", { name: "Expand TEST-1" }));
    const childRow = await screen.findByTestId("task-row-TEST-2");
    // The status is its own column now, not a badge inside the row's button:
    // visible text, and the tone that carries the meaning.
    const badge = within(childRow).getByText("In progress");
    expect(badge).toBeVisible();
    expect(badge).toHaveAttribute("data-tone", "live");
    expect(within(parentRow).queryByText("In progress")).toBeNull();
  });

  it("defaults to Active and accepts only the latest status-view tree", async () => {
    const closedPage = deferred<{ tasks: Task[]; sequence: number }>();
    const allPage = deferred<{ tasks: Task[]; sequence: number }>();
    api.listTasks
      .mockResolvedValueOnce({ tasks: [root, child], sequence: 10 })
      .mockReturnValueOnce(closedPage.promise)
      .mockReturnValueOnce(allPage.promise);

    render(<TasksWorkspace />);

    await screen.findByText("Ship native tasks");
    expect(api.listTasks).toHaveBeenLastCalledWith(
      expect.objectContaining({ status_view: "active" }),
      null,
      expect.any(AbortSignal),
    );

    await userEvent.click(
      within(screen.getByRole("radiogroup", { name: "Task status" })).getByRole("radio", {
        name: "Done",
      }),
    );
    await waitFor(() =>
      expect(api.listTasks).toHaveBeenLastCalledWith(
        expect.objectContaining({ status_view: "closed" }),
        null,
        expect.any(AbortSignal),
      ),
    );
    await userEvent.click(
      within(screen.getByRole("radiogroup", { name: "Task status" })).getByRole("radio", {
        name: "All",
      }),
    );
    await waitFor(() =>
      expect(api.listTasks).toHaveBeenLastCalledWith(
        expect.objectContaining({ status_view: "all" }),
        null,
        expect.any(AbortSignal),
      ),
    );

    await act(async () => closedPage.resolve({ tasks: [], sequence: 11 }));
    expect(screen.getByText("Ship native tasks")).toBeInTheDocument();
    await act(async () =>
      allPage.resolve({
        tasks: [{ ...root, title: "Completed task", status: "done", category: "done" }],
        sequence: 12,
      }),
    );
    expect(await screen.findByText("Completed task")).toBeInTheDocument();
    expect(screen.queryByText("Ship native tasks")).toBeNull();

    api.listTasks.mockResolvedValueOnce({
      tasks: [{ ...root, title: "Completed task", status: "done", category: "done" }],
      sequence: 13,
    });
    const onHint = taskSocket.options?.onHint as (hint: {
      sequence: number;
      kind: string;
      task_key: string;
    }) => void;
    act(() => onHint({ sequence: 13, kind: "task.updated", task_key: root.key }));
    await waitFor(() =>
      expect(api.listTasks).toHaveBeenLastCalledWith(
        expect.objectContaining({ status_view: "all" }),
        null,
        expect.any(AbortSignal),
      ),
    );
  });

  it("renders priority accessibly and saves edits through the task update", async () => {
    render(<TasksWorkspace />);

    expect(await screen.findByLabelText("P0 Critical")).toHaveTextContent("P0");
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    const priority = await screen.findByLabelText("Priority");
    expect(priority).toHaveValue("P0");

    await userEvent.selectOptions(priority, "P1");
    await userEvent.click(screen.getByRole("button", { name: "Save task" }));

    expect(api.updateTask).toHaveBeenCalledWith(
      "TEST-1",
      expect.objectContaining({ priority: "P1", revision: 2 }),
      null,
    );
  });

  it("saves wait-customer and set/clear pull request edits with loaded revisions on the explicit host", async () => {
    const target = {
      id: "remote",
      label: "Remote",
      baseURL: "https://remote.test",
      token: "secret",
    };
    api.updateTask.mockImplementation((_key: string, input: Partial<Task>) =>
      Promise.resolve({
        ...root,
        ...input,
        category: input.status ?? root.category,
        revision: Number(input.revision) + 1,
      }),
    );
    render(onHost(target, <TasksWorkspace />));

    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    await userEvent.selectOptions(await screen.findByLabelText("Status"), "wait_customer");
    // One paste event: typing re-renders the whole workspace once per key.
    await userEvent.click(screen.getByLabelText("Pull request"));
    await userEvent.paste("https://example.test/pull/7");
    await userEvent.click(screen.getByRole("button", { name: "Save task" }));

    expect(api.updateTask).toHaveBeenNthCalledWith(
      1,
      "TEST-1",
      expect.objectContaining({
        status: "wait_customer",
        pull_request: "https://example.test/pull/7",
        revision: 2,
      }),
      target,
    );
    const pullRequest = await screen.findByLabelText("Pull request");
    expect(pullRequest).toHaveValue("https://example.test/pull/7");
    await userEvent.clear(pullRequest);
    await userEvent.click(screen.getByRole("button", { name: "Save task" }));

    expect(api.updateTask).toHaveBeenNthCalledWith(
      2,
      "TEST-1",
      expect.objectContaining({
        status: "wait_customer",
        pull_request: "",
        revision: 3,
      }),
      target,
    );
  });

  it("renders the shared tree and opens task detail", async () => {
    render(<TasksWorkspace scopeAgent="worker" />);

    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Filters" })).toBeInTheDocument();
    expect(screen.queryByText("Desktop tree")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Expand TEST-1" }));
    expect(screen.getByText("Desktop tree")).toBeInTheDocument();

    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    expect(await screen.findByRole("heading", { name: "TEST-1" })).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Description" })).toHaveTextContent(
      "Central work system",
    );
    expect(screen.getByText("Starting now")).toBeInTheDocument();
  });

  it("highlights the row of the agent's Goal task only while it is in the list", async () => {
    const { rerender } = render(<TasksWorkspace scopeAgent="worker" goalTaskKey="TEST-2" />);

    const rootRow = await screen.findByTestId("task-row-TEST-1");
    expect(rootRow).not.toHaveAttribute("data-goal");
    await userEvent.click(screen.getByRole("button", { name: "Expand TEST-1" }));
    expect(screen.getByTestId("task-row-TEST-2")).toHaveAttribute("data-goal", "true");
    expect(screen.getByTestId("task-row-TEST-2")).toHaveAttribute(
      "title",
      "Current Goal of this agent",
    );
    expect(rootRow).not.toHaveAttribute("data-goal");

    rerender(<TasksWorkspace scopeAgent="worker" />);
    expect(screen.getByTestId("task-row-TEST-2")).not.toHaveAttribute("data-goal");
  });

  it("creates tasks and children inline in the selected queue", async () => {
    render(<TasksWorkspace />);
    await screen.findByText("Ship native tasks");

    await userEvent.click(screen.getByRole("button", { name: "New task" }));
    fireEvent.change(screen.getByLabelText("Task title"), { target: { value: "New root" } });
    await userEvent.click(screen.getByRole("button", { name: "Create task" }));
    expect(api.createTask).toHaveBeenCalledWith(
      expect.objectContaining({ queue: "TEST", title: "New root", parent_key: "" }),
      null,
    );

    await userEvent.click(await screen.findByRole("button", { name: "Close task detail" }));
    await userEvent.click(screen.getByRole("button", { name: "Add child to TEST-1" }));
    fireEvent.change(screen.getByLabelText("Task title"), { target: { value: "Child work" } });
    await userEvent.click(screen.getByRole("button", { name: "Create task" }));
    expect(api.createTask).toHaveBeenLastCalledWith(
      expect.objectContaining({ parent_key: "TEST-1", title: "Child work" }),
      null,
    );
  });

  it("preselects the queue of the last created root task, per server", async () => {
    const ops = { ...queue, prefix: "OPS", name: "Operations" };
    api.listTaskQueues.mockResolvedValue({ queues: [queue, ops], count: 2 });
    localStorage.setItem(
      "tasks:new-task-queue:v1",
      JSON.stringify({ local: "OPS", remote: "GONE" }),
    );
    const first = render(<TasksWorkspace />);
    await screen.findByText("Ship native tasks");
    await userEvent.click(screen.getByRole("button", { name: "New task" }));
    expect(screen.getByLabelText("Task queue")).toHaveValue("OPS");
    fireEvent.change(screen.getByLabelText("Task queue"), { target: { value: "TEST" } });
    fireEvent.change(screen.getByLabelText("Task title"), { target: { value: "New root" } });
    await userEvent.click(screen.getByRole("button", { name: "Create task" }));
    expect(api.createTask).toHaveBeenCalledWith(expect.objectContaining({ queue: "TEST" }), null);
    await waitFor(() =>
      expect(JSON.parse(localStorage.getItem("tasks:new-task-queue:v1") ?? "{}")).toEqual({
        local: "TEST",
        remote: "GONE",
      }),
    );
    first.unmount();

    const target = {
      id: "remote",
      label: "Remote",
      baseURL: "https://remote.example",
      token: "test",
    };
    render(onHost(target, <TasksWorkspace />));
    await screen.findByText("Ship native tasks");
    await userEvent.click(screen.getByRole("button", { name: "New task" }));
    // The remembered queue no longer exists on this server.
    expect(screen.getByLabelText("Task queue")).toHaveValue("TEST");
  });

  it("keys the remembered queue by the active server and lets the toolbar filter win", async () => {
    const ops = { ...queue, prefix: "OPS", name: "Operations" };
    api.listTaskQueues.mockResolvedValue({ queues: [queue, ops], count: 2 });
    const active = {
      id: "mac-mini",
      label: "Mac mini",
      baseURL: "https://mac-mini.test",
      token: "",
    };
    localStorage.setItem(
      "tasks:new-task-queue:v1",
      JSON.stringify({ local: "TEST", "mac-mini": "OPS" }),
    );
    sessionStorage.setItem("tasks:queue-filter:v1", "TEST");
    const filtered = render(onHost(active, <TasksWorkspace />));
    await screen.findByText("Ship native tasks");
    await userEvent.click(screen.getByRole("button", { name: "New task" }));
    expect(screen.getByLabelText("Task queue")).toHaveValue("TEST");
    filtered.unmount();

    sessionStorage.clear();
    render(onHost(active, <TasksWorkspace />));
    await screen.findByText("Ship native tasks");
    await userEvent.click(screen.getByRole("button", { name: "New task" }));
    expect(screen.getByLabelText("Task queue")).toHaveValue("OPS");
  });

  it("remembers the queue per agent in the agent's Tasks tab", async () => {
    const ops = { ...queue, prefix: "OPS", name: "Operations" };
    api.listTaskQueues.mockResolvedValue({ queues: [queue, ops], count: 2 });
    const createIn = async (agent: string, prefix: string) => {
      const view = render(<TasksWorkspace scopeAgent={agent} />);
      await screen.findByText("Ship native tasks");
      await userEvent.click(screen.getByRole("button", { name: "New task" }));
      fireEvent.change(screen.getByLabelText("Task queue"), { target: { value: prefix } });
      fireEvent.change(screen.getByLabelText("Task title"), { target: { value: "New root" } });
      await userEvent.click(screen.getByRole("button", { name: "Create task" }));
      await waitFor(() =>
        expect(api.createTask).toHaveBeenLastCalledWith(
          expect.objectContaining({ queue: prefix }),
          null,
        ),
      );
      view.unmount();
    };
    await createIn("alpha", "OPS");
    await createIn("beta", "TEST");

    const alpha = render(<TasksWorkspace scopeAgent="alpha" />);
    await screen.findByText("Ship native tasks");
    await userEvent.click(screen.getByRole("button", { name: "New task" }));
    expect(screen.getByLabelText("Task queue")).toHaveValue("OPS");
    alpha.unmount();

    sessionStorage.setItem("tasks:queue-filter:v1", "TEST");
    render(<TasksWorkspace scopeAgent="alpha" />);
    await screen.findByText("Ship native tasks");
    await userEvent.click(screen.getByRole("button", { name: "New task" }));
    expect(screen.getByLabelText("Task queue")).toHaveValue("TEST");
  });

  it("falls back to the server's queue when the agent's queue is gone", async () => {
    const ops = { ...queue, prefix: "OPS", name: "Operations" };
    api.listTaskQueues.mockResolvedValue({ queues: [queue, ops], count: 2 });
    localStorage.setItem("tasks:new-task-queue:v1", JSON.stringify({ local: "OPS" }));
    localStorage.setItem(
      "tasks:new-task-queue-by-agent:v1",
      JSON.stringify({ local: { alpha: "GONE" } }),
    );
    render(<TasksWorkspace scopeAgent="alpha" />);
    await screen.findByText("Ship native tasks");
    await userEvent.click(screen.getByRole("button", { name: "New task" }));
    expect(screen.getByLabelText("Task queue")).toHaveValue("OPS");
  });

  it("does not remember the queue of a child task or a failed create", async () => {
    render(<TasksWorkspace />);
    await screen.findByText("Ship native tasks");
    await userEvent.click(screen.getByRole("button", { name: "Add child to TEST-1" }));
    fireEvent.change(screen.getByLabelText("Task title"), { target: { value: "Child work" } });
    await userEvent.click(screen.getByRole("button", { name: "Create task" }));
    await waitFor(() => expect(api.createTask).toHaveBeenCalled());
    await userEvent.click(await screen.findByRole("button", { name: "Close task detail" }));

    api.createTask.mockRejectedValueOnce(new Error("nope"));
    await userEvent.click(screen.getByRole("button", { name: "New task" }));
    fireEvent.change(screen.getByLabelText("Task title"), { target: { value: "New root" } });
    await userEvent.click(screen.getByRole("button", { name: "Create task" }));
    await waitFor(() => expect(api.createTask).toHaveBeenCalledTimes(2));
    expect(localStorage.getItem("tasks:new-task-queue:v1")).toBeNull();
  });

  it("shows task metadata, dependencies, and history and edits relations", async () => {
    render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    await screen.findByRole("heading", { name: "TEST-1" });

    expect(screen.getAllByText("user:owner", { selector: "dd" })).toHaveLength(2);
    expect(screen.getByText("TEST", { selector: "dd" })).toBeInTheDocument();
    const dependencies = screen.getByText("Dependencies").closest("section")!;
    expect(within(dependencies).getByText("blocks")).toBeInTheDocument();
    expect(within(dependencies).getByText("TEST-2")).toBeInTheDocument();
    expect(screen.getByText("task.updated")).toBeInTheDocument();

    await userEvent.selectOptions(screen.getByLabelText("Relation type"), "related");
    fireEvent.change(screen.getByLabelText("Related task key"), { target: { value: "TEST-2" } });
    expect(screen.getByRole("button", { name: "Add relation" })).toBeEnabled();
    await userEvent.click(screen.getByRole("button", { name: "Add relation" }));
    await waitFor(() =>
      expect(api.addTaskRelation).toHaveBeenCalledWith(
        "TEST-1",
        "TEST-2",
        "related",
        2,
        null,
        expect.any(String),
      ),
    );

    await userEvent.click(screen.getByRole("button", { name: "Remove relation to TEST-2" }));
    expect(api.deleteTaskRelation).toHaveBeenCalledWith("TEST-1", 9, 2, null, expect.any(String));
  });

  it("filters waiting work by the customer principal", async () => {
    render(<TasksWorkspace />);
    await screen.findByText("Ship native tasks");

    await toggleWaitingForMe();
    await waitFor(() =>
      expect(api.listTasks).toHaveBeenLastCalledWith(
        expect.objectContaining({ waiting_for: "user:owner" }),
        null,
        expect.any(AbortSignal),
      ),
    );
  });

  it("creates and updates queues", async () => {
    render(<TasksWorkspace />);
    await screen.findByText("Ship native tasks");

    await openManageQueues();
    await userEvent.click(await screen.findByRole("button", { name: "New queue" }));
    fireEvent.change(screen.getByLabelText("Queue prefix"), { target: { value: "OPS" } });
    fireEvent.change(screen.getByLabelText("New queue name"), { target: { value: "Operations" } });
    await userEvent.click(screen.getByRole("button", { name: "Add queue" }));
    expect(api.createTaskQueue).toHaveBeenCalledWith(
      expect.objectContaining({ prefix: "OPS", name: "Operations" }),
      null,
    );

    await userEvent.click(screen.getByRole("button", { name: /^Queue TEST/ }));
    fireEvent.change(screen.getByLabelText("Queue name TEST"), {
      target: { value: "Tests updated" },
    });
    await userEvent.click(screen.getByRole("button", { name: "Save TEST" }));
    expect(api.updateTaskQueue).toHaveBeenCalledWith(
      "TEST",
      expect.objectContaining({ name: "Tests updated", revision: 1 }),
      null,
    );
  });

  it("replaces the navigation rail with toolbar filters", async () => {
    render(<TasksWorkspace />);
    await screen.findByText("Ship native tasks");

    expect(document.querySelector(".tasks-navigation")).toBeNull();
    expect(screen.queryByRole("separator", { name: "Resize task navigation" })).toBeNull();

    expect(screen.getByRole("button", { name: "Filters" })).toBeInTheDocument();
    await pickTaskAssignee("My tasks");
    await waitFor(() =>
      expect(api.listTasks).toHaveBeenLastCalledWith(
        expect.objectContaining({ assignee: "user:owner" }),
        null,
        expect.any(AbortSignal),
      ),
    );
    expect(screen.getByRole("button", { name: "Filters, 1 on" })).toBeInTheDocument();

    // Waiting for me narrows the assignee's tasks further.
    await toggleWaitingForMe();
    await waitFor(() =>
      expect(api.listTasks).toHaveBeenLastCalledWith(
        expect.objectContaining({ assignee: "user:owner", waiting_for: "user:owner" }),
        null,
        expect.any(AbortSignal),
      ),
    );
    expect(screen.getByRole("button", { name: "Filters, 2 on" })).toBeInTheDocument();

    await toggleWaitingForMe();
    await pickTaskAssignee("Anyone");
    await waitFor(() => {
      const last = api.listTasks.mock.calls.at(-1)?.[0];
      expect(last.waiting_for).toBeUndefined();
      expect(last.assignee).toBeUndefined();
    });
    expect(screen.getByRole("button", { name: "Filters" })).toBeInTheDocument();
  });

  it("filters by one agent from the principals list", async () => {
    render(<TasksWorkspace />);
    await screen.findByText("Ship native tasks");

    await pickTaskAssignee("triager");
    await waitFor(() =>
      expect(api.listTasks).toHaveBeenLastCalledWith(
        expect.objectContaining({ assignee: "agent:triager" }),
        null,
        expect.any(AbortSignal),
      ),
    );
  });

  it("puts the host control at the start of the toolbar", async () => {
    render(<TasksWorkspace hostControl={<button type="button">Host: local</button>} />);
    await screen.findByText("Ship native tasks");
    const toolbar = screen.getByRole("toolbar", { name: "Task filters" });
    expect(toolbar.firstElementChild).toContainElement(
      within(toolbar).getByRole("button", { name: "Host: local" }),
    );
  });

  it("filters the tree by queue from the toolbar without narrowing the daemon request", async () => {
    const ops: Task = { ...root, key: "OPS-1", queue: "OPS", title: "Rotate credentials" };
    api.listTaskQueues.mockResolvedValue({
      queues: [queue, { ...queue, prefix: "OPS", name: "Operations" }],
      count: 2,
    });
    api.listTasks.mockResolvedValue({ tasks: [root, child, ops], sequence: 10 });
    render(<TasksWorkspace />);
    await screen.findByText("Ship native tasks");
    expect(screen.getByText("Rotate credentials")).toBeInTheDocument();

    await pickTaskQueue("TEST");

    expect(await screen.findByRole("button", { name: "Filters, 1 on" })).toBeInTheDocument();
    expect(screen.queryByText("Rotate credentials")).toBeNull();
    expect(screen.getByText("Ship native tasks")).toBeInTheDocument();
    for (const call of api.listTasks.mock.calls) expect(call[0].queue).toBeUndefined();
  });

  it("keeps the queue filter between the agent and server workspaces", async () => {
    const ops: Task = { ...root, key: "OPS-1", queue: "OPS", title: "Rotate credentials" };
    api.listTaskQueues.mockResolvedValue({
      queues: [queue, { ...queue, prefix: "OPS", name: "Operations" }],
      count: 2,
    });
    api.listTasks.mockResolvedValue({ tasks: [root, child, ops], sequence: 10 });

    const agent = render(<TasksWorkspace scopeAgent="worker" />);
    await screen.findByText("Ship native tasks");
    await pickTaskQueue("OPS");
    expect(await screen.findByRole("button", { name: "Filters, 1 on" })).toBeInTheDocument();
    agent.unmount();

    render(<TasksWorkspace />);
    expect(await screen.findByRole("button", { name: "Filters, 1 on" })).toBeInTheDocument();
    expect(await screen.findByText("Rotate credentials")).toBeInTheDocument();
    expect(screen.queryByText("Ship native tasks")).toBeNull();
  });
});

describe("priority in the list", () => {
  it("saves a priority picked on the row with its revision and shows the saved value", async () => {
    const target = {
      id: "remote",
      label: "Remote",
      baseURL: "https://remote.example",
      token: "test",
    };
    api.updateTask.mockResolvedValue({ ...root, priority: "P2", revision: 3 });
    render(onHost(target, <TasksWorkspace />));
    const row = await screen.findByTestId(`task-row-${root.key}`);

    await userEvent.click(within(row).getByRole("button", { name: "P0 Critical" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: /P2/ }));

    expect(api.updateTask).toHaveBeenCalledWith(root.key, { priority: "P2", revision: 2 }, target);
    await waitFor(() =>
      expect(
        within(screen.getByTestId(`task-row-${root.key}`)).getByRole("button", {
          name: "P2 Normal",
        }),
      ).toBeInTheDocument(),
    );
    expect(api.getTask).not.toHaveBeenCalled();
  });

  it("reloads the tree and reports a revision conflict", async () => {
    render(<TasksWorkspace />);
    const row = await screen.findByTestId(`task-row-${root.key}`);
    const listed = api.listTasks.mock.calls.length;
    api.updateTask.mockRejectedValueOnce(new ApiError(409, "conflict", "revision conflict"));

    await userEvent.click(within(row).getByRole("button", { name: "P0 Critical" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: /P3/ }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("revision conflict"));
    expect(api.listTasks.mock.calls.length).toBeGreaterThan(listed);
  });
});
