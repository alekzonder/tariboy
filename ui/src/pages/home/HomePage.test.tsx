import { beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { fireEvent, render, screen, waitFor, within } from "@/test/queryRender";
import type { HostAgents } from "@/lib/aggregate";
import type { Letter, Task } from "@/types";
import type { HostNeeds } from "@/lib/needsYou";
import { agentRow } from "@/test/fixtures/apiFixtures";
import { letter } from "@/test/fixtures/mailFixtures";
import { RoutedLocation } from "@/test/terminals/RoutedLocation";
import { NeedsYouContext } from "@/pages/terminals/needsYouContext";
import HomePage from "./HomePage";

vi.mock("@/hooks/useTaskActivity", () => ({
  useTaskActivity: () => ({
    report: {
      totals: {
        tasks: 35,
        completed: 32,
        transitions: 4,
        comments: 2,
        iterations: 101,
        cost_usd: 145.23,
      },
    },
    error: "",
  }),
}));
vi.mock("@/pages/tasks/NeedsYouCard", () => ({
  default: ({ taskKey }: { taskKey: string }) => <div data-testid="needs-card">{taskKey}</div>,
}));
vi.mock("@/pages/tasks/TaskDrawer", () => ({
  default: ({ taskKey, onClose }: { taskKey: string; onClose: () => void }) => (
    <div data-testid="task-drawer">
      {taskKey}
      <button onClick={onClose}>Close drawer</button>
    </div>
  ),
}));

const startedAt = new Date(Date.now() - 12 * 60_000).toISOString();
const hosts: HostAgents[] = [
  {
    host: { id: "", label: "Local" },
    agents: [
      agentRow({
        name: "techlead",
        state: "running",
        current_iteration_started_at: startedAt,
        last_iteration_at: startedAt,
      }),
      agentRow({ name: "homer", state: "idle" }),
    ],
  },
];
const needs = (over: Partial<HostNeeds> = {}): HostNeeds => ({
  hostId: "",
  hostLabel: "Local",
  tasks: [],
  letters: [],
  open: 0,
  ...over,
});
const task = {
  key: "DEV-1",
  title: "Zoom hotkeys",
  status: "approval",
  waiting_on: "customer",
  assignee: "agent:bob",
  updated_at: "2026-10-30T10:00:00Z",
} as Task;

function renderHome(value: HostNeeds[] | null, reload = vi.fn()) {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <NeedsYouContext.Provider value={{ hosts: value ?? [], loaded: value !== null, reload }}>
        <HomePage hosts={hosts} pageHostId="" onNewAgent={vi.fn()} />
      </NeedsYouContext.Provider>
      <RoutedLocation />
    </MemoryRouter>,
  );
}

beforeEach(() => vi.clearAllMocks());

describe("HomePage", () => {
  it("shows the four sections and says when nothing needs the user", () => {
    renderHome([needs()]);
    expect(screen.getByRole("heading", { name: "Needs you (0)" })).toBeInTheDocument();
    expect(screen.getByText("Nothing needs you.")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Running now (1)" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Today" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Recent" })).toBeInTheDocument();
    expect(screen.getByText("101")).toBeInTheDocument();
  });

  it("shows a running agent with the time of its iteration and links to its Terminal", () => {
    renderHome([needs()]);
    const [link, recent] = screen.getAllByRole("link", { name: /techlead\s*12 min/ });
    expect(link).toHaveAttribute("href", "/agents/local/techlead/console");
    // Recent says when the last iteration was, not whether the agent runs.
    expect(recent).toHaveTextContent("techlead12 min ago");
    expect(screen.getByRole("link", { name: /homer\s*not run yet/ })).toBeInTheDocument();
  });

  it("shows 7 of 100 letters, counts 100+ and links Show all to the Inbox", () => {
    const letters = Array.from({ length: 100 }, (_, i) =>
      letter({ id: `m${i}`, subject: `Question ${i}`, data: { task_key: `T-${i}` } }),
    );
    renderHome([needs({ letters, open: 100 })]);
    expect(screen.getByRole("heading", { name: "Needs you (100+)" })).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /Question \d+/ })).toHaveLength(7);
    expect(screen.getByRole("link", { name: "Show all (100+)" })).toHaveAttribute("href", "/inbox");
  });

  it("sends Show all to Tasks when a task is among the rows", () => {
    const letters = Array.from({ length: 7 }, (_, i) =>
      letter({ id: `m${i}`, subject: `Question ${i}`, data: { task_key: `T-${i}` } }),
    );
    renderHome([needs({ tasks: [task], letters })]);
    expect(screen.getByRole("link", { name: "Show all (8)" })).toHaveAttribute("href", "/tasks");
  });

  it("merges the repeated letters of a task and keeps the subject first", () => {
    const repeated = [1, 2, 3, 4].map((i) =>
      letter({
        id: `r${i}`,
        subject: "Answer on TASK-nzex: pick an image",
        data: { task_key: "TASK-nzex" },
        created_at: `2026-10-30T10:0${i}:00Z`,
      }),
    );
    renderHome([needs({ letters: repeated, open: 4 })]);
    expect(screen.getByRole("heading", { name: "Needs you (1)" })).toBeInTheDocument();
    const row = screen.getByRole("button", { name: /pick an image/ });
    expect(row).toHaveTextContent(/4 letters · Answer on TASK-nzex/);
    expect(screen.queryByRole("link", { name: /Show all/ })).toBeNull();
  });

  it("opens a task in the drawer and reads again when it closes", async () => {
    const reload = vi.fn();
    renderHome([needs({ tasks: [task] })], reload);
    fireEvent.click(screen.getByRole("button", { name: /DEV-1/ }));
    expect(await screen.findByTestId("task-drawer")).toHaveTextContent("DEV-1");
    fireEvent.click(screen.getByRole("button", { name: "Close drawer" }));
    expect(reload).toHaveBeenCalled();
  });

  it("expands a task row into its Needs you card, and the row still opens the drawer", async () => {
    renderHome([needs({ tasks: [task] })]);
    const expand = screen.getByRole("button", { name: "Answer in place" });
    expect(expand).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(expand);
    expect(expand).toHaveAttribute("aria-expanded", "true");
    expect(await screen.findByTestId("needs-card")).toHaveTextContent("DEV-1");
    fireEvent.click(screen.getByRole("button", { name: /DEV-1/ }));
    expect(await screen.findByTestId("task-drawer")).toHaveTextContent("DEV-1");
  });

  it("opens a letter in the Inbox", async () => {
    const open: Letter = letter({ id: "m-9", subject: "Question" });
    renderHome([needs({ letters: [open], open: 1 })]);
    fireEvent.click(screen.getByRole("button", { name: /Question/ }));
    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toBe("/inbox?letter=m-9"),
    );
  });

  it("offers Retry when no host could be read", () => {
    const reload = vi.fn();
    renderHome([needs({ error: "connection refused" })], reload);
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("connection refused");
    fireEvent.click(within(alert).getByRole("button", { name: "Retry" }));
    expect(reload).toHaveBeenCalled();
  });

  it("links History to the Activity table", () => {
    renderHome([needs()]);
    expect(screen.getByRole("link", { name: /History/ })).toHaveAttribute("href", "/activity");
  });
});
