import { afterEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import JudgeRunsPage, { compactCriteria } from "./JudgeRunsPage";

afterEach(() => vi.restoreAllMocks());

function response(result: unknown, ok = true): Response {
  return { ok, status: ok ? 200 : 500, text: async () => JSON.stringify(result) } as Response;
}

describe("compactCriteria", () => {
  it("returns the placeholder for undefined without throwing", () => {
    expect(compactCriteria(undefined)).toBe("—");
  });
  it("returns the placeholder for an empty string", () => {
    expect(compactCriteria("")).toBe("—");
  });
  it("collapses whitespace onto one line", () => {
    expect(compactCriteria("a\n  b")).toBe("a b");
  });
});

it("renders the table when a run has undefined criteria", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response({ ok: true, result: { count: 1, runs: [{
    id: "run 9", status: "running", targets_ready: 0, targets_total: 1, assignments_completed: 0, assignments_total: 1,
  }] } })));
  render(<MemoryRouter><JudgeRunsPage /></MemoryRouter>);

  await waitFor(() => expect(screen.getByText("running")).toBeInTheDocument());
  expect(screen.getByRole("link", { name: "—" })).toHaveAttribute("href", "/settings/advanced/judges/run%209");
});

it("displays judge run progress, status, model, cost, and creator", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response({ ok: true, result: { count: 1, runs: [{
    id: "run 1", original_request: "Assess implementation quality", judge_agents: ["claude-opus"], lead_agent: "judge-lead",
    status: "partial", targets_ready: 2, targets_total: 3, assignments_completed: 4, assignments_total: 6, cost_usd: 1.25,
  }] } })));
  render(<MemoryRouter><JudgeRunsPage /></MemoryRouter>);

  await waitFor(() => expect(screen.getByText("Assess implementation quality")).toBeInTheDocument());
  expect(screen.getByText("2/3")).toBeInTheDocument();
  expect(screen.getByText("4/6")).toBeInTheDocument();
  expect(screen.getByText("partial")).toBeInTheDocument();
  expect(screen.getByText("claude-opus")).toBeInTheDocument();
  expect(screen.getByText("$1.2500")).toBeInTheDocument();
  expect(screen.getByText("judge-lead")).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "Assess implementation quality" })).toHaveAttribute("href", "/settings/advanced/judges/run%201");
});

it("shows empty and error states", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response({ ok: true, result: { count: 0, runs: [] } })));
  const { unmount } = render(<MemoryRouter><JudgeRunsPage /></MemoryRouter>);
  await waitFor(() => expect(screen.getByText("No judge runs yet.")).toBeInTheDocument());
  unmount();

  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response({ ok: false, error: { code: "offline", message: "offline" } }, false)));
  render(<MemoryRouter><JudgeRunsPage /></MemoryRouter>);
  await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("Could not load judge runs: offline"));
});

it("stops polling when unmounted", async () => {
  vi.useFakeTimers();
  const fetchMock = vi.fn().mockResolvedValue(response({ ok: true, result: { count: 0, runs: [] } }));
  vi.stubGlobal("fetch", fetchMock);
  const { unmount } = render(<MemoryRouter><JudgeRunsPage /></MemoryRouter>);
  await act(async () => { await Promise.resolve(); });
  const callsBeforeUnmount = fetchMock.mock.calls.length;
  expect(callsBeforeUnmount).toBeGreaterThan(0);
  unmount();
  await act(async () => { await vi.advanceTimersByTimeAsync(10_000); });
  expect(fetchMock).toHaveBeenCalledTimes(callsBeforeUnmount);
});
