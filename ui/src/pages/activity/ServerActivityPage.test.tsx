import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import ServerActivityPage from "./ServerActivityPage";

const REPORT = {
  since: "", until: "", truncated: true,
  totals: { tasks: 1, completed: 0, transitions: 1, comments: 0, iterations: 2, requests: 3, tokens: 30, cost_usd: 0.5 },
  tasks: [{
    key: "DEV-7", title: "Shared work", queue: "DEV", status: "develop", category: "in_progress",
    assignee: "", completed: false, iterations: 2, requests: 3, tokens: 30, cost_usd: 0.5,
    comments: 0, outcomes: [], entered_statuses: ["develop"], agents: ["alice", "bob"],
    first_at: "2026-10-18T10:00:00.000Z", last_at: "2026-10-18T11:00:00.000Z",
  }],
  agents: [{ agent: "alice", tasks: 1, completed: 0, iterations: 2, requests: 3, cost_usd: 0.5 }],
};

let urls: string[] = [];
beforeEach(() => {
  urls = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation((url: string) => {
    urls.push(url);
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: REPORT }) } as Response);
  }));
});
afterEach(() => vi.restoreAllMocks());

const renderPage = () => render(
  <MemoryRouter initialEntries={["/servers/local/activity?period=week"]}>
    <ServerActivityPage hostId="" target={null} />
  </MemoryRouter>,
);

describe("ServerActivityPage", () => {
  it("lists every agent's tasks with the server task link", async () => {
    renderPage();
    expect(await screen.findByRole("link", { name: "DEV-7" })).toHaveAttribute("href", "/servers/local/tasks?task=DEV-7");
    expect(screen.getByText("alice, bob")).toBeInTheDocument();
    expect(screen.getByText(/Showing the 1 most recent tasks/)).toBeInTheDocument();
    expect(new URL(urls[0], "http://x").searchParams.has("agent")).toBe(false);
  });

  it("breaks the period down by agent and opens an agent with the same period", async () => {
    renderPage();
    await screen.findByRole("link", { name: "DEV-7" });
    fireEvent.click(screen.getByRole("tab", { name: "Agents" }));
    expect(screen.getByRole("link", { name: "alice" })).toHaveAttribute("href", "/agents/local/alice/activity?period=week");
  });
});
