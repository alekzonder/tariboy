import { afterEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import JudgeRunDetailPage from "./JudgeRunDetailPage";

afterEach(() => vi.restoreAllMocks());

const result = { run: { id: "r 1", status: "partial", original_request: "Check safety", judge_group: "judges", judges_per_iteration: 1, max_attempts: 2, targets_ready: 1, targets_total: 1, assignments_completed: 1, assignments_total: 1, current_summary_version: 1, last_error: "", judge_agents: ["judge-a"] }, targets: [{ id: "t1", iteration: "i1", agent: "worker", sequence: 0, target_state: "done", consensus_verdict: "pass", assignments_completed: 1, assignments_failed: 0, assignments_pending: 0 }], analyses: [{ id: "a1", target_id: "t1", judge_agent: "judge-a", result: { verdict: "pass", score: 0.9, confidence: 0.8, summary: "Good", violations: [{ description: "Evidence", citations: [{ artifact: "audit", locator: "12" }] }] } }], summaries: [{ id: "s1", version: 1, summary_agent: "judge-lead", result: { executive_conclusion: "Approved" } }], usage: [{ iteration: "i1", requests: 1, input_tokens: 2, output_tokens: 3, cache_write_tokens: 0, cache_read_tokens: 0, cost_usd: 0.01 }] };

it("shows detail, retrieves immutable evidence, and retries after confirmation", async () => {
  const fetchMock = vi.fn().mockImplementation((url: string, init?: RequestInit) => {
    const body = url.includes("evidence") ? { evidence: { text: "immutable" } } : init?.method === "POST" ? { id: "r 1", retried: true } : result;
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: body }) } as Response);
  });
  vi.stubGlobal("fetch", fetchMock);
  render(<MemoryRouter initialEntries={["/judges/r%201"]}><Routes><Route path="/judges/:id" element={<JudgeRunDetailPage />} /></Routes></MemoryRouter>);
  await screen.findByText("Check safety");
  expect(screen.getByText("Version 1")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "[audit:12]" }));
  await screen.findByText(/Immutable evidence/);
  fireEvent.click(screen.getByRole("button", { name: "Retry failed work" }));
  fireEvent.click(screen.getByRole("button", { name: /^Retry$/ }));
  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith("/api/judges/r%201/retry", expect.objectContaining({ method: "POST" })));
});

it("renders without white-screening when judge_agents is undefined", async () => {
  const run = { ...result.run, model: undefined, judge_agents: undefined, original_request: undefined };
  const body = { ...result, run };
  const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: body }) } as Response);
  vi.stubGlobal("fetch", fetchMock);
  render(<MemoryRouter initialEntries={["/judges/r%201"]}><Routes><Route path="/judges/:id" element={<JudgeRunDetailPage />} /></Routes></MemoryRouter>);
  // Detail page mounts and shows the run id instead of crashing on undefined judge_agents.
  await screen.findByText("r 1");
  expect(screen.getByText("Approved")).toBeInTheDocument();
});
