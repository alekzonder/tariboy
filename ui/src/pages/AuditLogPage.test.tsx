import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import AuditLogPage from "./AuditLogPage";
import { AgentNameContext } from "@/lib/agent";

// A "today" started_at (built from the current local date) must render as HH:MM
// only; a fixed different-year one must render as YYYY-MM-DD HH:MM (see
// ui/src/lib/time.ts).
const pad = (n: number) => String(n).padStart(2, "0");
const now = new Date();
const TODAY_ISO = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}T09:05:00`;
const OLD_ISO = "2020-01-02T03:04:00";
const TODAY_ID = "dev-worker-20260709065606-3"; // shortIter → "065606-3"
const OLD_ID = "dev-worker-20200102030400-1";

// Backend returns oldest-first; the page must sort newest-first for display.
// Statuses/states are production-faithful: Status is only
// running|done|no_i_am_done|harness_error|timeout|killed, and productive=false
// is only reachable on a done|no_i_am_done row (engine.go:445 — only those call
// SetIterationDone; failure states leave productive at its DEFAULT 1). OLD_ID is
// thus a done+idle iteration; TODAY_ID a done+productive one.
const ITERS = [
  { id: OLD_ID, trigger: "manual", status: "done", started_at: OLD_ISO, done: true, productive: false, judge: { latest_completed: null, active: null } },
  { id: TODAY_ID, trigger: "manual", status: "done", started_at: TODAY_ISO, done: true, productive: true, judge: { latest_completed: { run_id: "run", target_id: "target", created_at: TODAY_ISO, state: "done", verdict: "pass", score: 0, completed: 1, failed: 0, pending: 0 }, active: null } },
];
const mkEvent = (trigger: string, iter: string) => [
  { seq: 1, kind: "iteration_started", source: "system", at: "t1", data: JSON.stringify({ trigger }), iteration_id: iter },
];
const FULL_EVENTS = mkEvent("fulldefault", "x");
const ITER_EVENTS = mkEvent("iterpane", TODAY_ID);

let urls: string[] = [];
beforeEach(() => {
  urls = [];
  vi.stubGlobal("EventSource", class { addEventListener() {} removeEventListener() {} close() {} } as unknown as typeof EventSource);
  vi.stubGlobal("fetch", vi.fn().mockImplementation((url: string) => {
    urls.push(url);
    let result: unknown = { events: [] };
    if (url.includes("transcript")) result = { calls: [] };
    else if (url.includes("logs?iteration=")) result = { events: ITER_EVENTS };
    else if (url.includes("/iterations")) result = { iterations: ITERS, count: ITERS.length };
    else if (url.includes("logs")) result = { events: FULL_EVENTS };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response);
  }));
});
afterEach(() => vi.restoreAllMocks());

// route defaults to "/"; pass "?iteration=<id>" to exercise the preselection.
const renderPage = (route = "/") =>
  render(
    <MemoryRouter initialEntries={[route]}>
      <AgentNameContext.Provider value="dev-worker">
        <AuditLogPage />
      </AgentNameContext.Provider>
    </MemoryRouter>,
  );

function NavigationProbe() {
  const location = useLocation();
  const navigate = useNavigate();
  return <><output data-testid="location">{location.search}</output><button onClick={() => navigate("?keep=yes&iteration=does-not-exist")}>external</button><button onClick={() => navigate(-1)}>back</button></>;
}

const renderRoutedPage = (route: string) => render(
  <MemoryRouter initialEntries={[route]}>
    <AgentNameContext.Provider value="dev-worker">
      <Routes><Route path="/agents/:hostId/:agent/activity" element={<><NavigationProbe /><AuditLogPage /></>} /></Routes>
    </AgentNameContext.Provider>
  </MemoryRouter>,
);

describe("AuditLogPage (merged iterations + audit log)", () => {
  it("renders the Full log in the right pane by default", async () => {
    renderPage();
    // FullAuditLog's initial "logs" fetch renders its events (trigger chip).
    await waitFor(() => expect(screen.getByText("fulldefault")).toBeInTheDocument());
    // The "Full log" entry is the selected (highlighted) row.
    const full = screen.getByText("Full log").closest("button")!;
    expect(full.className).toContain("bg-accent");
  });

  it("lists iterations newest-first, formatting today as HH:MM and a different year as YYYY-MM-DD HH:MM", async () => {
    renderPage();
    await waitFor(() => expect(screen.getByText("09:05")).toBeInTheDocument());
    expect(screen.getByText("2020-01-02 03:04")).toBeInTheDocument();
    // DOM order of the left column: Full log, then newest (today) → oldest.
    const buttons = screen.getAllByRole("button");
    expect(buttons[0].textContent).toContain("Full log");
    expect(buttons[1].textContent).toContain("09:05");
    expect(buttons[2].textContent).toContain("2020-01-02 03:04");
  });

  it("renders an 'idle' badge on non-productive iterations only (productive=false)", async () => {
    renderPage();
    await waitFor(() => expect(screen.getByText("2020-01-02 03:04")).toBeInTheDocument());
    // Exactly one idle badge: on the old (productive=false) row, not the today
    // (productive=true) row.
    const idle = screen.getAllByText("idle");
    expect(idle).toHaveLength(1);
    const oldRow = screen.getByText("2020-01-02 03:04").closest("button")!;
    const todayRow = screen.getByText("09:05").closest("button")!;
    expect(oldRow).toContainElement(idle[0]);
    expect(todayRow.textContent).not.toContain("idle");
  });

  it("switches the right pane to the single-iteration log when an iteration is clicked", async () => {
    renderPage();
    await waitFor(() => expect(screen.getByText("09:05")).toBeInTheDocument());
    fireEvent.click(screen.getByText("09:05").closest("button")!);
    // Right pane is now IterationAuditLog: it fetches logs?iteration=<id> and
    // shows the iteration header chip (condensed id · status from the row).
    await waitFor(() => expect(urls.some((u) => u.includes(`logs?iteration=${TODAY_ID}`))).toBe(true));
    await waitFor(() => expect(screen.getByText(/065606-3 · done/)).toBeInTheDocument());
    expect(screen.getByText("iterpane")).toBeInTheDocument();
  });

  it("preselects the iteration named by the ?iteration= query param (deep link from the Usage tab)", async () => {
    renderPage(`/agent/dev-worker/logs?iteration=${TODAY_ID}`);
    // Right pane opens directly on that iteration (not Full log): its log fetch
    // and header chip appear without any click, and once items load the
    // preselection survives the pruning-fallback effect.
    await waitFor(() => expect(urls.some((u) => u.includes(`logs?iteration=${TODAY_ID}`))).toBe(true));
    await waitFor(() => expect(screen.getByText(/065606-3 · done/)).toBeInTheDocument());
    expect(screen.getByText("iterpane")).toBeInTheDocument();
    // Full log is NOT the selected row. Match the standalone selected token, not
    // the always-present hover:bg-accent, which also contains "bg-accent".
    const full = screen.getByText("Full log").closest("button")!;
    expect(full.className).not.toMatch(/(^|\s)bg-accent($|\s)/);
  });

  it("shows an explicit missing state when ?iteration= names an unknown iteration", async () => {
    renderPage("/agent/dev-worker/logs?iteration=does-not-exist");
    await waitFor(() => expect(screen.getByText("Iteration does-not-exist was not found.")).toBeInTheDocument());
    const full = screen.getByText("Full log").closest("button")!;
    expect(full.className).not.toMatch(/(^|\s)bg-accent($|\s)/);
  });

  it("renders zero score and verdict in the iteration row", async () => {
    renderPage();
    const row = (await screen.findByText("09:05")).closest("button")!;
    expect(row).toHaveTextContent("0");
    expect(row).toHaveTextContent("pass");
  });

  it("keeps selection in the URL, preserves other params, and follows external and back navigation", async () => {
    renderRoutedPage("/agents/local/dev-worker/activity?keep=yes");
    fireEvent.click(await screen.findByText("09:05"));
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent(`keep=yes&iteration=${TODAY_ID}`));

    fireEvent.click(screen.getByRole("button", { name: "external" }));
    expect(await screen.findByText("Iteration does-not-exist was not found.")).toBeInTheDocument();
    expect(screen.queryByText("fulldefault")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "back" }));
    await waitFor(() => expect(screen.getByText(/065606-3 · done/)).toBeInTheDocument());
  });
});
