import { act, fireEvent, render, screen, waitFor, within } from "@/test/queryRender";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { setActiveDaemon, type Task, type TaskDetail } from "@/lib/api";
import {
  child,
  deferred,
  detail,
  mockWorkspaceDefaults,
  onHost,
  root,
} from "@/test/tasks/tasksWorkspace";
import { pickTaskAssignee, toggleWaitingForMe } from "@/test/tasks/taskFilters";
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
  it("opens the task named by an initial deep-link key after loading the workspace", async () => {
    const asked = {
      ...root,
      key: "ASK-7",
      title: "Choose the release date",
      description: "Customer needs a date",
    };
    const askedDetail = { ...detail, task: asked };
    api.listTasks.mockResolvedValue({ tasks: [asked], sequence: 10 });
    api.getTask.mockResolvedValue(askedDetail);

    render(<TasksWorkspace initialTaskKey="ASK-7" />);

    expect(await screen.findByRole("heading", { name: "ASK-7" })).toBeInTheDocument();
    expect(await screen.findByRole("textbox", { name: "Description" })).toHaveTextContent(
      "Customer needs a date",
    );
  });

  it("keeps a closed deep-linked task closed when the host target is re-created", async () => {
    const asked = { ...root, key: "ASK-7", title: "Choose the release date" };
    api.listTasks.mockResolvedValue({ tasks: [asked], sequence: 10 });
    api.getTask.mockResolvedValue({ ...detail, task: asked });
    const target = () => ({
      id: "remote",
      label: "Remote",
      baseURL: "https://remote.example",
      token: "test",
    });

    const view = render(onHost(target(), <TasksWorkspace initialTaskKey="ASK-7" />));
    expect(await screen.findByRole("heading", { name: "ASK-7" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Close task detail" }));
    const loads = api.listTaskEvents.mock.calls.length;

    view.rerender(onHost(target(), <TasksWorkspace initialTaskKey="ASK-7" />));
    await act(async () => {});

    expect(api.listTaskEvents).toHaveBeenCalledTimes(loads);
    expect(screen.queryByRole("heading", { name: "ASK-7" })).not.toBeInTheDocument();
  });

  it("keeps the list, the open task and its drafts while the host reconnects", async () => {
    const remote = (baseURL: string) => ({ id: "remote", label: "Remote", baseURL, token: "test" });
    const view = render(onHost(remote("http://127.0.0.1:41001"), <TasksWorkspace />));
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    await screen.findByRole("heading", { name: root.key });
    const comments = screen.getByText("Comments").closest("section")!;
    await userEvent.click(within(comments).getByRole("button", { name: "Markdown" }));
    fireEvent.change(await screen.findByLabelText("Comment"), {
      target: { value: "Unsent comment" },
    });
    const loads = api.listTasks.mock.calls.length;

    const offline = remote("");
    view.rerender(onHost(offline, <TasksWorkspace />));
    await act(async () => {});
    expect(api.listTasks.mock.calls.filter(([, target]) => target === offline)).toHaveLength(0);
    const back = remote("http://127.0.0.1:41002");
    view.rerender(onHost(back, <TasksWorkspace />));
    await waitFor(() =>
      expect(api.listTasks).toHaveBeenLastCalledWith(
        expect.anything(),
        back,
        expect.any(AbortSignal),
      ),
    );

    expect(api.listTasks.mock.calls.length).toBe(loads + 1);
    expect(screen.getByTestId(`task-row-${root.key}`)).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: root.key })).toBeInTheDocument();
    expect(screen.getByLabelText("Comment")).toHaveValue("Unsent comment");
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("updates the selected task when the deep-link key changes", async () => {
    const first = {
      ...root,
      key: "ASK-7",
      title: "First question",
      description: "First answer needed",
    };
    const second = {
      ...root,
      key: "ASK-8",
      title: "Second question",
      description: "Second answer needed",
    };
    api.listTasks.mockResolvedValue({ tasks: [first, second], sequence: 10 });
    api.getTask.mockImplementation(async (key: string) => ({
      ...detail,
      task: key === "ASK-8" ? second : first,
    }));

    const view = render(<TasksWorkspace initialTaskKey="ASK-7" />);
    expect(await screen.findByRole("heading", { name: "ASK-7" })).toBeInTheDocument();

    view.rerender(<TasksWorkspace initialTaskKey="ASK-8" />);

    expect(await screen.findByRole("heading", { name: "ASK-8" })).toBeInTheDocument();
    expect(await screen.findByRole("textbox", { name: "Description" })).toHaveTextContent(
      "Second answer needed",
    );
  });

  it("keeps the latest task selected during a real-time refresh", async () => {
    const first = deferred<TaskDetail>();
    const second = deferred<TaskDetail>();
    const secondDetail = { ...detail, task: child };
    api.getTask.mockImplementation((key: string) =>
      key === "TEST-1" ? first.promise : Promise.resolve(secondDetail),
    );

    render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    first.resolve(detail);
    await screen.findByRole("heading", { name: "TEST-1" });
    api.getTask.mockImplementation((key: string) =>
      key === "TEST-2" ? second.promise : Promise.resolve(detail),
    );
    await userEvent.click(screen.getByRole("button", { name: "Close task detail" }));
    await userEvent.click(screen.getByRole("button", { name: "Expand TEST-1" }));
    await userEvent.click(screen.getByRole("button", { name: /Desktop tree/ }));
    await act(async () => taskSocket.options?.onHint?.({ sequence: 11 }));
    second.resolve(secondDetail);

    expect(await screen.findByRole("heading", { name: "TEST-2" })).toBeInTheDocument();
  });

  it("waits for the accepted initial sequence and coalesces a replay burst into one refresh", async () => {
    const firstPage = deferred<{ tasks: Task[]; sequence: number }>();
    const refreshPage = deferred<{ tasks: Task[]; sequence: number }>();
    api.listTasks.mockReturnValueOnce(firstPage.promise);

    render(<TasksWorkspace />);

    expect(taskSocket.options).toEqual(expect.objectContaining({ after: 0, enabled: false }));
    await act(async () => firstPage.resolve({ tasks: [root, child], sequence: 10 }));
    await screen.findByText("Ship native tasks");
    await waitFor(() =>
      expect(taskSocket.options).toEqual(expect.objectContaining({ after: 10, enabled: true })),
    );
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    const metadataCalls = api.listTaskQueues.mock.calls.length;
    const principalCalls = api.listTaskPrincipals.mock.calls.length;
    const notificationCalls = api.mailList.mock.calls.length;
    api.listTasks.mockReturnValueOnce(refreshPage.promise);

    const onHint = taskSocket.options?.onHint as (hint: {
      sequence: number;
      kind: string;
      task_key: string;
    }) => void;
    act(() => {
      for (let sequence = 11; sequence <= 110; sequence += 1) {
        onHint({ sequence, kind: "task.updated", task_key: root.key });
      }
    });

    await waitFor(() => expect(api.listTasks).toHaveBeenCalledTimes(2));
    await act(async () => refreshPage.resolve({ tasks: [root, child], sequence: 110 }));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(api.listTasks).toHaveBeenCalledTimes(2);
    expect(api.listTaskQueues).toHaveBeenCalledTimes(metadataCalls + 1);
    expect(api.listTaskPrincipals).toHaveBeenCalledTimes(principalCalls + 1);
    expect(api.mailList).toHaveBeenCalledTimes(notificationCalls + 1);
  });

  it("runs one trailing refresh when a hint arrives during an active refresh", async () => {
    const refresh = deferred<{ tasks: Task[]; sequence: number }>();
    render(<TasksWorkspace />);
    await screen.findByText("Ship native tasks");
    const onHint = taskSocket.options?.onHint as (hint: {
      sequence: number;
      kind: string;
      task_key: string;
    }) => void;
    api.listTasks.mockReturnValueOnce(refresh.promise);

    act(() => onHint({ sequence: 11, kind: "task.updated", task_key: root.key }));
    await waitFor(() => expect(api.listTasks).toHaveBeenCalledTimes(2));
    act(() => onHint({ sequence: 12, kind: "task.updated", task_key: root.key }));
    await act(async () =>
      refresh.resolve({ tasks: [{ ...root, title: "First refresh" }], sequence: 11 }),
    );

    await waitFor(() => expect(api.listTasks).toHaveBeenCalledTimes(3));
    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
  });

  it("shows the loading placeholder only until the initial task tree arrives", async () => {
    const firstPage = deferred<{ tasks: Task[]; sequence: number }>();
    api.listTasks.mockReturnValueOnce(firstPage.promise);

    render(<TasksWorkspace />);

    expect(await screen.findByText("Loading tasks…")).toBeInTheDocument();
    expect(screen.queryByText("Ship native tasks")).toBeNull();

    await act(async () => firstPage.resolve({ tasks: [root, child], sequence: 10 }));
    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
    expect(screen.queryByText("Loading tasks…")).toBeNull();
  });

  it("keeps the current tree visible and marks a background refresh busy", async () => {
    const refreshPage = deferred<{ tasks: Task[]; sequence: number }>();
    render(<TasksWorkspace />);
    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
    api.listTasks.mockReturnValueOnce(refreshPage.promise);

    await userEvent.click(screen.getByRole("button", { name: "Refresh tasks" }));

    expect(screen.getByText("Ship native tasks")).toBeInTheDocument();
    expect(screen.queryByText("Loading tasks…")).toBeNull();
    expect(screen.getByRole("button", { name: "Refresh tasks" })).toHaveAttribute(
      "aria-busy",
      "true",
    );

    await act(async () => refreshPage.resolve({ tasks: [root], sequence: 11 }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Refresh tasks" })).toHaveAttribute(
        "aria-busy",
        "false",
      ),
    );
  });

  it("retains the current tree when a background refresh fails", async () => {
    const refreshPage = deferred<{ tasks: Task[]; sequence: number }>();
    render(<TasksWorkspace />);
    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
    api.listTasks.mockReturnValueOnce(refreshPage.promise);

    await userEvent.click(screen.getByRole("button", { name: "Refresh tasks" }));
    await act(async () => refreshPage.reject(new Error("refresh failed")));

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Refresh tasks" })).toHaveAttribute(
        "aria-busy",
        "false",
      ),
    );
    expect(screen.getByText("Ship native tasks")).toBeInTheDocument();
    expect(screen.queryByText("Loading tasks…")).toBeNull();
    expect(toast.error).toHaveBeenCalledWith("refresh failed");
  });

  it("does not reload the initial tree when principal metadata arrives", async () => {
    const people = deferred<{ customer: string; agents: string[]; groups: string[] }>();
    api.listTaskPrincipals.mockReturnValueOnce(people.promise);

    render(<TasksWorkspace />);
    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
    expect(api.listTasks).toHaveBeenCalledTimes(1);

    await act(async () =>
      people.resolve({
        customer: "user:owner",
        agents: ["worker", "triager"],
        groups: ["platform"],
      }),
    );
    await waitFor(() => expect(api.listTaskPrincipals).toHaveBeenCalledTimes(1));
    expect(api.listTasks).toHaveBeenCalledTimes(1);
  });

  it("reloads a principal-filtered view when delayed customer metadata arrives", async () => {
    const people = deferred<{ customer: string; agents: string[]; groups: string[] }>();
    api.listTaskPrincipals.mockReturnValueOnce(people.promise);

    render(<TasksWorkspace />);
    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
    const callsBeforeFilter = api.listTasks.mock.calls.length;

    await pickTaskAssignee("My tasks");
    expect(await screen.findByText("Loading tasks…")).toBeInTheDocument();
    expect(api.listTasks).toHaveBeenCalledTimes(callsBeforeFilter);

    await act(async () =>
      people.resolve({
        customer: "user:owner",
        agents: ["worker", "triager"],
        groups: ["platform"],
      }),
    );

    await waitFor(() =>
      expect(api.listTasks).toHaveBeenLastCalledWith(
        expect.objectContaining({ assignee: "user:owner" }),
        null,
        expect.any(AbortSignal),
      ),
    );
  });

  it("does not accept an in-flight unfiltered result while waiting for principals", async () => {
    const allTasks = deferred<{ tasks: Task[]; sequence: number }>();
    const people = deferred<{ customer: string; agents: string[]; groups: string[] }>();
    api.listTasks.mockReturnValueOnce(allTasks.promise);
    api.listTaskPrincipals.mockReturnValueOnce(people.promise);

    render(<TasksWorkspace />);
    expect(await screen.findByText("Loading tasks…")).toBeInTheDocument();
    await pickTaskAssignee("My tasks");
    await act(async () => allTasks.resolve({ tasks: [root, child], sequence: 10 }));

    expect(screen.getByText("Loading tasks…")).toBeInTheDocument();
    expect(screen.queryByText("Ship native tasks")).toBeNull();
    await act(async () => people.resolve({ customer: "user:owner", agents: [], groups: [] }));
    await waitFor(() =>
      expect(api.listTasks).toHaveBeenLastCalledWith(
        expect.objectContaining({ assignee: "user:owner" }),
        null,
        expect.any(AbortSignal),
      ),
    );
    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
  });

  it("shows principal metadata errors and retries them from Refresh", async () => {
    const people = deferred<{ customer: string; agents: string[]; groups: string[] }>();
    api.listTaskPrincipals.mockReturnValueOnce(people.promise);

    render(<TasksWorkspace />);
    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
    await act(async () => people.reject(new Error("principals failed")));
    await pickTaskAssignee("My tasks");
    expect(await screen.findByText("principals failed")).toBeInTheDocument();

    api.listTaskPrincipals.mockResolvedValueOnce({
      customer: "user:owner",
      agents: [],
      groups: [],
    });
    await userEvent.click(screen.getByRole("button", { name: "Refresh tasks" }));

    await waitFor(() =>
      expect(api.listTasks).toHaveBeenLastCalledWith(
        expect.objectContaining({ assignee: "user:owner" }),
        null,
        expect.any(AbortSignal),
      ),
    );
    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
  });

  it("ignores an older metadata failure after a newer metadata load succeeds", async () => {
    const olderPeople = deferred<{ customer: string; agents: string[]; groups: string[] }>();
    const newerPeople = deferred<{ customer: string; agents: string[]; groups: string[] }>();
    api.listTaskPrincipals
      .mockReturnValueOnce(olderPeople.promise)
      .mockReturnValueOnce(newerPeople.promise);

    render(<TasksWorkspace />);
    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
    await pickTaskAssignee("My tasks");
    await userEvent.click(screen.getByRole("button", { name: "Refresh tasks" }));
    await act(async () => newerPeople.resolve({ customer: "user:new", agents: [], groups: [] }));
    await waitFor(() =>
      expect(api.listTasks).toHaveBeenLastCalledWith(
        expect.objectContaining({ assignee: "user:new" }),
        null,
        expect.any(AbortSignal),
      ),
    );

    await act(async () => olderPeople.reject(new Error("obsolete metadata failure")));

    expect(toast.error).not.toHaveBeenCalledWith("obsolete metadata failure");
    expect(screen.queryByText("obsolete metadata failure")).toBeNull();
  });

  it("ignores older metadata that succeeds after the latest customer", async () => {
    const olderPeople = deferred<{ customer: string; agents: string[]; groups: string[] }>();
    const newerPeople = deferred<{ customer: string; agents: string[]; groups: string[] }>();
    api.listTaskPrincipals
      .mockReturnValueOnce(olderPeople.promise)
      .mockReturnValueOnce(newerPeople.promise);

    render(<TasksWorkspace />);
    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
    await toggleWaitingForMe();
    await userEvent.click(screen.getByRole("button", { name: "Refresh tasks" }));
    await act(async () => newerPeople.resolve({ customer: "user:new", agents: [], groups: [] }));
    await waitFor(() =>
      expect(api.listTasks).toHaveBeenLastCalledWith(
        expect.objectContaining({ waiting_for: "user:new" }),
        null,
        expect.any(AbortSignal),
      ),
    );
    const callsAfterLatest = api.listTasks.mock.calls.length;

    await act(async () => olderPeople.resolve({ customer: "user:old", agents: [], groups: [] }));

    expect(api.listTasks).toHaveBeenCalledTimes(callsAfterLatest);
  });

  it("does not reload an unfiltered view when principal metadata fails", async () => {
    const people = deferred<{ customer: string; agents: string[]; groups: string[] }>();
    api.listTaskPrincipals.mockReturnValueOnce(people.promise);
    render(<TasksWorkspace />);
    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
    const callsBeforeFailure = api.listTasks.mock.calls.length;

    await act(async () => people.reject(new Error("principals failed")));

    expect(api.listTasks).toHaveBeenCalledTimes(callsBeforeFailure);
    expect(screen.getByText("Ship native tasks")).toBeInTheDocument();
  });

  it("resets the global workspace when the active daemon changes", async () => {
    const nextTree = deferred<{ tasks: Task[]; sequence: number }>();
    // The active-host boundary hands the workspace a client for the new host.
    const { rerender } = render(onHost(null, <TasksWorkspace />));
    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
    api.listTasks.mockReturnValueOnce(nextTree.promise);

    rerender(
      onHost(
        { id: "second-host", label: "Second", baseURL: "https://second.test", token: "" },
        <TasksWorkspace />,
      ),
    );

    expect(await screen.findByText("Loading tasks…")).toBeInTheDocument();
    expect(screen.queryByText("Ship native tasks")).toBeNull();
    await act(async () =>
      nextTree.resolve({ tasks: [{ ...root, title: "Second host tree" }], sequence: 1 }),
    );
    expect(await screen.findByText("Second host tree")).toBeInTheDocument();
  });

  it("reloads a principal-filtered view with the new target customer", async () => {
    const firstTarget = {
      id: "first",
      label: "First",
      baseURL: "http://first.test",
      token: "first",
    };
    const secondTarget = {
      id: "second",
      label: "Second",
      baseURL: "http://second.test",
      token: "second",
    };
    const secondPeople = deferred<{ customer: string; agents: string[]; groups: string[] }>();
    api.listTaskPrincipals
      .mockResolvedValueOnce({ customer: "user:first", agents: [], groups: [] })
      .mockReturnValueOnce(secondPeople.promise);

    const { rerender } = render(onHost(firstTarget, <TasksWorkspace />));
    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
    await toggleWaitingForMe();
    await waitFor(() =>
      expect(api.listTasks).toHaveBeenLastCalledWith(
        expect.objectContaining({ waiting_for: "user:first" }),
        firstTarget,
        expect.any(AbortSignal),
      ),
    );

    rerender(onHost(secondTarget, <TasksWorkspace />));
    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
    await toggleWaitingForMe();
    const secondTargetCalls = () =>
      api.listTasks.mock.calls.filter(([, callTarget]) => callTarget === secondTarget);
    expect(secondTargetCalls()).toHaveLength(1);
    await act(async () =>
      secondPeople.resolve({ customer: "user:second", agents: [], groups: [] }),
    );

    await waitFor(() =>
      expect(api.listTasks).toHaveBeenLastCalledWith(
        expect.objectContaining({ waiting_for: "user:second" }),
        secondTarget,
        expect.any(AbortSignal),
      ),
    );
  });

  it("ignores principal metadata that resolves after switching targets", async () => {
    const firstTarget = {
      id: "first",
      label: "First",
      baseURL: "http://first.test",
      token: "first",
    };
    const secondTarget = {
      id: "second",
      label: "Second",
      baseURL: "http://second.test",
      token: "second",
    };
    const firstPeople = deferred<{ customer: string; agents: string[]; groups: string[] }>();
    api.listTaskPrincipals
      .mockReturnValueOnce(firstPeople.promise)
      .mockResolvedValueOnce({ customer: "user:second", agents: [], groups: [] });

    const { rerender } = render(onHost(firstTarget, <TasksWorkspace />));
    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
    rerender(onHost(secondTarget, <TasksWorkspace />));
    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
    await pickTaskAssignee("My tasks");
    await waitFor(() =>
      expect(api.listTasks).toHaveBeenLastCalledWith(
        expect.objectContaining({ assignee: "user:second" }),
        secondTarget,
        expect.any(AbortSignal),
      ),
    );
    const callsAfterSecondCustomer = api.listTasks.mock.calls.length;

    await act(async () => firstPeople.resolve({ customer: "user:first", agents: [], groups: [] }));

    expect(api.listTasks).toHaveBeenCalledTimes(callsAfterSecondCustomer);
  });

  it("does not toast an obsolete refresh failure after a newer refresh succeeds", async () => {
    const olderRefresh = deferred<{ tasks: Task[]; sequence: number }>();
    const newerTask = { ...root, title: "Newest tree" };
    render(<TasksWorkspace />);
    expect(await screen.findByText("Ship native tasks")).toBeInTheDocument();
    api.listTasks
      .mockReturnValueOnce(olderRefresh.promise)
      .mockResolvedValueOnce({ tasks: [newerTask], sequence: 12 });

    await userEvent.click(screen.getByRole("button", { name: "Refresh tasks" }));
    await userEvent.click(screen.getByRole("button", { name: "Refresh tasks" }));
    expect(await screen.findByText("Newest tree")).toBeInTheDocument();
    await act(async () => olderRefresh.reject(new Error("obsolete failure")));

    expect(screen.getByText("Newest tree")).toBeInTheDocument();
    expect(toast.error).not.toHaveBeenCalledWith("obsolete failure");
  });
});
