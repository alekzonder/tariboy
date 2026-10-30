import { act, fireEvent, render, screen, waitFor, within } from "@/test/queryRender";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { setActiveDaemon, type Letter, type MailList, type Task } from "@/lib/api";
import { taskKeys } from "./taskQueries";
import {
  child,
  deferred,
  detail,
  inbox,
  mockWorkspaceDefaults,
  notification,
  root,
} from "@/test/tasks/tasksWorkspace";
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
  it("does not restore unread notifications from a read started during mark-read", async () => {
    const pendingWrite = deferred<{ read: number; closed_notices: number }>();
    const staleInbox = deferred<MailList>();
    api.mailRead.mockReturnValue(pendingWrite.promise);
    const mounted = render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    await waitFor(() => expect(api.mailRead).toHaveBeenCalledTimes(1));
    api.mailList.mockReturnValueOnce(staleInbox.promise);
    await act(async () => {
      void mounted.client.invalidateQueries({ queryKey: taskKeys.notifications(null) });
    });
    api.mailList.mockResolvedValue(inbox([{ ...notification, read_at: "now" }]));
    await act(async () => pendingWrite.resolve({ read: 1, closed_notices: 0 }));
    await waitFor(() =>
      expect(
        mounted.client.getQueryData<Letter[]>(taskKeys.notifications(null))?.[0].read_at,
      ).toBeTruthy(),
    );
    await act(async () => staleInbox.resolve(inbox([notification])));
    expect(
      mounted.client.getQueryData<Letter[]>(taskKeys.notifications(null))?.[0].read_at,
    ).toBeTruthy();
    expect(api.mailRead).toHaveBeenCalledTimes(1);
  });

  it("shows one accessible question indicator only for task rows with active question notifications", async () => {
    api.mailList.mockResolvedValue(
      inbox([
        notification,
        { ...notification, id: "unrelated", data: { task_key: child.key }, kind: "task.assigned" },
        { ...notification, id: "read", data: { task_key: child.key }, read_at: root.updated_at },
      ]),
    );

    render(<TasksWorkspace />);

    const rootRow = await screen.findByTestId("task-row-TEST-1");
    expect(
      await within(rootRow).findByRole("img", { name: "Unread question notification for TEST-1" }),
    ).toBeVisible();
    await userEvent.click(within(rootRow).getByRole("button", { name: "Expand TEST-1" }));
    const childRow = await screen.findByTestId("task-row-TEST-2");
    expect(
      within(childRow).queryByRole("img", { name: "Unread question notification for TEST-2" }),
    ).toBeNull();
  });

  it("marks a task's question notification read when the task is opened, clearing its indicator", async () => {
    render(<TasksWorkspace />);

    const rootRow = await screen.findByTestId("task-row-TEST-1");
    expect(
      await within(rootRow).findByRole("img", { name: "Unread question notification for TEST-1" }),
    ).toBeVisible();
    await userEvent.click(within(rootRow).getByRole("button", { name: /Ship native tasks/ }));

    await waitFor(() => expect(api.mailRead).toHaveBeenCalledWith(["notification-1"], null));
    expect(
      within(await screen.findByTestId("task-row-TEST-1")).queryByRole("img", {
        name: "Unread question notification for TEST-1",
      }),
    ).toBeNull();
  });

  it("keeps a closed task with an unread question in Active until it is opened", async () => {
    const closed: Task = {
      ...root,
      key: "TEST-9",
      title: "Closed with a question",
      status: "done",
      category: "done",
    };
    api.getTask.mockImplementation(async (key: string) =>
      key === closed.key ? { ...detail, task: closed } : detail,
    );
    api.mailList.mockResolvedValue(
      inbox([
        notification,
        { ...notification, id: "closed-question", data: { task_key: closed.key } },
      ]),
    );

    render(<TasksWorkspace />);

    const closedRow = await screen.findByTestId("task-row-TEST-9");
    expect(
      await within(closedRow).findByRole("img", {
        name: "Unread question notification for TEST-9",
      }),
    ).toBeVisible();
    expect(api.getTask).toHaveBeenCalledWith("TEST-9", null, expect.any(AbortSignal));
    expect(api.getTask).not.toHaveBeenCalledWith("TEST-1", null);
    await userEvent.click(
      within(closedRow).getByRole("button", { name: /Closed with a question/ }),
    );
    await waitFor(() => expect(api.mailRead).toHaveBeenCalledWith(["closed-question"], null));
  });

  it("pulls in only the scoped agent's closed questions", async () => {
    const closed: Task = {
      ...root,
      key: "TEST-9",
      title: "Closed with a question",
      status: "done",
      category: "done",
    };
    api.getTask.mockImplementation(async (key: string) =>
      key === closed.key ? { ...detail, task: closed } : detail,
    );
    api.mailList.mockResolvedValue(
      inbox([
        {
          ...notification,
          id: "other-agent",
          data: { task_key: closed.key },
          from: "agent:triager",
        },
      ]),
    );

    render(<TasksWorkspace scopeAgent="worker" />);

    await screen.findByTestId("task-row-TEST-1");
    await act(async () => {});
    expect(screen.queryByTestId("task-row-TEST-9")).toBeNull();
    expect(api.getTask).not.toHaveBeenCalled();
  });

  it("has no notification inbox of its own: the task rows carry the questions", async () => {
    render(<TasksWorkspace />);

    await screen.findByTestId("task-row-TEST-1");
    expect(screen.queryByRole("button", { name: /Notifications/ })).toBeNull();
    expect(screen.queryByText("Need a product decision")).toBeNull();
  });

  it("updates task row question indicators when realtime refreshes the notification inbox", async () => {
    render(<TasksWorkspace />);

    const rootRow = await screen.findByTestId("task-row-TEST-1");
    expect(
      await within(rootRow).findByRole("img", { name: "Unread question notification for TEST-1" }),
    ).toBeVisible();
    api.mailList.mockResolvedValue(inbox([]));
    const onHint = taskSocket.options?.onHint as (hint: {
      sequence: number;
      kind: string;
      task_key: string;
    }) => void;
    act(() => onHint({ sequence: 11, kind: "task.question", task_key: root.key }));

    await waitFor(() =>
      expect(
        within(screen.getByTestId("task-row-TEST-1")).queryByRole("img", {
          name: "Unread question notification for TEST-1",
        }),
      ).toBeNull(),
    );
  });

  it("picks an assignee from the agents and posts questions as mentions", async () => {
    render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    await screen.findByRole("heading", { name: "TEST-1" });

    const assignee = screen.getByLabelText("Assignee");
    await userEvent.selectOptions(assignee, "agent:triager");
    await userEvent.click(screen.getByRole("button", { name: "Save task" }));
    expect(api.updateTask).toHaveBeenCalledWith(
      "TEST-1",
      expect.objectContaining({ assignee: "agent:triager", revision: 2 }),
      null,
    );

    await userEvent.selectOptions(screen.getByLabelText("Ask an agent"), "user:owner");
    await userEvent.click(
      within(screen.getByText("Comments").closest("section")!).getByRole("button", {
        name: "Markdown",
      }),
    );
    fireEvent.change(screen.getByLabelText("Comment"), { target: { value: "Which release?" } });
    await userEvent.click(screen.getByRole("button", { name: "Send comment" }));
    expect(api.addTaskComment).toHaveBeenCalledWith(
      "TEST-1",
      "@user:owner\n\nWhich release?",
      null,
      expect.any(String),
    );
  });

  it("defaults a comment question to the task assignee", async () => {
    render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    await screen.findByRole("heading", { name: "TEST-1" });

    await userEvent.click(
      within(screen.getByText("Comments").closest("section")!).getByRole("button", {
        name: "Markdown",
      }),
    );
    fireEvent.change(screen.getByLabelText("Comment"), { target: { value: "Can you verify?" } });
    await userEvent.click(screen.getByRole("button", { name: "Send comment" }));

    expect(api.addTaskComment).toHaveBeenCalledWith(
      "TEST-1",
      "@agent:worker\n\nCan you verify?",
      null,
      expect.any(String),
    );
  });

  it("sends Ok with the selected question principal", async () => {
    render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    await screen.findByRole("heading", { name: "TEST-1" });

    const comments = screen.getByText("Comments").closest("section")!;
    await userEvent.selectOptions(screen.getByLabelText("Ask an agent"), "user:owner");

    const sendOk = within(comments).getByRole("button", { name: "Reply Ok to the question" });
    const sendComment = within(comments).getByRole("button", { name: "Send comment" });
    expect(sendOk).toHaveAttribute("data-variant", "secondary");
    expect(
      sendOk.compareDocumentPosition(sendComment) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    await userEvent.click(sendOk);

    await waitFor(() =>
      expect(api.addTaskComment).toHaveBeenCalledWith(
        "TEST-1",
        "@user:owner\n\nOk",
        null,
        expect.any(String),
      ),
    );
    expect(screen.getByLabelText("Ask an agent")).toHaveValue("user:owner");
  });

  it("hides Reply Ok when the comment contains text", async () => {
    render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    await screen.findByRole("heading", { name: "TEST-1" });

    const comments = screen.getByText("Comments").closest("section")!;
    await userEvent.click(within(comments).getByRole("button", { name: "Markdown" }));
    fireEvent.change(screen.getByLabelText("Comment"), { target: { value: "Keep this draft" } });

    expect(
      within(comments).queryByRole("button", { name: "Reply Ok to the question" }),
    ).not.toBeInTheDocument();
  });

  it("updates and restores the default question when the assignee changes", async () => {
    render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    await screen.findByRole("heading", { name: "TEST-1" });

    const ask = screen.getByLabelText("Ask an agent");
    expect(ask).toHaveValue("worker");
    await userEvent.selectOptions(ask, "");
    await userEvent.click(
      within(screen.getByText("Comments").closest("section")!).getByRole("button", {
        name: "Markdown",
      }),
    );
    fireEvent.change(screen.getByLabelText("Comment"), { target: { value: "Can you verify?" } });
    expect(ask).toHaveValue("");

    const reassigned = { ...root, assignee: "agent:triager", revision: 3 };
    api.getTask.mockResolvedValue({ ...detail, task: reassigned });
    await userEvent.selectOptions(screen.getByLabelText("Assignee"), reassigned.assignee);
    await userEvent.click(screen.getByRole("button", { name: "Save task" }));
    expect(ask).toHaveValue("triager");

    await userEvent.selectOptions(ask, "user:owner");
    await userEvent.click(screen.getByRole("button", { name: "Send comment" }));
    await waitFor(() =>
      expect(api.addTaskComment).toHaveBeenCalledWith(
        "TEST-1",
        "@user:owner\n\nCan you verify?",
        null,
        expect.any(String),
      ),
    );
    expect(ask).toHaveValue("triager");
  });
});
