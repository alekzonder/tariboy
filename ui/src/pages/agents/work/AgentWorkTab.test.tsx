import { MemoryRouter } from "react-router-dom";
import type { ReactElement } from "react";
import { act, fireEvent, render as renderWith, screen, waitFor, within } from "@/test/queryRender";
import { beforeEach, expect, it, vi } from "vitest";
import type { Task, TaskFilters } from "@/lib/api";
import { fakeApi } from "@/test/api";
import { agentRow, taskItem } from "@/test/fixtures/apiFixtures";
import AgentWorkTab from "./AgentWorkTab";

vi.mock("@/pages/tasks/useTaskQueryEvents", () => ({ useTaskQueryEvents: vi.fn() }));
vi.mock("@/pages/tasks/NeedsYouCard", () => ({
  default: ({
    taskKey,
    focus,
    onOpen,
  }: {
    taskKey: string;
    focus?: boolean;
    onOpen?: () => void;
  }) => (
    <div data-testid="needs-card">
      <button type="button" onClick={onOpen}>
        {taskKey}
      </button>
      {focus ? " focused" : ""}
    </div>
  ),
}));
vi.mock("@/pages/tasks/TaskDrawer", () => ({
  default: ({ taskKey }: { taskKey: string }) => <div role="dialog">drawer {taskKey}</div>,
}));

const api = vi.hoisted(() => ({
  listTasks: vi.fn(),
  listTaskQueues: vi.fn(),
  createTask: vi.fn(),
  getQueueWorkflow: vi.fn(),
  advanceTask: vi.fn(),
  getTransitionRequest: vi.fn(),
  startAgent: vi.fn(),
  getTaskActivity: vi.fn(),
}));
const client = fakeApi({
  tasks: {
    listTasks: api.listTasks,
    listTaskQueues: api.listTaskQueues,
    createTask: api.createTask,
  },
  workflows: {
    getQueueWorkflow: api.getQueueWorkflow,
    advanceTask: api.advanceTask,
    getTransitionRequest: api.getTransitionRequest,
  },
  agents: { startAgent: api.startAgent },
  usage: { getTaskActivity: api.getTaskActivity },
});

const NOW = new Date();
const ago = (days: number) => new Date(NOW.getTime() - days * 86_400_000).toISOString();

let active: Task[] = [];
let closed: Task[] = [];
const bob = agentRow({ name: "bob", enabled: true, goal_enabled: true });

function render(ui: ReactElement, path = "/agents/local/bob/work") {
  return renderWith(<MemoryRouter initialEntries={[path]}>{ui}</MemoryRouter>, { api: client });
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  active = [];
  closed = [];
  api.listTasks.mockImplementation(async (filters: TaskFilters) => ({
    tasks: filters.status_view === "closed" ? closed : active,
    sequence: 1,
  }));
  api.listTaskQueues.mockResolvedValue({
    count: 2,
    queues: [
      { prefix: "TASK", name: "Tasks" },
      { prefix: "DEV", name: "Development" },
    ],
  });
  api.getQueueWorkflow.mockImplementation(async (queue: string) =>
    queue === "DEV" ? { queue: "DEV", name: "development", version: "0.10.1" } : null,
  );
  api.createTask.mockImplementation(async (input: { queue: string; title: string }) =>
    taskItem({ key: `${input.queue}-9`, queue: input.queue, title: input.title }),
  );
  api.getTaskActivity.mockResolvedValue({ tasks: [], totals: {} });
});

const composer = () => screen.getByRole("textbox", { name: "New task for bob" });

