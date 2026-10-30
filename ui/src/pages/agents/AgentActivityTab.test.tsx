import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@/test/queryRender";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { AgentNameContext } from "@/lib/agent";
import AgentActivityTab from "./AgentActivityTab";

vi.mock("@/pages/tasks/TaskDrawer", () => ({
  default: ({ taskKey }: { taskKey: string }) => <div data-testid="task-drawer">{taskKey}</div>,
}));

const TOTALS = {
  tasks: 3,
  completed: 2,
  transitions: 2,
  comments: 3,
  iterations: 4,
  requests: 9,
  tokens: 900,
  cost_usd: 3,
};

const REPORT = {
  since: "",
  until: "",
  agent: "alice",
  truncated: false,
  totals: TOTALS,
  tasks: [
    {
      key: "DEV-1",
      title: "Ship the report",
      queue: "DEV",
      status: "review",
      category: "in_progress",
      assignee: "agent:alice",
      completed: true,
      iterations: 4,
      requests: 9,
      tokens: 900,
      cost_usd: 1.25,
      comments: 3,
      outcomes: ["ready"],
      entered_statuses: ["review"],
      agents: ["alice"],
      first_at: "2026-10-18T10:00:00.000Z",
      last_at: "2026-10-18T11:00:00.000Z",
    },
  ],
};

const USAGE = {
  agent: "alice",
  group_by: "model",
  bucket: "1h",
  totals: {
    requests: 3,
    input_tokens: 1200,
    output_tokens: 800,
    cache_write_tokens: 0,
    cache_read_tokens: 500,
    cost_usd: 0.35,
  },
  rows: [],
  series: [{ bucket_start: "2026-10-18T10:00:00Z", requests: 3, tokens: 2000, cost_usd: 0.35 }],
};

let urls: string[] = [];
let report: typeof REPORT = REPORT;
beforeEach(() => {
  urls = [];
  report = REPORT;
  vi.stubGlobal(
    "EventSource",
    class {
      addEventListener() {}
      removeEventListener() {}
      close() {}
    } as unknown as typeof EventSource,
  );
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((url: string) => {
      urls.push(url);
      let result: unknown = { events: [] };
      if (url.includes("/api/task-activity")) result = report;
      else if (url.includes("/usage")) result = USAGE;
      else if (url.includes("/iterations")) result = { iterations: [], count: 0 };
      else if (url.includes("status/history"))
        result = {
          events: [
            { ts: new Date(Date.now() - 60e3).toISOString(), message: "recent status" },
            { ts: "2020-01-01T00:00:00Z", message: "old status" },
          ],
          count: 2,
        };
      return Promise.resolve({
        ok: true,
        status: 200,
        text: async () => JSON.stringify({ ok: true, result }),
      } as Response);
    }),
  );
});
afterEach(() => vi.restoreAllMocks());

function Location() {
  const location = useLocation();
  return (
    <output data-testid="location" data-path={location.pathname}>
      {location.search}
    </output>
  );
}

const renderTab = (search = "") =>
  render(
    <MemoryRouter initialEntries={[`/agents/local/alice/activity${search}`]}>
      <AgentNameContext.Provider value="alice">
        <Routes>
          <Route
            path="/agents/:hostId/:agent/activity"
            element={
              <>
                <Location />
                <AgentActivityTab />
              </>
            }
          />
        </Routes>
      </AgentNameContext.Provider>
    </MemoryRouter>,
  );

const total = (label: string) =>
  document.querySelector(`[data-total="${label}"]`)?.textContent ?? "";

const more = () => screen.getByRole("radiogroup", { name: "More" });

