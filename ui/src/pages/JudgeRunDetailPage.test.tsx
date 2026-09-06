import { afterEach, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, MemoryRouter, Outlet, Route, RouterProvider, Routes } from "react-router-dom";
import JudgeRunDetailPage from "./JudgeRunDetailPage";

afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers(); });

const result = { run: { id: "r 1", status: "partial", original_request: "Check safety", judge_group: "judges", judges_per_iteration: 1, max_attempts: 2, targets_ready: 1, targets_total: 1, assignments_completed: 1, assignments_total: 1, current_summary_version: 1, last_error: "", judge_agents: ["judge-a"] }, targets: [{ id: "t1", iteration: "i1", agent: "worker", sequence: 0, target_state: "done", consensus_verdict: "pass", assignments_completed: 1, assignments_failed: 0, assignments_pending: 0 }], analyses: [{ id: "a1", target_id: "t1", judge_agent: "judge-a", result: { verdict: "pass", score: 0.9, confidence: 0.8, summary: "Good", violations: [{ description: "Evidence", citations: [{ artifact: "audit", locator: "12" }] }] } }], summaries: [{ id: "s1", version: 1, summary_agent: "judge-lead", result: { executive_conclusion: "Approved" } }], improvements: [{ id: "proposal-1", revision_hash: "sha256:revision", status: "awaiting_plan_approval", draft: { target: { repository: "images", base_commit: "91ab820" }, findings: [], changes: [], acceptance: [], subject_ids: [], risk: "low", rollback_image: "reviewer:v7" } }], usage: [{ iteration: "i1", requests: 1, input_tokens: 2, output_tokens: 3, cache_write_tokens: 0, cache_read_tokens: 0, cost_usd: 0.01 }] };

it("shows detail, retrieves immutable evidence, and retries after confirmation", async () => {
  const fetchMock = vi.fn().mockImplementation((url: string, init?: RequestInit) => {
    const body = url.includes("evidence") ? { evidence: { text: "immutable" } } : init?.method === "POST" ? { id: "r 1", retried: true } : result;
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: body }) } as Response);
  });
  vi.stubGlobal("fetch", fetchMock);
  render(<MemoryRouter initialEntries={["/judges/r%201"]}><Routes><Route path="/judges/:id" element={<JudgeRunDetailPage />} /></Routes></MemoryRouter>);
  await screen.findByText("Check safety");
  expect(screen.getByText("Version 1")).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "proposal-1" })).toHaveAttribute("href", "/servers/local/settings/advanced/improvements/proposal-1");
  fireEvent.click(screen.getByRole("button", { name: "[audit:12]" }));
  await screen.findByText(/Immutable evidence/);
  fireEvent.click(screen.getByRole("button", { name: "Retry failed work" }));
  fireEvent.click(screen.getByRole("button", { name: /^Retry$/ }));
  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith("/api/judges/r%201/retry", expect.objectContaining({ method: "POST" })));
});

it("shows only the URL-selected target and links back to its iteration on the same host", async () => {
  const body = {
    ...result,
    targets: [
      result.targets[0],
      { id: "target two", iteration: "iteration/two", agent: "worker two", sequence: 1, target_state: "partial", consensus_verdict: "fail", consensus_score: 0.25, assignments_completed: 1, assignments_failed: 1, assignments_pending: 1 },
    ],
    analyses: [
      result.analyses[0],
      { id: "a2", target_id: "target two", judge_agent: "judge-b", result: { verdict: "fail", score: 0.2, confidence: 0.7, summary: "Second analysis", violations: [{ severity: "high", description: "Second violation", citations: [{ artifact: "audit", locator: "22" }] }], evidence_gaps: ["Missing terminal output"] } },
    ],
  };
  const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: String(input).includes("/evidence?") ? { evidence: { text: "second evidence" } } : body }) } as Response));
  vi.stubGlobal("fetch", fetchMock);
  const target = { id: "remote host", label: "Remote", baseURL: "https://remote.example", token: "secret" };
  render(<MemoryRouter initialEntries={["/servers/remote%20host/settings/advanced/judges/r%201?target=target%20two"]}><Routes><Route element={<Outlet context={target} />}><Route path="/servers/:hostId/settings/advanced/judges/:id" element={<JudgeRunDetailPage />} /></Route></Routes></MemoryRouter>);

  await screen.findByText("Second analysis");
  expect(screen.getByText("Consensus score 0.25")).toBeInTheDocument();
  expect(screen.getByText(/1 completed/)).toHaveTextContent("1 completed · 1 failed · 1 pending · fail");
  expect(screen.getByText((_, element) => element?.tagName === "P" && element.textContent === "judge-b: fail (0.20, confidence 0.70)")).toBeInTheDocument();
  expect(screen.getByText("Second violation")).toBeInTheDocument();
  expect(screen.getByText("Missing terminal output")).toBeInTheDocument();
  expect(screen.queryByText("Good")).not.toBeInTheDocument();
  expect(screen.queryByText("Approved")).not.toBeInTheDocument();
  expect(screen.queryByRole("link", { name: "proposal-1" })).not.toBeInTheDocument();
  expect(screen.getByRole("link", { name: "Judge runs" })).toHaveAttribute("href", "/servers/remote%20host/settings/advanced/judges");
  expect(screen.getByRole("link", { name: "worker two iteration iteration/two" })).toHaveAttribute("href", "/agents/remote%20host/worker%20two/activity?iteration=iteration%2Ftwo");
  expect(fetchMock).toHaveBeenCalledWith("https://remote.example/api/judges/r%201", expect.objectContaining({ method: "GET" }));
  fireEvent.click(screen.getByRole("button", { name: "[audit:22]" }));
  await screen.findByText(/second evidence/);
  expect(fetchMock).toHaveBeenCalledWith("https://remote.example/api/judges/r%201/targets/target%20two/evidence?artifact=audit&locator=22", expect.objectContaining({ method: "GET" }));
});