it("creates a P2 task assigned to the agent in TASK when Enter is pressed", async () => {
  render(
    <AgentWorkTab base="/agents/local/bob" agent={bob} needs={{ count: 0 }} refresh={vi.fn()} />,
  );
  fireEvent.change(composer(), { target: { value: "Add a README" } });
  await act(async () => {
    fireEvent.keyDown(composer(), { key: "Enter" });
  });
  expect(api.createTask).toHaveBeenCalledTimes(1);
  expect(api.createTask).toHaveBeenCalledWith(
    expect.objectContaining({
      queue: "TASK",
      title: "Add a README",
      priority: "P2",
      assignee: "agent:bob",
    }),
  );
  expect(composer()).toHaveValue("");
  expect(JSON.parse(localStorage.getItem("tasks:new-task-queue-by-agent:v1") ?? "{}")).toEqual({
    local: { bob: "TASK" },
  });
});

it("does nothing for an empty title", async () => {
  render(
    <AgentWorkTab base="/agents/local/bob" agent={bob} needs={{ count: 0 }} refresh={vi.fn()} />,
  );
  await act(async () => {
    fireEvent.keyDown(composer(), { key: "Enter" });
  });
  expect(api.createTask).not.toHaveBeenCalled();
});

it("sends no assignee to a bound queue, shows the note and then the dispatched line", async () => {
  localStorage.setItem(
    "tasks:new-task-queue-by-agent:v1",
    JSON.stringify({ local: { bob: "DEV" } }),
  );
  render(
    <AgentWorkTab base="/agents/local/bob" agent={bob} needs={{ count: 0 }} refresh={vi.fn()} />,
  );
  expect(
    await screen.findByText("Dispatched by the development workflow; a pool member takes it"),
  ).toBeInTheDocument();
  fireEvent.change(composer(), { target: { value: "Zoom hotkeys" } });
  await act(async () => {
    fireEvent.keyDown(composer(), { key: "Enter" });
  });
  const input = api.createTask.mock.calls[0]![0];
  expect(input).toMatchObject({ queue: "DEV", title: "Zoom hotkeys" });
  expect(input).not.toHaveProperty("assignee");
  fireEvent.click(
    await screen.findByRole("button", { name: "DEV-9 dispatched by the development workflow" }),
  );
  expect(screen.getByRole("dialog")).toHaveTextContent("drawer DEV-9");
});

it("offers Start & give for an agent that is off and starts it after the create", async () => {
  api.startAgent.mockResolvedValue({});
  const refresh = vi.fn();
  render(
    <AgentWorkTab
      base="/agents/local/bob"
      agent={{ ...bob, enabled: false }}
      needs={{ count: 0 }}
      refresh={refresh}
    />,
  );
  fireEvent.change(composer(), { target: { value: "Fix lint" } });
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Start & give" }));
  });
  expect(api.createTask).toHaveBeenCalled();
  expect(api.startAgent).toHaveBeenCalledWith("bob");
  expect(refresh).toHaveBeenCalled();
});

it("says when Goal is off and links to Settings", () => {
  render(
    <AgentWorkTab
      base="/agents/local/bob"
      agent={{ ...bob, goal_enabled: false }}
      needs={{ count: 0 }}
      refresh={vi.fn()}
    />,
  );
  expect(screen.getByText(/Goal is off: this agent does not pick up tasks/)).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "Turn it on in Settings" })).toHaveAttribute(
    "href",
    "/agents/local/bob/settings",
  );
});

