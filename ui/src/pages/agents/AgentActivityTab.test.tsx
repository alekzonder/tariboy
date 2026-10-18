import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { AgentNameContext } from "@/lib/agent";
import AgentActivityTab from "./AgentActivityTab";

const REPORT = {
  since: "", until: "", agent: "alice", truncated: false,
  totals: { tasks: 1, completed: 1, transitions: 2, comments: 3, iterations: 4, requests: 9, tokens: 900, cost_usd: 1.25 },
  tasks: [{
    key: "DEV-1", title: "Ship the report", queue: "DEV", status: "review", category: "in_progress",
    assignee: "agent:alice", completed: true, iterations: 4, requests: 9, tokens: 900, cost_usd: 1.25,
    comments: 3, outcomes: ["ready"], entered_statuses: ["review"], agents: ["alice"],
    first_at: "2026-10-18T10:00:00.000Z", last_at: "2026-10-18T11:00:00.000Z",
  }],
};

let urls: string[] = [];
beforeEach(() => {
  urls = [];
  vi.stubGlobal("EventSource", class { addEventListener() {} removeEventListener() {} close() {} } as unknown as typeof EventSource);
  vi.stubGlobal("fetch", vi.fn().mockImplementation((url: string) => {
    urls.push(url);
    let result: unknown = { events: [] };
    if (url.includes("/api/task-activity")) result = REPORT;
    else if (url.includes("/iterations")) result = { iterations: [], count: 0 };
    else if (url.includes("status/history")) result = {
      events: [
        { ts: new Date(Date.now() - 3600e3).toISOString(), message: "recent status" },
        { ts: "2020-01-01T00:00:00Z", message: "old status" },
      ], count: 2,
    };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response);
  }));
});
afterEach(() => vi.restoreAllMocks());

function Location() {
  return <output data-testid="location">{useLocation().search}</output>;
}

const renderTab = (search = "") => render(
  <MemoryRouter initialEntries={[`/agents/local/alice/activity${search}`]}>
    <AgentNameContext.Provider value="alice">
      <Routes>
        <Route path="/agents/:hostId/:agent/activity" element={<><Location /><AgentActivityTab /></>} />
      </Routes>
    </AgentNameContext.Provider>
  </MemoryRouter>,
);

describe("AgentActivityTab", () => {
  it("summarizes the agent's tasks for the last day and links each task key", async () => {
    renderTab();
    const link = await screen.findByRole("link", { name: "DEV-1" });
    expect(link).toHaveAttribute("href", "/agents/local/alice/tasks?task=DEV-1");
    expect(screen.getByRole("button", { name: "Day" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText("Ship the report")).toBeInTheDocument();
    expect(screen.getByText("$1.25", { selector: "[data-total] *" })).toBeInTheDocument();
    const request = new URL(urls.find((u) => u.includes("/api/task-activity"))!, "http://x");
    expect(request.searchParams.get("agent")).toBe("alice");
    const span = Date.parse(request.searchParams.get("until")!) - Date.parse(request.searchParams.get("since")!);
    expect(span).toBe(24 * 3600e3);
  });

  it("keeps the period in the URL and reloads the summary for it", async () => {
    renderTab("?keep=1");
    await screen.findByRole("link", { name: "DEV-1" });
    fireEvent.click(screen.getByRole("button", { name: "Week" }));
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent("keep=1&period=week"));
    await waitFor(() => expect(urls.some((u) => {
      const q = new URL(u, "http://x").searchParams;
      return u.includes("/api/task-activity") && Date.parse(q.get("until")!) - Date.parse(q.get("since")!) === 7 * 24 * 3600e3;
    })).toBe(true));
  });

  it("asks for a custom range and reports a reversed one", async () => {
    renderTab("?period=custom&since=2026-10-02T00:00&until=2026-10-01T00:00");
    expect(await screen.findByText("The start must be before the end.")).toBeInTheDocument();
    expect(screen.getByLabelText("From")).toHaveValue("2026-10-02T00:00");
    fireEvent.change(screen.getByLabelText("To"), { target: { value: "2026-10-03T00:00" } });
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent("until=2026-10-03T00%3A00"));
    expect(await screen.findByRole("link", { name: "DEV-1" })).toBeInTheDocument();
  });

  it("opens the Iterations view for an iteration link and filters it by the period", async () => {
    renderTab("?iteration=it-1");
    expect(await screen.findByText("Full log")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Iterations" })).toHaveAttribute("aria-selected", "true");
    await waitFor(() => expect(urls.some((u) => u.includes("/iterations?") && u.includes("started_after="))).toBe(true));
  });

  it("shows only status changes inside the period", async () => {
    renderTab("?view=status");
    expect(await screen.findByText("recent status")).toBeInTheDocument();
    expect(screen.queryByText("old status")).not.toBeInTheDocument();
  });
});
