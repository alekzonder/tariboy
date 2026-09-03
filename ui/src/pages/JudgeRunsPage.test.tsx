import { afterEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Outlet, Route, Routes } from "react-router-dom";
import JudgeRunsPage, { compactCriteria } from "./JudgeRunsPage";

afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers(); });

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
  expect(await screen.findByText("Could not load judge runs: offline")).toHaveAttribute("role", "alert");
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

it("edits raw automation JSON and renders daemon validation diagnostics", async () => {
  const canonical = "{\n  \"schema_version\": 1\n}\n";
  const fetchMock = vi.fn().mockImplementation((_input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(_input);
    if (url.endsWith("/api/judge-automation/validate")) {
      return Promise.resolve(response({ ok: true, result: { diagnostics: [{ path: "/judge/workers", message: "must contain exactly two agents" }] } }));
    }
    if (url.endsWith("/api/judge-automation") && init?.method === "PUT") {
      return Promise.resolve(response({ ok: true, result: { revision: { revision: 2, hash: "h", canonical_json: canonical, created_at: "now" } } }));
    }
    if (url.endsWith("/api/judge-automation")) {
      return Promise.resolve(response({ ok: true, result: { configured: true, revision: { revision: 1, hash: "h", canonical_json: canonical, created_at: "now" } } }));
    }
    return Promise.resolve(response({ ok: true, result: { count: 0, runs: [] } }));
  });
  vi.stubGlobal("fetch", fetchMock);
  render(<MemoryRouter><JudgeRunsPage /></MemoryRouter>);

  const editor = await screen.findByLabelText("Judge automation JSON");
  await waitFor(() => expect(editor).toHaveValue(canonical));
  fireEvent.change(editor, { target: { value: "{bad" } });
  fireEvent.click(screen.getByRole("button", { name: "Validate" }));
  await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("/judge/workers"));
  fireEvent.change(editor, { target: { value: canonical } });
  fireEvent.click(screen.getByRole("button", { name: "Apply" }));
  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining("/api/judge-automation"), expect.objectContaining({ method: "PUT", body: expect.stringContaining("config_json") })));
});

it("queues an automation run from the selected Judge UI", async () => {
  const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
    const url = String(input);
    if (url.endsWith("/api/judge-automation/run-once")) return Promise.resolve(response({ ok: true, result: { queued: true } }));
    if (url.endsWith("/api/judge-automation")) return Promise.resolve(response({ ok: true, result: { configured: false } }));
    return Promise.resolve(response({ ok: true, result: { count: 0, runs: [] } }));
  });
  vi.stubGlobal("fetch", fetchMock);
  const target = { id: "remote", label: "Remote", baseURL: "https://remote.example", token: "secret" };
  render(<MemoryRouter><Routes><Route element={<Outlet context={target} />}><Route path="/" element={<JudgeRunsPage />} /></Route></Routes></MemoryRouter>);

  fireEvent.click(await screen.findByRole("button", { name: "Run once" }));

  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith("https://remote.example/api/judge-automation/run-once", expect.objectContaining({ method: "POST" })));
});

it("creates the missing judges team from the selected server's automation config", async () => {
  const canonical = JSON.stringify({ judge: { lead: "judge-lead", workers: ["judge-one", "judge-two"] } });
  let created = false;
  const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith("/api/judge-automation")) return Promise.resolve(response({ ok: true, result: { configured: true, revision: { revision: 1, hash: "h", canonical_json: canonical, created_at: "now" } } }));
    if (url.endsWith("/api/groups") && init?.method === "POST") { created = true; return Promise.resolve(response({ ok: true, result: { name: "judges" } })); }
    if (url.endsWith("/api/groups")) return Promise.resolve(response({ ok: true, result: { groups: created ? [{ name: "judges", lead: "judge-lead", members: 3 }] : [], count: created ? 1 : 0 } }));
    if (url.includes("/api/groups/judges/assign")) return Promise.resolve(response({ ok: true, result: { assigned: true } }));
    return Promise.resolve(response({ ok: true, result: { count: 0, runs: [] } }));
  });
  vi.stubGlobal("fetch", fetchMock);
  const target = { id: "remote", label: "Remote", baseURL: "https://remote.example", token: "secret" };
  render(<MemoryRouter><Routes><Route element={<Outlet context={target} />}><Route path="/" element={<JudgeRunsPage />} /></Route></Routes></MemoryRouter>);

  fireEvent.click(await screen.findByRole("button", { name: "Create judges team" }));

  await waitFor(() => expect(screen.queryByRole("button", { name: "Create judges team" })).not.toBeInTheDocument());
  const writes = fetchMock.mock.calls.filter(([, init]) => init?.method === "POST");
  expect(writes.map(([url, init]) => [url, JSON.parse(String(init?.body))])).toEqual([
    ["https://remote.example/api/groups", { name: "judges", lead: "judge-lead" }],
    ["https://remote.example/api/groups/judges/assign", { agent: "judge-lead" }],
    ["https://remote.example/api/groups/judges/assign", { agent: "judge-one" }],
    ["https://remote.example/api/groups/judges/assign", { agent: "judge-two" }],
  ]);
});

