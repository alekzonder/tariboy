import { render, screen, waitFor, within } from "@/test/queryRender";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { HostAgents } from "@/lib/aggregate";
import { setActiveDaemon, type Task } from "@/lib/api";
import { inbox, mockWorkspaceDefaults, queue, root } from "@/test/tasks/tasksWorkspace";
import { pickTaskAssignee, pickTaskQueue } from "@/test/tasks/taskFilters";
import AllHostsTasks from "./AllHostsTasks";

const api = vi.hoisted(() => ({
  addTaskComment: vi.fn(),
  addTaskRelation: vi.fn(),
  createTask: vi.fn(),
  createTaskQueue: vi.fn(),
  deleteTaskRelation: vi.fn(),
  getTask: vi.fn(),
  getTaskWorkflow: vi.fn(),
  getQueueWorkflow: vi.fn(),
  listAgentPools: vi.fn(),
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

// Each mocked call receives the host of the client that sent it at this
// argument (null = local), so an assertion proves which host was asked.
const hostArgument = vi.hoisted((): Record<string, number> => ({
  addTaskComment: 2,
  addTaskRelation: 4,
  createTask: 1,
  createTaskQueue: 1,
  deleteTaskRelation: 3,
  getTask: 1,
  getTaskWorkflow: 1,
  getQueueWorkflow: 1,
  listAgentPools: 1,
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
const remote = vi.hoisted(() => ({
  id: "remote",
  label: "Remote",
  baseURL: "http://remote.test",
  token: "remote-token",
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
        const inMail = name.startsWith("mail");
        const domain = inMail ? mail : name in tasks ? tasks : workflows;
        domain[inMail ? name.slice(4).toLowerCase() : name] = (...args: unknown[]) => {
          args.splice(at, 0, client.target?.id ?? null);
          return (api as Record<string, (...values: unknown[]) => unknown>)[name](...args);
        };
      }
      return { ...client, tasks, workflows, mail };
    },
  };
});
vi.mock("@/lib/terminalsHost", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/terminalsHost")>()),
  targetFor: (id: string) => (id === "remote" ? remote : null),
}));
vi.mock("@/hooks/useTasksSocket", () => ({ useTasksSocket: vi.fn(() => "open") }));
vi.mock("sonner", () => ({ toast }));

const remoteTask: Task = {
  ...root,
  key: "OPS-1",
  queue: "OPS",
  title: "Rotate remote keys",
  assignee: "agent:deployer",
};

const hosts: HostAgents[] = [
  { host: { id: "", label: "Local" }, agents: [] },
  { host: { id: "remote", label: "Remote" }, agents: [] },
];

/** Answers by host: `local` for the local daemon, `remote` for the other. */
function byHost<T>(local: T, other: T) {
  return (...args: unknown[]) =>
    Promise.resolve(args.includes("remote") ? other : local) as Promise<T>;
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  sessionStorage.clear();
  setActiveDaemon(null);
  mockWorkspaceDefaults(api);
  api.listTasks.mockImplementation(
    byHost({ tasks: [root], sequence: 1 }, { tasks: [remoteTask], sequence: 1 }),
  );
  api.listTaskQueues.mockImplementation(
    byHost(
      { queues: [queue], count: 1 },
      { queues: [{ ...queue, prefix: "OPS", name: "Operations" }], count: 1 },
    ),
  );
  api.listTaskPrincipals.mockImplementation(
    byHost(
      { customer: "user:owner", agents: ["worker"], groups: [] },
      { customer: "user:ops", agents: ["deployer"], groups: [] },
    ),
  );
});

const group = (name: string) => screen.findByRole("region", { name });

describe("AllHostsTasks", () => {
  it("lists every host's tasks under the host's name", async () => {
    render(<AllHostsTasks hosts={hosts} />);

    expect(
      within(await group("Local")).getByRole("button", { name: /Ship native tasks/ }),
    ).toBeInTheDocument();
    expect(
      within(await group("Remote")).getByRole("button", { name: /Rotate remote keys/ }),
    ).toBeInTheDocument();
    expect(api.listTasks).toHaveBeenCalledWith(
      expect.objectContaining({ status_view: "active" }),
      null,
      expect.anything(),
    );
    expect(api.listTasks).toHaveBeenCalledWith(
      expect.objectContaining({ status_view: "active" }),
      "remote",
      expect.anything(),
    );
  });

  it("applies the search and the status to every host", async () => {
    render(<AllHostsTasks hosts={hosts} />);
    await group("Remote");

    await userEvent.type(screen.getByRole("searchbox", { name: "Search tasks" }), "keys");
    await userEvent.click(screen.getByRole("radio", { name: "Done" }));

    for (const host of [null, "remote"]) {
      await waitFor(() =>
        expect(api.listTasks).toHaveBeenCalledWith(
          expect.objectContaining({ text: "keys", status_view: "closed" }),
          host,
          expect.anything(),
        ),
      );
    }
  });

  it("offers every host's queues and agents and filters every host by them", async () => {
    render(<AllHostsTasks hosts={hosts} />);
    await group("Local");
    await group("Remote");

    await pickTaskQueue("OPS");
    await waitFor(() => expect(screen.queryByRole("region", { name: "Local" })).toBeNull());
    expect(within(await group("Remote")).getByText("Rotate remote keys")).toBeInTheDocument();

    await pickTaskAssignee("deployer");
    for (const host of [null, "remote"]) {
      await waitFor(() =>
        expect(api.listTasks).toHaveBeenCalledWith(
          expect.objectContaining({ assignee: "agent:deployer" }),
          host,
          expect.anything(),
        ),
      );
    }
  });

  it("says when no host has a task in the view", async () => {
    api.listTasks.mockResolvedValue({ tasks: [], sequence: 1 });
    // A task with an unread question stays listed, so the inbox is empty.
    api.mailList.mockResolvedValue(inbox([]));
    render(<AllHostsTasks hosts={hosts} />);
    expect(await screen.findByText("No tasks match this view.")).toBeInTheDocument();
  });

  it("opens a task on its own host", async () => {
    api.getTask.mockResolvedValue({
      task: remoteTask,
      comments: [],
      waiting_for: [],
      relations: [],
      descendants: 0,
      active_descendants: 0,
      children: [],
      hidden_children: 0,
    });
    render(<AllHostsTasks hosts={hosts} />);

    await userEvent.click(
      within(await group("Remote")).getByRole("button", { name: /Rotate remote keys/ }),
    );
    await waitFor(() =>
      expect(api.getTask).toHaveBeenCalledWith("OPS-1", "remote", expect.anything()),
    );
    expect(api.getTask).not.toHaveBeenCalledWith("OPS-1", null, expect.anything());
  });

  it("shows an unreachable host as unavailable without asking it", async () => {
    render(<AllHostsTasks hosts={[hosts[0], { ...hosts[1], error: "connection refused" }]} />);

    expect(within(await group("Remote")).getByText("unavailable")).toBeInTheDocument();
    expect(await group("Local")).toBeInTheDocument();
    expect(api.listTasks).not.toHaveBeenCalledWith(expect.anything(), "remote", expect.anything());
  });

  it("creates a new task on the picked host", async () => {
    api.createTask.mockResolvedValue({ ...remoteTask, key: "OPS-2" });
    render(<AllHostsTasks hosts={hosts} />);
    await group("Remote");

    await userEvent.click(screen.getByRole("button", { name: "New task" }));
    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Task host" }), "remote");
    await waitFor(() =>
      expect(screen.getByRole("combobox", { name: "Task queue" })).toHaveValue("OPS"),
    );
    await userEvent.type(screen.getByRole("textbox", { name: "Task title" }), "Renew certificates");
    await userEvent.click(screen.getByRole("button", { name: "Create task" }));

    await waitFor(() =>
      expect(api.createTask).toHaveBeenCalledWith(
        expect.objectContaining({ queue: "OPS", title: "Renew certificates" }),
        "remote",
      ),
    );
  });
});
