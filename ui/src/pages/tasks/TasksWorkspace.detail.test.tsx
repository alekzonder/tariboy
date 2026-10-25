import { act, fireEvent, render, screen, waitFor, within } from "@/test/queryRender";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, setActiveDaemon, type Task, type TaskDetail } from "@/lib/api";
import { taskKeys } from "./taskQueries";
import { customerView } from "@/test/fixtures/workflowFixtures";
import {
  child,
  deferred,
  detail,
  mockWorkspaceDefaults,
  onHost,
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
  listTaskNotifications: vi.fn(),
  listTaskEvents: vi.fn(),
  listTaskPrincipals: vi.fn(),
  listTaskQueues: vi.fn(),
  listTasks: vi.fn(),
  markTaskNotificationRead: vi.fn(),
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
  listTaskNotifications: 1,
  listTaskEvents: 3,
  listTaskPrincipals: 0,
  listTaskQueues: 0,
  listTasks: 1,
  markTaskNotificationRead: 1,
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
      for (const [name, at] of Object.entries(hostArgument)) {
        const domain = name in tasks ? tasks : workflows;
        domain[name] = (...args: unknown[]) => {
          args.splice(at, 0, client.target);
          return (api as Record<string, (...values: unknown[]) => unknown>)[name](...args);
        };
      }
      return { ...client, tasks, workflows };
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
  it("refreshes the list row after a drawer action, without waiting for the socket", async () => {
    const workflowRoot: Task = {
      ...root,
      status: "approval",
      category: "wait_customer",
      waiting_on: "customer",
      workflow_digest: "abc",
      workflow_name: "release",
      workflow_version: "1.0.0",
    };
    const approved: Task = {
      ...workflowRoot,
      status: "done",
      category: "done",
      waiting_on: undefined,
      revision: 3,
    };
    api.listTasks.mockResolvedValue({ tasks: [workflowRoot, child], sequence: 10 });
    api.getTask.mockResolvedValue({ ...detail, task: workflowRoot });
    api.getTaskWorkflow.mockResolvedValue({
      ...customerView,
      status: "approval",
      outcomes: [{ on: "approved", to: "done" }],
      visits: [],
      last_request: undefined,
    });
    api.advanceTask.mockResolvedValue({
      id: 1,
      task_key: root.key,
      outcome: "approved",
      actor: "user:owner",
      state: "applied",
      created_at: "",
    });
    render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    const approve = await screen.findByRole("button", { name: "approved" });
    const listed = api.listTasks.mock.calls.length;
    const inboxReads = api.listTaskNotifications.mock.calls.length;
    const workflowReads = api.getTaskWorkflow.mock.calls.length;
    const detailReads = api.getTask.mock.calls.length;
    api.listTasks.mockResolvedValue({ tasks: [approved, child], sequence: 10 });
    api.getTask.mockResolvedValue({ ...detail, task: approved });
    await userEvent.click(approve);
    await waitFor(() => expect(api.listTasks.mock.calls.length).toBeGreaterThan(listed));
    // No socket hint was delivered: the drawer action alone refreshed the row.
    const row = screen.getByTestId(`task-row-${root.key}`);
    await waitFor(() => expect(row).toHaveTextContent("Done"));
    await act(async () => {});
    expect(api.listTasks).toHaveBeenCalledTimes(listed + 1);
    expect(api.listTaskNotifications).toHaveBeenCalledTimes(inboxReads + 1);
    expect(api.getTask).toHaveBeenCalledTimes(detailReads + 1);
    expect(api.getTaskWorkflow).toHaveBeenCalledTimes(workflowReads + 1);
  });

  it("uploads files without an agent to the task host and appends only to the selected draft", async () => {
    const target = {
      id: "remote",
      label: "Remote",
      baseURL: "https://remote.example",
      token: "test",
    };
    api.getTask.mockResolvedValue({ ...detail, task: { ...root, assignee: "" } });
    const upload = deferred<Response>();
    const fetchMock = vi.spyOn(globalThis, "fetch").mockReturnValue(upload.promise);
    render(onHost(target, <TasksWorkspace />));
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    const description = (await screen.findByLabelText("Description")).closest(".task-description")!;
    const comments = screen.getByText("Comments").closest("section")!;
    expect(
      within(description as HTMLElement).getByRole("button", { name: "Send files" }),
    ).toBeEnabled();
    expect(within(comments).getByRole("button", { name: "Send files" })).toBeEnabled();
    await userEvent.click(
      within(description as HTMLElement).getByRole("button", { name: "Markdown" }),
    );
    await userEvent.click(within(comments).getByRole("button", { name: "Markdown" }));
    fireEvent.change(screen.getByLabelText("Comment"), { target: { value: "Comment draft" } });
    const file = new File(["hello"], "notes.txt");
    await userEvent.upload(
      description.querySelector('input[type="file"]') as HTMLInputElement,
      file,
    );
    fireEvent.change(screen.getByLabelText("Description"), { target: { value: "Still typing" } });
    expect(screen.getByRole("button", { name: "Save task" })).toBeDisabled();
    const envelope = {
      ok: true,
      result: { path: "files/one/notes.txt", abs: "/server/files/one/notes.txt", bytes: 5 },
    };
    await act(async () =>
      upload.resolve({
        ok: true,
        status: 200,
        text: async () => JSON.stringify(envelope),
        json: async () => envelope,
      } as Response),
    );
    await waitFor(() =>
      expect(screen.getByLabelText("Description")).toHaveValue(
        "Still typing\n/server/files/one/notes.txt",
      ),
    );
    expect(screen.getByLabelText("Comment")).toHaveValue("Comment draft");
    expect(fetchMock).toHaveBeenCalledWith(
      "https://remote.example/api/files/raw?name=notes.txt",
      expect.objectContaining({ method: "PUT", body: file }),
    );
    await userEvent.upload(
      comments.querySelector('input[type="file"]') as HTMLInputElement,
      new File(["hello"], "notes.txt"),
    );
    await waitFor(() =>
      expect(screen.getByLabelText("Comment")).toHaveValue(
        "Comment draft\n/server/files/one/notes.txt",
      ),
    );
    expect(screen.getByLabelText("Description")).toHaveValue(
      "Still typing\n/server/files/one/notes.txt",
    );
    expect(api.updateTask).not.toHaveBeenCalled();
    expect(api.addTaskComment).not.toHaveBeenCalled();
    fetchMock.mockRestore();
  });

  it("opens details only on selection and protects an unsaved title on outside close", async () => {
    render(<TasksWorkspace />);
    const row = await screen.findByRole("button", { name: /Ship native tasks/ });
    expect(screen.queryByText("Select a task")).not.toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await userEvent.click(row);
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Close task detail" })).toHaveFocus();
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Unsaved title" } });
    await userEvent.click(document.querySelector('[data-slot="dialog-overlay"]') as HTMLElement);
    expect(screen.getByDisplayValue("Unsaved title")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Are you sure you want to close this task? Unsaved changes will be discarded.",
      ),
    ).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Keep editing" }));
    expect(screen.getByDisplayValue("Unsaved title")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Close task detail" }));
    await userEvent.click(screen.getByRole("button", { name: "Discard changes" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(row).toHaveFocus();
  });

  it("opens the drawer at once in a loading state and fills it when the task arrives", async () => {
    const pending = deferred<TaskDetail>();
    api.getTask.mockReturnValue(pending.promise);
    render(<TasksWorkspace />);
    const row = await screen.findByRole("button", { name: /Ship native tasks/ });
    await userEvent.click(row);
    const dialog = await screen.findByRole("dialog", { name: "TEST-1" });
    expect(within(dialog).getByText("Loading task…")).toBeInTheDocument();
    pending.resolve(detail);
    expect(await screen.findByLabelText("Title")).toHaveValue("Ship native tasks");
    expect(screen.queryByText("Loading task…")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Close task detail" })).toHaveFocus();
    await userEvent.click(screen.getByRole("button", { name: "Close task detail" }));
    expect(row).toHaveFocus();
  });

  it("closes a loading drawer and keeps it closed when the task arrives", async () => {
    const pending = deferred<TaskDetail>();
    api.getTask.mockReturnValue(pending.promise);
    render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    await screen.findByText("Loading task…");
    await userEvent.click(screen.getByRole("button", { name: "Close task detail" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await act(async () => pending.resolve(detail));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("shows the next task loading at once and refreshes the open one without a skeleton", async () => {
    render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    await screen.findByRole("heading", { name: "TEST-1" });
    const refresh = deferred<TaskDetail>();
    api.getTask.mockReturnValue(refresh.promise);
    await act(async () => taskSocket.options?.onHint?.({ sequence: 11 }));
    expect(screen.queryByText("Loading task…")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Title")).toBeInTheDocument();
    await act(async () => refresh.resolve(detail));
    const next = deferred<TaskDetail>();
    api.getTask.mockReturnValue(next.promise);
    await userEvent.click(screen.getByRole("button", { name: "Close task detail" }));
    await userEvent.click(screen.getByRole("button", { name: "Expand TEST-1" }));
    await userEvent.click(screen.getByRole("button", { name: /Desktop tree/ }));
    expect(await screen.findByRole("dialog", { name: "TEST-2" })).toHaveTextContent(
      "Loading task…",
    );
    next.resolve({ ...detail, task: child });
    expect(await screen.findByLabelText("Title")).toHaveValue(child.title);
  });

  it("closes a clean task when the backdrop is clicked", async () => {
    render(<TasksWorkspace />);
    const row = await screen.findByRole("button", { name: /Ship native tasks/ });
    await userEvent.click(row);

    await userEvent.click(document.querySelector('[data-slot="dialog-overlay"]') as HTMLElement);

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(row).toHaveFocus();
  });

  it("keeps a newer authoritative revision after saving an older mutation result", async () => {
    const mounted = render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Saved" } });
    api.updateTask.mockResolvedValue({ ...root, title: "Saved", revision: 3 });
    api.getTask.mockResolvedValue({ ...detail, task: { ...root, title: "Newer", revision: 4 } });
    api.listTasks.mockResolvedValue({
      tasks: [{ ...root, title: "Newer", revision: 4 }],
      sequence: 12,
    });
    await userEvent.click(screen.getByRole("button", { name: "Save task" }));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Task updated"));
    expect(
      mounted.client.getQueryData<TaskDetail>(taskKeys.detail(null, root.key))?.task.revision,
    ).toBe(4);
    const lists = mounted.client.getQueriesData<{ tasks: Task[] }>({
      queryKey: [...taskKeys.server(null), "list"],
    });
    expect(lists[0][1]?.tasks[0].revision).toBe(4);
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Next edit" } });
    await userEvent.click(screen.getByRole("button", { name: "Save task" }));
    expect(api.updateTask).toHaveBeenLastCalledWith(
      root.key,
      expect.objectContaining({ revision: 4 }),
      null,
    );
  });

  it("keeps unsaved fields through a remote revision and failed save", async () => {
    render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "My draft" } });
    api.getTask.mockResolvedValue({
      ...detail,
      task: { ...root, revision: 3, title: "Remote title" },
    });
    await act(async () => taskSocket.options?.onHint?.({ sequence: 11 }));
    await waitFor(() => expect(api.getTask).toHaveBeenCalledTimes(2));
    expect(screen.getByDisplayValue("My draft")).toBeInTheDocument();
    api.updateTask.mockRejectedValue(new Error("Save failed"));
    await userEvent.click(screen.getByRole("button", { name: "Save task" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Save failed"));
    expect(screen.getByDisplayValue("My draft")).toBeInTheDocument();
  });

  it("retains a draft when background detail refresh fails", async () => {
    render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Keep this draft" } });
    api.getTask.mockRejectedValueOnce(new Error("Refresh failed"));
    await act(async () => taskSocket.options?.onHint?.({ sequence: 12 }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Refresh failed"));
    expect(screen.getByDisplayValue("Keep this draft")).toBeInTheDocument();
  });

  it("recovers a conflict explicitly while preserving draft and unrelated remote fields", async () => {
    render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "My title" } });
    api.getTask.mockResolvedValue({
      ...detail,
      task: { ...root, title: "Remote title", priority: "P1", revision: 3 },
    });
    api.updateTask.mockRejectedValueOnce(new ApiError(409, "conflict", "revision conflict"));
    await userEvent.click(screen.getByRole("button", { name: "Save task" }));
    await userEvent.click(await screen.findByRole("button", { name: "Keep my edits" }));
    await userEvent.click(screen.getByRole("button", { name: "Save task" }));
    expect(api.updateTask).toHaveBeenLastCalledWith(
      root.key,
      expect.objectContaining({
        title: "My title",
        priority: "P1",
        revision: 3,
      }),
      null,
    );
  });

  it("opens the detail sheet at half width and restores its persisted resize", async () => {
    localStorage.setItem(
      "tasks:workspace:v1",
      JSON.stringify({
        schemaVersion: 1,
        navigationWidth: 280,
        detailWidth: 520,
      }),
    );

    render(<TasksWorkspace />);

    const workspace = await screen.findByTestId("tasks-workspace");
    expect(workspace.style.getPropertyValue("--tasks-detail-width")).toBe("");
    expect(
      screen.queryByRole("separator", { name: "Resize task details" }),
    ).not.toBeInTheDocument();

    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));

    expect(screen.getByRole("separator", { name: "Resize task details" })).toHaveAttribute(
      "aria-valuenow",
      "520",
    );
    expect(screen.getByRole("dialog").style.getPropertyValue("--tasks-detail-width")).toBe("520px");
  });

  it("uses half the viewport for the first detail sheet", async () => {
    vi.stubGlobal("innerWidth", 1200);
    render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));

    expect(screen.getByRole("separator", { name: "Resize task details" })).toHaveAttribute(
      "aria-valuenow",
      "600",
    );
    expect(screen.getByRole("dialog").style.getPropertyValue("--tasks-detail-width")).toBe("600px");
    vi.unstubAllGlobals();
  });

  it("resizes the detail sheet with the keyboard and keeps the legacy storage record readable", async () => {
    localStorage.setItem(
      "tasks:workspace:v1",
      JSON.stringify({
        schemaVersion: 1,
        navigationWidth: 280,
        detailWidth: 512,
      }),
    );
    render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));

    fireEvent.keyDown(screen.getByRole("separator", { name: "Resize task details" }), {
      key: "ArrowRight",
    });
    expect(JSON.parse(localStorage.getItem("tasks:workspace:v1") ?? "{}")).toEqual({
      schemaVersion: 1,
      detailWidth: 504,
    });

    fireEvent.keyDown(screen.getByRole("separator", { name: "Resize task details" }), {
      key: "Home",
    });
    expect(JSON.parse(localStorage.getItem("tasks:workspace:v1") ?? "{}")).toEqual({
      schemaVersion: 1,
      detailWidth: 512,
    });
  });

  it("drags the detail handle, stops at pointer-up, and resets on double-click", async () => {
    render(<TasksWorkspace />);
    const workspace = await screen.findByTestId("tasks-workspace");
    vi.spyOn(workspace, "getBoundingClientRect").mockReturnValue({
      x: 100,
      y: 0,
      left: 100,
      right: 1200,
      top: 0,
      bottom: 800,
      width: 1100,
      height: 800,
      toJSON: () => ({}),
    });
    fireEvent(window, new Event("resize"));
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    const handle = screen.getByRole("separator", { name: "Resize task details" });

    fireEvent.pointerDown(handle, { pointerId: 1, button: 0, isPrimary: true });
    fireEvent.pointerMove(window, { pointerId: 1, clientX: 420 });
    fireEvent.pointerUp(window, { pointerId: 1 });
    fireEvent.pointerMove(window, { pointerId: 1, clientX: 300 });
    expect(JSON.parse(localStorage.getItem("tasks:workspace:v1") ?? "{}")).toMatchObject({
      detailWidth: 780,
    });

    fireEvent.doubleClick(handle);
    expect(JSON.parse(localStorage.getItem("tasks:workspace:v1") ?? "{}")).toEqual({
      schemaVersion: 1,
      detailWidth: 512,
    });
  });

  it("cleans up an active resize on pointer cancellation and unmount", async () => {
    const view = render(<TasksWorkspace />);
    const workspace = await screen.findByTestId("tasks-workspace");
    vi.spyOn(workspace, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      left: 0,
      right: 1400,
      top: 0,
      bottom: 800,
      width: 1400,
      height: 800,
      toJSON: () => ({}),
    });
    fireEvent(window, new Event("resize"));
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    const handle = screen.getByRole("separator", { name: "Resize task details" });

    fireEvent.pointerDown(handle, { pointerId: 7, button: 0, isPrimary: true });
    fireEvent.pointerMove(window, { pointerId: 7, clientX: 1000 });
    fireEvent.pointerCancel(window, { pointerId: 7 });
    fireEvent.pointerMove(window, { pointerId: 7, clientX: 900 });
    expect(JSON.parse(localStorage.getItem("tasks:workspace:v1") ?? "{}")).toMatchObject({
      detailWidth: 400,
    });
    expect(document.body.style.cursor).toBe("");
    expect(document.body.style.userSelect).toBe("");

    fireEvent.pointerDown(handle, { pointerId: 8, button: 0, isPrimary: true });
    view.unmount();
    fireEvent.pointerMove(window, { pointerId: 8, clientX: 800 });
    expect(JSON.parse(localStorage.getItem("tasks:workspace:v1") ?? "{}")).toMatchObject({
      detailWidth: 400,
    });
    expect(document.body.style.cursor).toBe("");
    expect(document.body.style.userSelect).toBe("");
  });

  it("clamps the detail resize to the workspace width", async () => {
    render(<TasksWorkspace />);
    const workspace = await screen.findByTestId("tasks-workspace");
    vi.spyOn(workspace, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      left: 0,
      right: 1100,
      top: 0,
      bottom: 800,
      width: 1100,
      height: 800,
      toJSON: () => ({}),
    });
    fireEvent(window, new Event("resize"));
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    const handle = screen.getByRole("separator", { name: "Resize task details" });
    expect(handle).toHaveAttribute("aria-valuemax", "1020");

    fireEvent.pointerDown(handle, { pointerId: 9, button: 0, isPrimary: true });
    fireEvent.pointerMove(window, { pointerId: 9, clientX: -5000 });
    fireEvent.pointerUp(window, { pointerId: 9 });

    // The handle stops at the workspace, the stored width at the viewport.
    expect(JSON.parse(localStorage.getItem("tasks:workspace:v1") ?? "{}")).toMatchObject({
      detailWidth: 944,
    });
  });

  it("shows newest comments first and can switch to oldest first", async () => {
    const oldComment = { ...detail.comments[0], id: 1, body: "Oldest" };
    const newComment = {
      ...detail.comments[0],
      id: 2,
      body: "Newest",
      created_at: "2026-08-01T10:00:00Z",
    };
    api.getTask.mockResolvedValue({ ...detail, comments: [oldComment, newComment] });

    render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));

    const comments = screen.getByText("Comments").closest("section")!;
    const commentList = comments.querySelector(".task-comment-list")!;
    const commentForm = comments.querySelector("form")!;
    expect(within(comments).getAllByRole("article")[0]).toHaveTextContent("Newest");
    expect(commentForm.compareDocumentPosition(commentList)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
    await userEvent.selectOptions(screen.getByLabelText("Comment order"), "oldest");
    expect(within(comments).getAllByRole("article")[0]).toHaveTextContent("Oldest");
    expect(commentList.compareDocumentPosition(commentForm)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
  });

  it("copies the original comment Markdown", async () => {
    const markdown = "# Status\n\n- **ready**\n- `tested`";
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    api.getTask.mockResolvedValue({
      ...detail,
      comments: [{ ...detail.comments[0], body: markdown }],
    });

    render(<TasksWorkspace />);
    await userEvent.click(await screen.findByRole("button", { name: /Ship native tasks/ }));
    await userEvent.click(screen.getByRole("button", { name: "Copy comment Markdown" }));

    expect(writeText).toHaveBeenCalledWith(markdown);
    expect(toast.success).toHaveBeenCalledWith("Comment Markdown copied");
  });
});