describe("AgentActivityTab", () => {
  it("summarizes the agent's tasks for today and opens a task key in place", async () => {
    renderTab();
    fireEvent.click(await screen.findByRole("button", { name: "DEV-1" }));
    expect(screen.getByTestId("task-drawer")).toHaveTextContent("DEV-1");
    expect(screen.getByTestId("location")).toHaveAttribute(
      "data-path",
      "/agents/local/alice/activity",
    );
    expect(screen.getByText("Ship the report")).toBeInTheDocument();
    const request = new URL(
      urls.find((u) => u.includes("/api/task-activity"))!,
      "http://x",
    );
    expect(request.searchParams.get("agent")).toBe("alice");
    const midnight = new Date(request.searchParams.get("until")!);
    midnight.setHours(0, 0, 0, 0);
    expect(request.searchParams.get("since")).toBe(midnight.toISOString());
    expect(screen.getByTestId("tab-page")).toContainElement(
      screen.getByRole("button", { name: "Today" }),
    );
  });

  it("offers Today, Week, Month and Custom with Today selected", async () => {
    renderTab();
    await screen.findByRole("button", { name: "DEV-1" });
    const bar = screen.getByRole("group", { name: "Period" });
    expect(
      within(bar)
        .getAllByRole("button")
        .map((b) => b.textContent),
    ).toEqual(["Today", "Week", "Month", "Custom"]);
    expect(screen.getByRole("button", { name: "Today" })).toHaveAttribute("aria-pressed", "true");
  });

  it("shows tasks done, runs, cost and average cost per task", async () => {
    renderTab();
    await screen.findByRole("button", { name: "DEV-1" });
    expect(
      [...document.querySelectorAll("[data-total]")].map((n) => n.getAttribute("data-total")),
    ).toEqual(["Tasks done", "Runs", "Cost", "Avg. cost per task"]);
    expect(total("Tasks done")).toContain("2");
    expect(total("Runs")).toContain("4");
    expect(total("Cost")).toContain("$3.00");
    expect(total("Avg. cost per task")).toContain("$1.50");
  });

  it("shows a dash for the average cost when no task was completed", async () => {
    report = { ...REPORT, totals: { ...TOTALS, completed: 0 } };
    renderTab();
    await screen.findByRole("button", { name: "DEV-1" });
    expect(total("Avg. cost per task")).toContain("—");
  });

  it("draws the agent's cost over the period", async () => {
    renderTab();
    expect(await screen.findByRole("img", { name: "cost over time" })).toBeInTheDocument();
    const usage = new URL(
      urls.find((u) => u.includes("/usage"))!,
      "http://x",
    );
    const activity = new URL(
      urls.find((u) => u.includes("/api/task-activity"))!,
      "http://x",
    );
    expect(usage.searchParams.get("since")).toBe(activity.searchParams.get("since"));
  });

  it("keeps the period in the URL and reloads the summary for it", async () => {
    renderTab("?keep=1");
    await screen.findByRole("button", { name: "DEV-1" });
    fireEvent.click(screen.getByRole("button", { name: "Month" }));
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("keep=1&period=month"),
    );
    await waitFor(() =>
      expect(
        urls.some((u) => {
          const q = new URL(u, "http://x").searchParams;
          return (
            u.includes("/api/task-activity") &&
            Date.parse(q.get("until")!) - Date.parse(q.get("since")!) === 30 * 24 * 3600e3
          );
        }),
      ).toBe(true),
    );
  });

  it("still opens an old day link from the server Activity page", async () => {
    renderTab("?period=day");
    await screen.findByRole("button", { name: "DEV-1" });
    const request = new URL(
      urls.find((u) => u.includes("/api/task-activity"))!,
      "http://x",
    );
    expect(
      Date.parse(request.searchParams.get("until")!) -
        Date.parse(request.searchParams.get("since")!),
    ).toBe(24 * 3600e3);
  });

  it("asks for a custom range and reports a reversed one", async () => {
    renderTab("?period=custom&since=2026-10-02T00:00&until=2026-10-01T00:00");
    expect(await screen.findByText("The start must be before the end.")).toBeInTheDocument();
    expect(screen.getByLabelText("From")).toHaveValue("2026-10-02T00:00");
    fireEvent.change(screen.getByLabelText("To"), { target: { value: "2026-10-03T00:00" } });
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("until=2026-10-03T00%3A00"),
    );
    expect(await screen.findByRole("button", { name: "DEV-1" })).toBeInTheDocument();
  });

  it("switches the More area between Runs log, Usage and Status history", async () => {
    renderTab();
    await screen.findByRole("button", { name: "DEV-1" });
    expect(
      within(more())
        .getAllByRole("radio")
        .map((b) => b.textContent),
    ).toEqual(["Runs log", "Usage", "Status history"]);
    expect(within(more()).getByRole("radio", { name: "Runs log" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    expect(await screen.findByText("Full log")).toBeInTheDocument();

    fireEvent.click(within(more()).getByRole("radio", { name: "Usage" }));
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent("view=usage"));
    expect(await screen.findByRole("button", { name: "Models" })).toBeInTheDocument();
    expect(screen.queryByText("Full log")).not.toBeInTheDocument();

    fireEvent.click(within(more()).getByRole("radio", { name: "Status history" }));
    expect(await screen.findByText("recent status")).toBeInTheDocument();
    expect(screen.queryByText("old status")).not.toBeInTheDocument();
  });

  it("draws the cost chart once when the Usage view is open", async () => {
    renderTab("?view=usage");
    expect(await screen.findByRole("button", { name: "Models" })).toBeInTheDocument();
    await screen.findByRole("img", { name: "cost over time" });
    expect(screen.getAllByRole("img", { name: "cost over time" })).toHaveLength(1);
  });

  it.each([
    ["?view=iterations", "Runs log"],
    ["?view=usage", "Usage"],
    ["?view=status", "Status history"],
    ["?iteration=it-1", "Runs log"],
  ])("opens %s in the matching More view", async (search, label) => {
    renderTab(search);
    await screen.findByRole("button", { name: "DEV-1" });
    expect(within(more()).getByRole("radio", { name: label })).toHaveAttribute(
      "aria-checked",
      "true",
    );
  });

  it("filters the Runs log of an iteration link by the period", async () => {
    renderTab("?iteration=it-1");
    await screen.findByRole("button", { name: "DEV-1" });
    await waitFor(() =>
      expect(urls.some((u) => u.includes("/iterations?") && u.includes("started_after="))).toBe(
        true,
      ),
    );
  });

  it("calls iterations Runs in every label", async () => {
    renderTab("?view=usage");
    await screen.findByRole("button", { name: "DEV-1" });
    expect(screen.getAllByRole("columnheader", { name: "Runs" }).length).toBeGreaterThan(0);
    expect(await screen.findByRole("button", { name: "Runs" })).toBeInTheDocument();
    expect(screen.queryByText(/Iterations/)).not.toBeInTheDocument();
  });
});