it("shows target not found instead of falling back to the whole run", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response));
  render(<MemoryRouter initialEntries={["/servers/local/settings/advanced/judges/r%201?target=missing"]}><Routes><Route path="/servers/:hostId/settings/advanced/judges/:id" element={<JudgeRunDetailPage />} /></Routes></MemoryRouter>);

  expect(await screen.findByRole("alert")).toHaveTextContent("Target missing was not found in judge run r 1.");
  expect(screen.queryByText("Good")).not.toBeInTheDocument();
});

it("restores the previous whole-run location after target remount and browser back", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response));
  const router = createMemoryRouter([{ path: "/servers/:hostId/settings/advanced/judges/:id", element: <JudgeRunDetailPage /> }], { initialEntries: ["/servers/local/settings/advanced/judges/r%201"] });
  const first = render(<RouterProvider router={router} />);
  await screen.findByText("Check safety");
  await act(() => router.navigate("/servers/local/settings/advanced/judges/r%201?target=t1"));
  await screen.findByText("Judge analysis for");

  first.unmount();
  render(<RouterProvider router={router} />);
  await screen.findByText("Judge analysis for");
  await act(() => router.navigate(-1));

  expect(await screen.findByText("Check safety")).toBeInTheDocument();
  expect(screen.getByText("Version 1")).toBeInTheDocument();
});

it("shows a target polling failure and clears it after a successful refresh", async () => {
  vi.useFakeTimers();
  const running = { ...result, run: { ...result.run, status: "running" } };
  let calls = 0;
  vi.stubGlobal("fetch", vi.fn().mockImplementation(() => {
    calls += 1;
    const envelope = calls === 2 ? { ok: false, error: { code: "offline", message: "temporarily offline" } } : { ok: true, result: running };
    return Promise.resolve({ ok: calls !== 2, status: calls === 2 ? 500 : 200, text: async () => JSON.stringify(envelope) } as Response);
  }));
  render(<MemoryRouter initialEntries={["/servers/local/settings/advanced/judges/r%201?target=t1"]}><Routes><Route path="/servers/:hostId/settings/advanced/judges/:id" element={<JudgeRunDetailPage />} /></Routes></MemoryRouter>);
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });

  await act(async () => { await vi.advanceTimersByTimeAsync(5_000); });
  expect(screen.getByRole("alert")).toHaveTextContent("Could not refresh judge run: temporarily offline");

  await act(async () => { await vi.advanceTimersByTimeAsync(5_000); });
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});

it("ignores a detail response from an obsolete descriptor for the same host", async () => {
  let resolveOld!: (value: Response) => void;
  const oldRequest = new Promise<Response>(resolve => { resolveOld = resolve; });
  const refreshed = { ...result, run: { ...result.run, original_request: "Refreshed detail" } };
  vi.stubGlobal("fetch", vi.fn().mockImplementation((input: RequestInfo | URL) => String(input).startsWith("https://old.example")
    ? oldRequest
    : Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: refreshed }) } as Response)));
  const route = <Route path="/servers/:hostId/settings/advanced/judges/:id" element={<JudgeRunDetailPage />} />;
  const view = (target: { id: string; label: string; baseURL: string; token: string }) => <MemoryRouter initialEntries={["/servers/remote/settings/advanced/judges/r%201"]}><Routes><Route element={<Outlet context={target} />}>{route}</Route></Routes></MemoryRouter>;
  const { rerender } = render(view({ id: "remote", label: "Remote", baseURL: "https://old.example", token: "old" }));

  rerender(view({ id: "remote", label: "Remote", baseURL: "https://new.example", token: "new" }));
  await screen.findByText("Refreshed detail");
  resolveOld({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response);
  await Promise.resolve();

  expect(screen.getByText("Refreshed detail")).toBeInTheDocument();
  expect(screen.queryByText("Check safety")).not.toBeInTheDocument();
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
