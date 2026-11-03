import { afterEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { ApiError, type Task, type TaskMetrics } from "@/lib/api";
import { apiWrapper, fakeApi } from "@/test/api";
import { taskItem, taskMetrics } from "@/test/fixtures/apiFixtures";
import { TaskMetricsSection } from "./TaskMetricsSection";

type Answer = TaskMetrics | Error | (() => Promise<TaskMetrics>);

const workflowTask = (over: Partial<Task> = {}) =>
  taskItem({
    key: "DEV-1",
    status: "review",
    category: "in_progress",
    workflow_name: "development",
    workflow_digest: "sha256:abc",
    ...over,
  });

function renderSection(task: Task = workflowTask(), answer: Answer = taskMetrics()) {
  const getTaskMetrics = vi.fn((_key: string, _signal?: AbortSignal) => {
    if (typeof answer === "function") return answer();
    return answer instanceof Error ? Promise.reject(answer) : Promise.resolve(answer);
  });
  const Wrapper = apiWrapper(fakeApi({ metrics: { getTaskMetrics } }));
  const view = render(
    <MemoryRouter>
      <Wrapper>
        <TaskMetricsSection task={task} />
      </Wrapper>
    </MemoryRouter>,
  );
  return { getTaskMetrics, ...view };
}

const toggle = () => screen.getByRole("button", { name: /^Metrics/ });
const open = async () => {
  fireEvent.click(toggle());
  await screen.findByRole("heading", { name: "Time by status" });
};
/** The value next to a summary label. */
const field = (label: string) =>
  screen.getByText(label, { selector: "dt" }).nextElementSibling as HTMLElement;

afterEach(() => localStorage.clear());

describe("TaskMetricsSection", () => {
  it("starts closed and asks for nothing", () => {
    const { getTaskMetrics } = renderSection();
    expect(toggle()).toHaveAttribute("aria-expanded", "false");
    expect(getTaskMetrics).not.toHaveBeenCalled();
  });

  it("loads on open, remembers it, and loads again on the next open of the task", async () => {
    const first = renderSection();
    await open();
    expect(toggle()).toHaveAttribute("aria-expanded", "true");
    expect(toggle()).toHaveAttribute("aria-controls", expect.any(String));
    expect(first.getTaskMetrics).toHaveBeenCalledWith("DEV-1", expect.any(AbortSignal));
    expect(localStorage.getItem("tasks:metrics-open:v1")).toBe("open");
    first.unmount();

    const second = renderSection();
    expect(toggle()).toHaveAttribute("aria-expanded", "true");
    expect(second.getTaskMetrics).toHaveBeenCalledTimes(1);
    fireEvent.click(toggle());
    expect(localStorage.getItem("tasks:metrics-open:v1")).toBe("closed");
  });

  it("shows the short line after the load and refreshes on request", async () => {
    const { getTaskMetrics } = renderSection();
    await open();
    expect(toggle()).toHaveTextContent("$12.40 · 23 runs · 2 rework");
    fireEvent.click(screen.getByRole("button", { name: "Refresh metrics" }));
    await waitFor(() => expect(getTaskMetrics).toHaveBeenCalledTimes(2));
  });

  it("shows every summary field", async () => {
    renderSection();
    await open();
    expect(field("Lead time")).toHaveTextContent("1d 0h");
    expect(field("Cost")).toHaveTextContent("$12.40");
    expect(field("Runs")).toHaveTextContent("23");
    expect(within(field("Tokens")).getByText("8.6M")).toHaveAttribute(
      "title",
      "Input 3,000,000 · Output 400,000 · Cache write 200,000 · Cache read 5,000,000",
    );
    expect(field("Rework rounds")).toHaveTextContent(
      "2Changes requested 1 · Rework 1 · Conflict 0",
    );
    expect(field("Rejected advances")).toHaveTextContent("1");
    expect(field("Questions to customer")).toHaveTextContent("3");
    expect(field("Started")).toHaveTextContent(new Date("2026-10-30T09:00:00Z").toLocaleString());
    expect(field("Completed")).toHaveTextContent(new Date("2026-10-31T09:00:00Z").toLocaleString());
  });

  it("says Not done yet for the lead time of an open task", async () => {
    renderSection(workflowTask(), taskMetrics({ lead_time_s: null, completed_at: "" }));
    await open();
    expect(field("Lead time")).toHaveTextContent("Not done yet");
  });

  it("explains every unknown value with a hint and a screen reader text", async () => {
    renderSection(
      workflowTask(),
      taskMetrics({ lead_time_s: null, started_at: "", completed_at: "" }),
    );
    await open();
    expect(within(field("Started")).getByTitle("The task has not started yet.")).toHaveTextContent(
      "—",
    );
    expect(field("Started")).toHaveTextContent("not known: The task has not started yet.");
    expect(within(field("Completed")).getByTitle("The task is not done yet.")).toBeInTheDocument();
    expect(field("Completed")).toHaveTextContent("not known: The task is not done yet.");
  });

  it("explains a done task without a start or completion time", async () => {
    renderSection(
      workflowTask({ status: "done", category: "done" }),
      taskMetrics({ lead_time_s: null, completed_at: "" }),
    );
    await open();
    expect(
      within(field("Lead time")).getByTitle("The task has no start time."),
    ).toBeInTheDocument();
    expect(field("Completed")).toHaveTextContent("not known: The task has no completion time.");
  });

  it("shows zero rework with a reason for a task without a workflow", async () => {
    const flexible = taskItem({ key: "DEV-1", status: "in_progress", category: "in_progress" });
    renderSection(flexible, taskMetrics({ rework_rounds: 0, rejected_advances: 0 }));
    await open();
    const reason = "This task has no workflow, so it has no rework rounds.";
    expect(within(field("Rework rounds")).getByTitle(reason)).toHaveTextContent("0");
    expect(within(field("Rejected advances")).getByTitle(reason)).toHaveTextContent("0");
  });

  it("lists the time by status, longest first, with the current status", async () => {
    renderSection(
      workflowTask({ status: "review" }),
      taskMetrics({
        status: "review",
        status_time_s: { implement: 5400, review: 600, plan: 3000 },
      }),
    );
    await open();
    const list = screen.getByRole("list", { name: "Time by status" });
    expect(
      within(list)
        .getAllByRole("listitem")
        .map((li) => li.textContent),
    ).toEqual(["Implement1h 30m60%", "Plan50m33%", "Review10m7%current, still counting"]);
  });

  it("labels the statuses of a task without a workflow", async () => {
    renderSection(
      taskItem({ key: "DEV-1", status: "wait_customer", category: "wait_customer" }),
      taskMetrics({ status: "wait_customer", status_time_s: { wait_customer: 60, open: 60 } }),
    );
    await open();
    const list = screen.getByRole("list", { name: "Time by status" });
    expect(within(list).getByText("Waiting for you")).toBeInTheDocument();
    expect(within(list).getByText("Open")).toBeInTheDocument();
  });

  it("says so when no time is in a status yet", async () => {
    renderSection(workflowTask(), taskMetrics({ status_time_s: {} }));
    await open();
    expect(screen.getByText("No time in a status yet.")).toBeInTheDocument();
  });

  it("lists the agents in a table with a pinned row header", async () => {
    const image = (version: string) => ({
      image: "developer",
      image_version: version,
      image_digest: "",
      image_ref: "",
      iterations: 2,
      cost_usd: 0.5,
      tokens: { input: 1, output: 1, cache_write: 0, cache_read: 0 },
    });
    renderSection(
      workflowTask(),
      taskMetrics({
        agents: [
          {
            agent: "alice",
            usage: { iterations: 4, cost_usd: 1.24, tokens: taskMetrics().usage.tokens },
            images: [image("0.23.2"), image("0.23.1")],
          },
        ],
      }),
    );
    await open();
    const table = screen.getByRole("table", { name: "Agents" });
    const headers = within(table).getAllByRole("columnheader");
    expect(headers.map((h) => h.textContent)).toEqual(["Agent", "Runs", "Cost", "Image versions"]);
    for (const h of headers) expect(h).toHaveAttribute("scope", "col");
    const rowHeader = within(table).getByRole("rowheader");
    expect(rowHeader).toHaveTextContent("alice");
    expect(rowHeader).toHaveAttribute("scope", "row");
    expect(rowHeader.className).toContain("sticky");
    expect(
      within(table)
        .getAllByRole("cell")
        .map((c) => c.textContent),
    ).toEqual(["4", "$1.24", "developer · 0.23.2, developer · 0.23.1"]);
  });

  it("shows a task without runs with zeros and no agents", async () => {
    const zero = {
      iterations: 0,
      cost_usd: 0,
      tokens: { input: 0, output: 0, cache_write: 0, cache_read: 0 },
    };
    renderSection(workflowTask(), taskMetrics({ usage: zero, agents: [], images: [] }));
    await open();
    expect(field("Cost")).toHaveTextContent("$0.00");
    expect(field("Runs")).toHaveTextContent("0");
    expect(screen.getByText("No agent runs recorded.")).toBeInTheDocument();
    expect(screen.getByText("No workflow")).toBeInTheDocument();
  });

  it("links a task on two image versions to their comparison", async () => {
    const image = (version: string) => ({
      image: "developer",
      image_version: version,
      image_digest: "",
      image_ref: "",
      iterations: 3,
      cost_usd: 0.6,
      tokens: { input: 0, output: 0, cache_write: 0, cache_read: 0 },
    });
    renderSection(
      workflowTask(),
      taskMetrics({
        images: [image("0.23.1"), image("0.23.2"), image("unknown")],
        workflows: [{ name: "development", version: "0.10.1", digest: "d" }],
      }),
    );
    await open();
    const versions = screen.getByRole("region", { name: "Versions" });
    expect(within(versions).getAllByRole("link", { name: "developer" })[0]).toHaveAttribute(
      "href",
      "/library/images/developer",
    );
    expect(within(versions).getByText("unknown version")).toHaveAttribute(
      "title",
      "The run's image version is not recorded, for example after data retention.",
    );
    expect(within(versions).getByText("development · 0.10.1")).toBeInTheDocument();
    expect(
      within(versions).getByText(
        "This task ran on 2 image versions. Version comparisons do not count it.",
      ),
    ).toBeInTheDocument();
    expect(within(versions).getByRole("link", { name: "Compare these versions" })).toHaveAttribute(
      "href",
      "/metrics/compare?host=local&period=quarter&kind=image&name=developer&a=0.23.1&b=0.23.2",
    );
  });

  it("shows the empty Quality state and partial scores", async () => {
    const first = renderSection();
    await open();
    expect(screen.getByText("No quality scores yet")).toBeInTheDocument();
    first.unmount();
    renderSection(
      workflowTask(),
      taskMetrics({
        quality: { image_process: { avg: 80, min: 70, count: 2 }, role: null, task_fit: null },
      }),
    );
    expect(await screen.findByRole("listitem", { name: "Role" })).toHaveTextContent("No scores");
    expect(screen.getByRole("listitem", { name: "Image and process" })).toHaveTextContent(
      "80min 70 · 2 scores",
    );
  });

  it("announces the load", async () => {
    renderSection(workflowTask(), () => new Promise(() => {}));
    fireEvent.click(toggle());
    expect(await screen.findByText("Loading metrics…")).toHaveAttribute("role", "status");
  });

  it("shows an error with Retry", async () => {
    const { getTaskMetrics } = renderSection(workflowTask(), new ApiError(500, "internal", "boom"));
    fireEvent.click(toggle());
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Could not load task metrics.");
    expect(alert).toHaveTextContent("boom");
    fireEvent.click(within(alert).getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(getTaskMetrics).toHaveBeenCalledTimes(2));
  });

  it("says a task without metrics has none", async () => {
    renderSection(workflowTask(), new ApiError(404, "not_found", 'task "DEV-1" not found'));
    fireEvent.click(toggle());
    expect(await screen.findByText("Metrics are not available for this task.")).toBeInTheDocument();
  });

  it("explains an older daemon", async () => {
    renderSection(
      workflowTask(),
      new ApiError(404, "not_found", "unknown route GET /api/metrics/tasks/DEV-1"),
    );
    fireEvent.click(toggle());
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "This host runs an older daemon without metrics.",
    );
  });

  it("explains a timeout", async () => {
    renderSection(workflowTask(), new ApiError(0, "network_error", "request timed out"));
    fireEvent.click(toggle());
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The host did not answer in 30 seconds.",
    );
  });

  it("keeps the numbers, marked as updating, while a refresh loads", async () => {
    let calls = 0;
    renderSection(workflowTask(), () =>
      calls++ === 0 ? Promise.resolve(taskMetrics()) : new Promise(() => {}),
    );
    await open();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Refresh metrics" })));
    const region = screen.getByRole("region", { name: "Task metrics" });
    expect(region).toHaveAttribute("aria-busy", "true");
    expect(within(region).getByText("Updating…")).toBeInTheDocument();
    expect(field("Cost")).toHaveTextContent("$12.40");
  });
});