it("hides the stale create button while a newly selected server loads", async () => {
  const canonical = JSON.stringify({ judge: { lead: "judge-lead", workers: ["judge-one", "judge-two"] } });
  let resolveRemote!: (value: Response) => void;
  const remoteAutomation = new Promise<Response>((resolve) => { resolveRemote = resolve; });
  const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
    const url = String(input);
    if (url === "https://second.example/api/judge-automation") return remoteAutomation;
    if (url.endsWith("/api/judge-automation")) return Promise.resolve(response({ ok: true, result: { configured: true, revision: { revision: 1, hash: "h", canonical_json: canonical, created_at: "now" } } }));
    if (url.endsWith("/api/groups")) return Promise.resolve(response({ ok: true, result: { groups: [], count: 0 } }));
    return Promise.resolve(response({ ok: true, result: { count: 0, runs: [] } }));
  });
  vi.stubGlobal("fetch", fetchMock);
  const first = { id: "first", label: "First", baseURL: "https://first.example", token: "secret" };
  const second = { id: "second", label: "Second", baseURL: "https://second.example", token: "secret" };
  const view = (target: typeof first) => <MemoryRouter><Routes><Route element={<Outlet context={target} />}><Route path="/" element={<JudgeRunsPage />} /></Route></Routes></MemoryRouter>;
  const { rerender } = render(view(first));
  expect(await screen.findByRole("button", { name: "Create judges team" })).toBeInTheDocument();

  rerender(view(second));

  expect(screen.queryByRole("button", { name: "Create judges team" })).not.toBeInTheDocument();
  resolveRemote(response({ ok: true, result: { configured: false } }));
});

it("ignores create completion from the previously selected server", async () => {
  const canonical = JSON.stringify({ judge: { lead: "judge-lead", workers: ["judge-one", "judge-two"] } });
  let finishCreate!: (value: Response) => void;
  const pendingCreate = new Promise<Response>((resolve) => { finishCreate = resolve; });
  const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url === "https://first.example/api/groups" && init?.method === "POST") return pendingCreate;
    if (url.endsWith("/api/judge-automation")) return Promise.resolve(response({ ok: true, result: { configured: true, revision: { revision: 1, hash: "h", canonical_json: canonical, created_at: "now" } } }));
    if (url.endsWith("/api/groups")) return Promise.resolve(response({ ok: true, result: { groups: [], count: 0 } }));
    if (url.includes("/api/groups/judges/assign")) return Promise.resolve(response({ ok: true, result: { assigned: true } }));
    return Promise.resolve(response({ ok: true, result: { count: 0, runs: [] } }));
  });
  vi.stubGlobal("fetch", fetchMock);
  const first = { id: "first", label: "First", baseURL: "https://first.example", token: "secret" };
  const second = { id: "second", label: "Second", baseURL: "https://second.example", token: "secret" };
  const view = (target: typeof first) => <MemoryRouter><Routes><Route element={<Outlet context={target} />}><Route path="/" element={<JudgeRunsPage />} /></Route></Routes></MemoryRouter>;
  const { rerender } = render(view(first));
  fireEvent.click(await screen.findByRole("button", { name: "Create judges team" }));

  rerender(view(second));
  expect(await screen.findByRole("button", { name: "Create judges team" })).toBeInTheDocument();
  finishCreate(response({ ok: true, result: { name: "judges" } }));

  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith("https://first.example/api/groups/judges/assign", expect.objectContaining({ method: "POST" })));
  expect(screen.getByRole("button", { name: "Create judges team" })).toBeInTheDocument();
});