it("lists the cards and the sections with counts, and opens a row in the drawer", async () => {
  active = [
    taskItem({ key: "TASK-1", title: "Working", status: "in_progress", category: "in_progress" }),
    taskItem({ key: "TASK-2", title: "Waiting", status: "open", category: "open" }),
    taskItem({
      key: "DEV-4",
      title: "Plan",
      workflow_digest: "d",
      status: "approval",
      category: "wait_customer",
      waiting_on: "customer",
    }),
  ];
  closed = [
    taskItem({
      key: "TASK-5",
      title: "Readme",
      status: "done",
      category: "done",
      completed_at: ago(1),
    }),
    taskItem({
      key: "TASK-6",
      title: "Old one",
      status: "done",
      category: "done",
      completed_at: ago(30),
    }),
  ];
  render(
    <AgentWorkTab
      base="/agents/local/bob"
      agent={bob}
      needs={{ count: 1, items: [{ key: "DEV-4", at: "", question: false }] }}
      refresh={vi.fn()}
    />,
  );
  expect(await screen.findByText("Working")).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "Needs you (1)" })).toBeInTheDocument();
  expect(screen.getByTestId("needs-card")).toHaveTextContent("DEV-4");
  expect(screen.getByRole("heading", { name: "In progress (1)" })).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "Queued (1)" })).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "Done · last 7 days (1)" })).toBeInTheDocument();
  expect(screen.queryByText("Old one")).not.toBeInTheDocument();
  expect(screen.queryByText("Plan")).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Show all" }));
  expect(screen.getByText("Old one")).toBeInTheDocument();
  fireEvent.change(screen.getByRole("searchbox", { name: "Search done tasks" }), {
    target: { value: "readme" },
  });
  expect(screen.queryByText("Old one")).not.toBeInTheDocument();
  fireEvent.click(screen.getByText("Working"));
  expect(screen.getByRole("dialog")).toHaveTextContent("drawer TASK-1");
});

it("starts a task in its initial status from the Queued row", async () => {
  active = [
    taskItem({
      key: "DEV-7",
      title: "Backlog item",
      workflow_digest: "d",
      status: "backlog",
      category: "wait_customer",
      waiting_on: "customer",
      workflow_initial: true,
      workflow_outcomes: [
        { on: "drop", to: "dropped", terminal: true },
        { on: "start", to: "plan" },
      ],
    }),
  ];
  api.advanceTask.mockResolvedValue({ id: 1, state: "applied" });
  render(
    <AgentWorkTab base="/agents/local/bob" agent={bob} needs={{ count: 0 }} refresh={vi.fn()} />,
  );
  const row = (await screen.findByText("Backlog item")).closest("li")!;
  expect(within(row).getByText(/^Not started · /)).toBeInTheDocument();
  expect(within(row).getByRole("button", { name: "drop → dropped" })).toBeInTheDocument();
  await act(async () => {
    fireEvent.click(within(row).getByRole("button", { name: "Start" }));
  });
  expect(api.advanceTask).toHaveBeenCalledWith("DEV-7", "start", "", "backlog");
});

it("focuses the card the header's Answer names and the composer for Give task", async () => {
  const needs = { count: 1, items: [{ key: "DEV-4", at: "", question: false }] };
  const { unmount } = render(
    <AgentWorkTab base="/agents/local/bob" agent={bob} needs={needs} refresh={vi.fn()} />,
    "/agents/local/bob/work?answer=DEV-4",
  );
  expect(screen.getByTestId("needs-card")).toHaveTextContent("DEV-4 focused");
  unmount();
  render(
    <AgentWorkTab base="/agents/local/bob" agent={bob} needs={{ count: 0 }} refresh={vi.fn()} />,
    "/agents/local/bob/work?give=1",
  );
  await waitFor(() => expect(composer()).toHaveFocus());
});

it("opens the drawer for ?task=", () => {
  render(
    <AgentWorkTab base="/agents/local/bob" agent={bob} needs={{ count: 0 }} refresh={vi.fn()} />,
    "/agents/local/bob/work?task=TASK-3",
  );
  expect(screen.getByRole("dialog")).toHaveTextContent("drawer TASK-3");
});

it("opens a Needs you task in the existing Work drawer", async () => {
  render(
    <AgentWorkTab
      base="/agents/local/bob"
      agent={bob}
      needs={{ count: 1, items: [{ key: "DEV-4", at: "", question: false }] }}
      refresh={vi.fn()}
    />,
  );
  fireEvent.click(await screen.findByRole("button", { name: "DEV-4" }));
  expect(screen.getByRole("dialog")).toHaveTextContent("drawer DEV-4");
  expect(api.advanceTask).not.toHaveBeenCalled();
});
