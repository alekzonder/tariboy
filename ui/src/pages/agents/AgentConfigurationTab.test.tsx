import { afterEach, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { AgentNameContext, AgentStatusContext } from "@/lib/agent";
import type { Daemon } from "@/lib/daemons";
import type { AgentImageStatus } from "@/lib/api";
import type { AgentStatus, AgentView } from "@/lib/types";
import { clearHarnessCatalogCache } from "@/hooks/useHarnessCatalog";
import { RUNTIME_PRESETS_STORAGE_KEY } from "@/lib/runtimePresets";
import AgentConfigurationTab from "./AgentConfigurationTab";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  clearHarnessCatalogCache();
  localStorage.clear();
});

const remote: Daemon = { id: "remote-1", label: "Remote", baseURL: "https://remote.example", token: "secret" };

const stopped: AgentView = {
  name: "worker",
  image: "worker:v1",
  digest: "sha256:abc",
  state: "stopped",
  cwd: "/srv/old",
  harness: "codex",
  model: "",
  effort: "",
  interactive: true,
  enabled: false,
  interval_s: 30,
  timeout_s: 60,
  hard_timeout_s: 120,
  on_timeout: "restart",
  on_error: "restart",
  max_idle_iterations: 0,
  ai_stall_timeout_s: 1800,
  messages_batch: 10,
  messages_max_queue: 100,
  user_prompt: "",
  env: {},
  plugins: [],
  goal_enabled: true,
  goal_wait_customer_timeout_s: 300,
  goal_delivery_cooldown_s: 60,
  current_goal_task_key: "",
  group: null,
  alias: "",
  notes: "",
  budget: {
    hour_usd: 1, day_usd: 0, week_usd: 0, month_usd: 0,
    hour_spent_usd: 0.25, day_spent_usd: 0.25, week_spent_usd: 0.25, month_spent_usd: 0.25,
    exhausted: [],
  },
};

const projection = {
  name: "worker",
  current: { ref: "worker:latest", digest: "old-digest", image_version: "1.0.0" },
  pending: { ref: "", digest: "", error: "" },
  next: { ref: "worker:latest", digest: "new-digest", image_version: "1.1.0", reason: "ref_moved" },
} satisfies AgentImageStatus;

function response(result: unknown, ok = true, status = 200) {
  const body = ok ? { ok: true, result } : { ok: false, error: result };
  return Promise.resolve({ ok, status, text: async () => JSON.stringify(body) } as Response);
}

type Call = { method: string; path: string; url: string; body: unknown; headers?: HeadersInit };

// The view field each loop/runtime/goal/message write lands in, so a reload
// after an accepted write reads the new value, exactly as a real daemon does.
const VIEW_FIELD: Record<string, [keyof AgentView, string]> = {
  "loop/interval": ["interval_s", "value"],
  "loop/timeout": ["timeout_s", "value"],
  "loop/hard-timeout": ["hard_timeout_s", "value"],
  "loop/on-timeout": ["on_timeout", "value"],
  "loop/on-error": ["on_error", "value"],
  "loop/max-idle": ["max_idle_iterations", "value"],
  "loop/ai-stall-timeout": ["ai_stall_timeout_s", "value"],
  model: ["model", "value"],
  effort: ["effort", "value"],
  harness: ["harness", "value"],
  interactive: ["interactive", "value"],
  cwd: ["cwd", "value"],
  "goal-enabled": ["goal_enabled", "enabled"],
  "goal-wait-customer-timeout": ["goal_wait_customer_timeout_s", "seconds"],
  "goal-delivery-cooldown": ["goal_delivery_cooldown_s", "seconds"],
  "messages/batch": ["messages_batch", "value"],
  "messages/max-queue": ["messages_max_queue", "value"],
};

// fakeServer answers every request the page makes from one mutable state.
// `fail` rejects a write by "METHOD rest" with the given message.
function fakeServer(opts: {
  view?: Partial<AgentView>;
  image?: () => unknown;
  images?: () => Promise<Response>;
  fail?: Record<string, string>;
} = {}) {
  const state = {
    view: { ...stopped, ...opts.view } as AgentView,
    pending: { ref: "", digest: "", error: "" },
    script: "echo hi\n",
    retention: { keep_iterations: 5, keep_days: 7, max_bytes: 1024, override: { keep_iterations: 5, keep_days: 0, max_bytes: 0 }, default: { keep_iterations: 10, keep_days: 7, max_bytes: 1024 } },
    secrets: ["API_KEY"],
  };
  const calls: Call[] = [];
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    const parsed = new URL(url, "http://local");
    const method = init?.method ?? "GET";
    const body = init?.body ? JSON.parse(String(init.body)) : undefined;
    calls.push({ method, path: parsed.pathname + parsed.search, url, body, headers: init?.headers });
    const agentPrefix = /^\/api\/agents\/[^/]+/.exec(parsed.pathname)?.[0];
    const agentRest = agentPrefix ? parsed.pathname.slice(agentPrefix.length).replace(/^\//, "") : null;
    const failure = agentRest !== null ? opts.fail?.[`${method} ${agentRest}`] : undefined;
    if (failure) return response({ code: "invalid", message: failure }, false, 400);
    if (parsed.pathname.startsWith("/api/fs/list")) return response({ path: "/srv", parent: "/", entries: [{ name: "new", dir: true }] });
    if (parsed.pathname === "/api/images") {
      return opts.images?.() ?? response({ images: [{ name: "worker", tag: "latest" }, { name: "reviewer", tag: "v2" }] });
    }
    if (parsed.pathname.startsWith("/api/harnesses/")) {
      return response({ harness: "codex", models: [{ id: "gpt-x" }], efforts: ["low", "high"], error: "" });
    }
    if (agentRest === null) return response({ code: "unexpected", message: `${method} ${url}` }, false, 500);
    if (agentRest === "") return response(state.view);
    if (agentRest === "image") {
      if (method === "POST") state.pending = { ref: body.image, digest: "sha256:v2", error: "" };
      if (method === "DELETE") state.pending = { ref: "", digest: "", error: "" };
      return response(opts.image?.() ?? { ...projection, pending: state.pending });
    }
    if (agentRest === "shell-script") {
      if (method === "POST") { state.script = body.script; return response({ saved: true }); }
      return response({ script: state.script });
    }
    if (agentRest === "retention") {
      if (method === "POST") state.retention = { ...state.retention, override: { ...state.retention.override, keep_iterations: body["keep-iterations"], keep_days: body["keep-days"] } };
      return response(state.retention);
    }
    if (agentRest === "secrets") {
      if (method === "POST") state.secrets = [...state.secrets, body.key];
      if (method === "DELETE") state.secrets = state.secrets.filter((key) => key !== parsed.searchParams.get("key"));
      return response({ keys: state.secrets, count: state.secrets.length });
    }
    if (method === "POST") {
      if (agentRest === "loop/schedule") state.view = { ...state.view, schedule: body.clear ? "" : body.value, schedule_next_at: body.clear ? "" : "2026-10-26T08:00:00Z" };
      else if (agentRest === "budget") state.view = { ...state.view, budget: { ...state.view.budget!, hour_usd: Number(body.hour_usd), day_usd: Number(body.day_usd), week_usd: Number(body.week_usd), month_usd: Number(body.month_usd) } };
      else if (VIEW_FIELD[agentRest]) {
        const [field, key] = VIEW_FIELD[agentRest];
        state.view = { ...state.view, [field]: body[key] };
      }
      return response({});
    }
    return response({ code: "unexpected", message: `${method} ${url}` }, false, 500);
  });
  vi.stubGlobal("fetch", fetchMock);
  const writes = () => calls.filter((c) => c.method !== "GET").map((c) => `${c.method} ${c.path.replace("/api/agents/worker/", "")}`);
  return { state, calls, writes, fetchMock };
}

function tree(target: Daemon | null = remote, name = "worker", status: AgentStatus | null = null, refresh = vi.fn()) {
  return (
    <MemoryRouter>
      <AgentNameContext.Provider value={name}>
        <AgentStatusContext.Provider value={{ status, refresh: async () => {} }}>
          <AgentConfigurationTab target={target} refresh={refresh} />
        </AgentStatusContext.Provider>
      </AgentNameContext.Provider>
    </MemoryRouter>
  );
}

const block = (title: string) => screen.getByRole("region", { name: title });
const queryBlock = (title: string) => screen.queryByRole("region", { name: title });
const search = (text: string) => fireEvent.change(screen.getByRole("searchbox", { name: "Search settings" }), { target: { value: text } });
const saveBar = () => screen.getByRole("region", { name: "Unsaved changes" });
const save = () => fireEvent.click(within(saveBar()).getByRole("button", { name: "Save" }));
const ready = () => screen.findByLabelText("Interval");

// --- The task scenario and the shared save mechanic --------------------------

it("finds one setting, changes it, finds another, changes it, and saves both with one Save", async () => {
  const server = fakeServer();
  render(tree());
  await ready();
  expect(screen.queryByRole("region", { name: "Unsaved changes" })).not.toBeInTheDocument();

  search("interval");
  fireEvent.change(within(block("Loop and timeouts")).getByLabelText("Interval"), { target: { value: "45" } });
  expect(saveBar()).toHaveTextContent("1 unsaved change in Loop and timeouts");

  search("budget");
  expect(queryBlock("Loop and timeouts")).not.toBeInTheDocument();
  fireEvent.change(within(block("Budgets (USD)")).getByLabelText("Day budget"), { target: { value: "5" } });
  expect(saveBar()).toHaveTextContent("2 unsaved changes in Loop and timeouts, Budgets (USD)");

  save();
  await waitFor(() => expect(saveBar()).toHaveTextContent("All changes saved"));
  expect(server.writes()).toEqual(["POST loop/interval", "POST budget"]);
  expect(server.calls.find((c) => c.path.endsWith("/budget"))?.body).toEqual({ hour_usd: "1", day_usd: "5", week_usd: "0", month_usd: "0" });
  expect(server.calls.every((c) => c.url.startsWith("https://remote.example/"))).toBe(true);
  expect(within(saveBar()).queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  search("");
  expect(screen.getByLabelText("Interval")).toHaveValue("45");
});

it("lays out in the shared narrow tab page", async () => {
  fakeServer();
  render(tree());
  await ready();
  const page = screen.getByTestId("tab-page");
  expect(page).toContainElement(screen.getByRole("searchbox", { name: "Search settings" }));
  expect(page).toHaveClass("max-w-5xl");
});

it("has no save buttons inside the blocks", async () => {
  fakeServer();
  render(tree());
  await ready();
  for (const region of screen.getAllByRole("region")) {
    expect(within(region).queryByRole("button", { name: /^save/i })).not.toBeInTheDocument();
  }
});

it("lets pop-up lists such as Model, Effort and Working directory overflow every block", async () => {
  fakeServer();
  render(tree());
  await ready();
  const blocks = document.querySelectorAll('[id^="config-block-"][role="region"]');
  expect(blocks.length).toBeGreaterThan(1);
  // The shared Card clips with overflow-hidden; jsdom has no layout, so check the class.
  for (const card of blocks) {
    expect(card).toHaveClass("overflow-visible");
    expect(card).not.toHaveClass("overflow-hidden");
  }
});

it("marks the changed field and counts changes on its block", async () => {
  fakeServer();
  render(tree());
  await ready();
  fireEvent.change(screen.getByLabelText("Timeout"), { target: { value: "90" } });
  const loop = block("Loop and timeouts");
  expect(within(loop).getByText("1 unsaved change")).toBeInTheDocument();
  expect(within(loop).getByText("Changed")).toBeInTheDocument();
  expect(within(screen.getByRole("navigation", { name: "Configuration blocks" })).getByRole("button", { name: /Loop and timeouts\s*1/ })).toBeInTheDocument();
});

it("resets every change on every block without writing", async () => {
  const server = fakeServer();
  render(tree());
  await ready();
  fireEvent.change(screen.getByLabelText("Interval"), { target: { value: "45" } });
  fireEvent.change(screen.getByLabelText("Hour budget"), { target: { value: "9" } });
  fireEvent.click(within(saveBar()).getByRole("button", { name: "Reset" }));
  expect(screen.getByLabelText("Interval")).toHaveValue("30");
  expect(screen.getByLabelText("Hour budget")).toHaveValue(1);
  expect(server.writes()).toEqual([]);
});

it("stops at a failed write, reports it at the field, and keeps the unsent changes", async () => {
  const server = fakeServer({ fail: { "POST cwd": "directory does not exist" } });
  render(tree());
  await ready();
  fireEvent.change(screen.getByLabelText("Working directory"), { target: { value: "/srv/missing" } });
  fireEvent.change(screen.getByLabelText("Interval"), { target: { value: "45" } });
  save();
  expect(await within(saveBar()).findByText(/Some changes were not saved/)).toBeInTheDocument();
  expect(server.writes()).toEqual(["POST cwd"]);
  const general = block("General");
  expect(within(general).getByRole("alert")).toHaveTextContent("directory does not exist");
  expect(screen.getByLabelText("Working directory")).toHaveValue("/srv/missing");
  expect(saveBar()).toHaveTextContent("2 unsaved changes in General, Loop and timeouts");
});

it("sends nothing while a change is invalid and shows the field even if search hid it", async () => {
  const server = fakeServer();
  render(tree());
  await ready();
  fireEvent.change(screen.getByLabelText("Goal delivery cooldown seconds"), { target: { value: "0" } });
  search("interval");
  fireEvent.change(screen.getByLabelText("Interval"), { target: { value: "45" } });
  save();
  const field = await screen.findByLabelText("Goal delivery cooldown seconds");
  expect(screen.getByRole("searchbox", { name: "Search settings" })).toHaveValue("");
  await waitFor(() => expect(field).toHaveFocus());
  expect(within(block("Goal")).getByRole("alert")).toHaveTextContent(/positive whole number/);
  expect(server.writes()).toEqual([]);
});

// --- Search ------------------------------------------------------------------

it("shows the whole block for a single matching setting and highlights that setting", async () => {
  fakeServer();
  render(tree());
  await ready();
  search("hard timeout");
  const loop = block("Loop and timeouts");
  expect(within(loop).getByLabelText("Interval")).toBeInTheDocument();
  expect(loop.querySelector('[data-field="hard-timeout"]')).toHaveAttribute("data-matched", "true");
  expect(loop.querySelector('[data-field="interval"]')).not.toHaveAttribute("data-matched");
  expect(queryBlock("Budgets (USD)")).not.toBeInTheDocument();
  expect(screen.getByText("1 of 8 blocks match")).toBeInTheDocument();
});

it("reports no matches and clears the search", async () => {
  fakeServer();
  render(tree());
  await ready();
  search("zzz");
  expect(screen.getByText("No settings match “zzz”")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
  expect(block("Runtime")).toBeInTheDocument();
});

it("shows only blocks with unsaved changes when Changed only is on", async () => {
  fakeServer();
  render(tree());
  await ready();
  fireEvent.change(screen.getByLabelText("Messages per iteration"), { target: { value: "20" } });
  fireEvent.click(screen.getByRole("button", { name: "Changed only (1)" }));
  expect(block("Messages")).toBeInTheDocument();
  expect(queryBlock("Runtime")).not.toBeInTheDocument();
});

// Layout classes are the only signal jsdom has for the page's padding and the
// search bar matching the white workspace panel, as in the reported screenshot.
it("pads the page and keeps the search bar flush with the cards on the panel color", async () => {
  fakeServer();
  const { container } = render(tree());
  await ready();
  expect(container.firstElementChild).toHaveClass("px-4", "pt-4", "pb-6");
  const bar = screen.getByRole("navigation", { name: "Configuration blocks" }).parentElement!;
  expect(bar).toHaveClass("bg-card/95");
  expect(bar).not.toHaveClass("bg-background/95", "-mx-1");
  const result = within(bar).getByRole("status");
  expect(result).toHaveClass("sr-only");
  search("interval");
  expect(result).not.toHaveClass("sr-only");
  expect(result).toHaveTextContent(/blocks match/);
});

// --- Run actions -------------------------------------------------------------

it("keeps Start, Stop and Restart as immediate actions in the page header", async () => {
  fakeServer({ view: { enabled: false } });
  render(tree());
  await ready();
  expect(screen.getByTestId("master-switch-state")).toHaveTextContent("Disabled");
  expect(screen.getByText("Run actions apply immediately.")).toBeInTheDocument();
  for (const name of ["Start", "Stop", "Restart"]) expect(screen.getByRole("button", { name })).toBeInTheDocument();
  for (const name of ["Kill", "Remove", "Exec"]) expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
});

it("has no Loop on/off switch: Start and Stop are the only switch", async () => {
  fakeServer();
  render(tree());
  await ready();
  const loop = block("Loop and timeouts");
  expect(within(loop).getByLabelText("Interval")).toBeInTheDocument();
  expect(screen.queryByRole("switch", { name: "Loop" })).not.toBeInTheDocument();
});

// --- Runtime -----------------------------------------------------------------

it("saves harness before model and remembers the model for that harness", async () => {
  const server = fakeServer();
  render(tree());
  await ready();
  fireEvent.change(screen.getByLabelText("Model"), { target: { value: "opus" } });
  fireEvent.change(screen.getByLabelText("Harness"), { target: { value: "claude" } });
  fireEvent.click(screen.getByRole("switch", { name: "Interactive (tmux TUI)" }));
  expect(server.writes()).toEqual([]);
  save();
  await waitFor(() => expect(server.writes()).toEqual(["POST harness", "POST model", "POST interactive"]));
  expect(server.calls.find((c) => c.path.endsWith("/interactive"))?.body).toEqual({ value: false });
  await waitFor(() => expect(localStorage.getItem(RUNTIME_PRESETS_STORAGE_KEY)).toContain("opus"));
});

it("offers the operator harnesses and keeps an agent's out-of-list harness", async () => {
  fakeServer({ view: { harness: "stub" } });
  render(tree());
  await ready();
  const options = within(screen.getByLabelText("Harness")).getAllByRole("option").map((o) => o.textContent);
  expect(options).toEqual(["claude", "codex", "opencode", "cursor", "stub"]);
});

// --- General: image, digest, working directory --------------------------------

it("selects a built image and applies it with Save", async () => {
  const server = fakeServer({ image: undefined });
  render(tree());
  fireEvent.click(await screen.findByRole("combobox", { name: "Agent image" }));
  fireEvent.click(await screen.findByRole("option", { name: "reviewer:v2" }));
  expect(server.writes()).toEqual([]);
  save();
  expect(await screen.findByText("Pending:")).toBeInTheDocument();
  expect(server.calls.find((c) => c.method === "POST" && c.path.endsWith("/image"))?.body).toEqual({ image: "reviewer:v2" });
  expect(within(screen.getByText("Pending:").parentElement!).getByRole("link", { name: "reviewer:v2" })).toHaveAttribute("href", "/servers/remote-1/images/reviewer/v2");
});

it("cancels an explicit pending image at once, outside Save", async () => {
  let state: AgentImageStatus = {
    ...projection,
    pending: { ref: "explicit:v2", digest: "explicit-digest", error: "" },
    next: { ref: "explicit:v2", digest: "explicit-digest", image_version: "2.0.0", reason: "pending" },
  };
  const server = fakeServer({ image: () => state });
  const original = server.fetchMock.getMockImplementation()!;
  server.fetchMock.mockImplementation((url: string, init?: RequestInit) => {
    if (url.endsWith("/image") && init?.method === "DELETE") state = projection;
    return original(url, init);
  });
  render(tree());
  fireEvent.click(await screen.findByRole("button", { name: "Cancel pending" }));
  expect(await screen.findByText("1.1.0")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Cancel pending" })).not.toBeInTheDocument();
  expect(server.writes()).toEqual(["DELETE image"]);
});

it("shows pinned Activated version and next digest even when source versions match", async () => {
  fakeServer({ image: () => ({ ...projection, next: { ...projection.next, image_version: "1.0.0" } }) });
  render(tree());
  const current = await screen.findByRole("group", { name: "Activated version" });
  const next = screen.getByRole("group", { name: "Next iteration" });
  expect(current).toHaveTextContent("old-digest");
  expect(next).toHaveTextContent("new-digest");
  expect(within(next).getByRole("link", { name: "worker:latest" })).toHaveAttribute("href", "/servers/remote-1/images/worker/latest");
  expect(screen.getByText("Update on next iteration")).toBeInTheDocument();
  expect(screen.getByText(/preview.*not a reservation/i)).toBeInTheDocument();
});

it("shows inspection and activation failures without a pending ref", async () => {
  fakeServer({ image: () => ({
    ...projection,
    current: { ...projection.current, image_version: undefined },
    next: { ref: "worker:latest", digest: "", reason: "ref_moved", error: "latest is unreadable" },
    pending: { ref: "", digest: "", error: "bridge preparation failed" },
  }) });
  render(tree());
  expect(await screen.findByText("latest is unreadable")).toBeInTheDocument();
  expect(screen.getByText("bridge preparation failed")).toBeInTheDocument();
  expect(within(screen.getByRole("group", { name: "Activated version" })).getByText("Version not specified")).toBeInTheDocument();
});

it("reports an unavailable image endpoint and disables the image choice", async () => {
  const server = fakeServer();
  const original = server.fetchMock.getMockImplementation()!;
  server.fetchMock.mockImplementation((url: string, init?: RequestInit) =>
    url.endsWith("/image") ? Promise.reject(new Error("image request failed")) : original(url, init));
  render(tree());
  expect(await screen.findByText(/Image status unavailable:.*image request failed/)).toBeInTheDocument();
  expect(screen.queryByRole("group", { name: "Next iteration" })).not.toBeInTheDocument();
});

it("disables the image choice while the inventory failed", async () => {
  fakeServer({ images: () => response({ code: "internal", message: "archive is corrupt" }, false, 500) });
  render(tree());
  expect(await screen.findByText(/Image inventory unavailable:/)).toHaveTextContent("archive is corrupt");
  expect(screen.getByRole("combobox", { name: "Agent image" })).toBeDisabled();
});

it.each(["host", "agent", "refresh"])("ignores delayed image responses after a %s change", async (change) => {
  let finish!: (value: Response) => void;
  let count = 0;
  const fresh = { ...projection, next: { ...projection.next, image_version: "2.0.0" } };
  const server = fakeServer({ image: () => fresh });
  const original = server.fetchMock.getMockImplementation()!;
  server.fetchMock.mockImplementation((url: string, init?: RequestInit) => {
    if (url.endsWith("/image") && count++ === 0) return new Promise<Response>((resolve) => { finish = resolve; });
    return original(url, init);
  });
  const { rerender } = render(tree());
  await waitFor(() => expect(finish).toBeDefined());
  if (change === "refresh") act(() => window.dispatchEvent(new Event("tariboy:image-built")));
  else rerender(tree(change === "host" ? { ...remote, id: "remote-2", baseURL: "https://other.example" } : remote, change === "agent" ? "other" : "worker"));
  expect(await screen.findByText("2.0.0")).toBeInTheDocument();
  await act(async () => finish(await response(projection)));
  expect(screen.getByText("2.0.0")).toBeInTheDocument();
  expect(screen.queryByText("1.1.0")).not.toBeInTheDocument();
});

it("truncates the image digest while exposing the complete value, and copies it", async () => {
  const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.assign(navigator, { clipboard: { writeText } });
  fakeServer({ view: { digest } });
  render(tree());
  const shown = await screen.findByTestId("image-digest");
  expect(shown.textContent).toContain("sha256:0123");
  expect(shown).toHaveAttribute("aria-hidden", "true");
  expect(screen.getByText(digest)).toHaveClass("sr-only");
  fireEvent.click(screen.getByRole("button", { name: "Copy digest" }));
  await waitFor(() => expect(writeText).toHaveBeenCalledWith(digest));
  expect(await screen.findByText("Digest copied")).toBeInTheDocument();
});

it("edits a stopped agent's working directory with suggestions from its route host", async () => {
  const refresh = vi.fn();
  const server = fakeServer();
  render(tree(remote, "worker", null, refresh));
  const input = await screen.findByLabelText("Working directory");
  expect(input).toHaveValue("/srv/old");
  fireEvent.change(input, { target: { value: "/srv/new" } });
  expect(await screen.findByRole("option", { name: "new" })).toBeInTheDocument();
  fireEvent.change(input, { target: { value: "  /srv/new  " } });
  save();
  await waitFor(() => expect(refresh).toHaveBeenCalled());
  const write = server.calls.find((c) => c.method === "POST" && c.path.endsWith("/cwd"))!;
  expect(write.body).toEqual({ value: "/srv/new" });
  expect(write.headers).toMatchObject({ Authorization: "Bearer secret" });
  await waitFor(() => expect(screen.getByLabelText("Working directory")).toHaveValue("/srv/new"));
});

it("keeps the working directory read-only while the agent is active", async () => {
  fakeServer({ view: { state: "running", enabled: true } });
  render(tree());
  expect(await screen.findByText("/srv/old")).toBeInTheDocument();
  expect(screen.queryByLabelText("Working directory")).not.toBeInTheDocument();
  expect(screen.getByText("Stop the agent before changing its working directory.")).toBeInTheDocument();
});

it("switches to read-only when live status reports that the agent started", async () => {
  fakeServer();
  render(tree(remote, "worker", { name: "worker", state: "running" } as AgentStatus));
  expect(await screen.findByText("/srv/old")).toBeInTheDocument();
  expect(screen.queryByLabelText("Working directory")).not.toBeInTheDocument();
});

it("keeps drafts when the route host is re-rendered or a reconnect fails", async () => {
  const server = fakeServer();
  const { rerender } = render(tree());
  const input = await screen.findByLabelText("Working directory");
  fireEvent.change(input, { target: { value: "/srv/new" } });
  rerender(tree({ ...remote }));
  await act(async () => {});
  expect(server.calls.filter((c) => c.url === "https://remote.example/api/agents/worker")).toHaveLength(1);
  rerender(tree({ ...remote, baseURL: "" }));
  const unreachable = vi.fn().mockRejectedValue(new TypeError("Failed to fetch"));
  vi.stubGlobal("fetch", unreachable);
  rerender(tree({ ...remote, baseURL: "https://moved.example" }));
  await waitFor(() => expect(unreachable).toHaveBeenCalled());
  await act(async () => {});
  expect(screen.getByLabelText("Working directory")).toHaveValue("/srv/new");
  expect(screen.getByText("worker:v1")).toBeInTheDocument();
});

it("refreshes the image projection on status and build events without replacing drafts", async () => {
  let state: AgentImageStatus = projection;
  fakeServer({ image: () => state });
  const { rerender } = render(tree());
  await screen.findByRole("group", { name: "Next iteration" });
  fireEvent.change(await screen.findByLabelText("Working directory"), { target: { value: "/srv/draft" } });
  state = { ...projection, next: { ...projection.next, image_version: "1.2.0" } };
  rerender(tree(remote, "worker", { name: "worker", state: "stopped" } as AgentStatus));
  expect(await screen.findByText("1.2.0")).toBeInTheDocument();
  state = { ...projection, next: { ...projection.next, image_version: "1.3.0" } };
  act(() => window.dispatchEvent(new Event("tariboy:image-built")));
  expect(await screen.findByText("1.3.0")).toBeInTheDocument();
  expect(screen.getByLabelText("Working directory")).toHaveValue("/srv/draft");
});

it("creates a missing working directory only when its switch is on", async () => {
  const refresh = vi.fn();
  const server = fakeServer();
  render(tree(remote, "worker", null, refresh));
  const input = await screen.findByLabelText("Working directory");
  const create = screen.getByRole("switch", { name: "Create directory if missing" });
  expect(create).not.toBeChecked();
  fireEvent.change(input, { target: { value: "/srv/new" } });
  fireEvent.click(create);
  save();
  await waitFor(() => expect(refresh).toHaveBeenCalled());
  const write = server.calls.find((c) => c.method === "POST" && c.path.endsWith("/cwd"))!;
  expect(write.body).toEqual({ value: "/srv/new", create: true });
  await waitFor(() => expect(screen.getByRole("switch", { name: "Create directory if missing" })).not.toBeChecked());
});

it("hides the create directory switch while the agent is active", async () => {
  fakeServer({ view: { state: "running", enabled: true } });
  render(tree());
  expect(await screen.findByText("/srv/old")).toBeInTheDocument();
  expect(screen.queryByRole("switch", { name: "Create directory if missing" })).not.toBeInTheDocument();
});

// --- Goal, budgets -----------------------------------------------------------

it("renders the current Goal task read-only with its help", async () => {
  fakeServer({ view: { current_goal_task_key: "DEV-7" } });
  render(tree());
  expect(await screen.findByLabelText("Current goal task")).toHaveValue("DEV-7");
  expect(screen.getByLabelText("Current goal task")).toBeDisabled();
});

it("shows spend beside each budget and exhausted periods", async () => {
  fakeServer({ view: { budget: { ...stopped.budget!, exhausted: ["day"] } } });
  render(tree());
  await ready();
  const budgets = block("Budgets (USD)");
  expect(within(budgets).getByText(/Spent 0.25 of 1.00/)).toBeInTheDocument();
  expect(within(budgets).getAllByText(/Unlimited/).length).toBeGreaterThan(0);
  expect(within(budgets).getByText("Out of budget: day")).toBeInTheDocument();
});

it("renders a budget projection from an older daemon without exhausted periods", async () => {
  const legacy = { ...stopped.budget! };
  delete legacy.exhausted;
  fakeServer({ view: { budget: legacy } });
  render(tree());
  await ready();
  expect(within(block("Budgets (USD)")).queryByText(/Out of budget/)).not.toBeInTheDocument();
});

it("leaves out the budget block when the daemon has no budget projection", async () => {
  fakeServer({ view: { budget: undefined } });
  render(tree());
  await ready();
  expect(queryBlock("Budgets (USD)")).not.toBeInTheDocument();
  search("x-no-match");
  expect(screen.getByText("0 of 7 blocks match")).toBeInTheDocument();
});

// --- Environment ---------------------------------------------------------------

it("saves the agent shell script with Save", async () => {
  const server = fakeServer();
  render(tree());
  const script = await screen.findByLabelText("Agent shell script");
  await waitFor(() => expect(script).toHaveValue("echo hi\n"));
  fireEvent.change(script, { target: { value: "export A=1\n" } });
  save();
  await waitFor(() => expect(server.writes()).toEqual(["POST shell-script"]));
  expect(server.state.script).toBe("export A=1\n");
});

it("adds and removes secrets on Save, never shows a value, and clears it after saving", async () => {
  const server = fakeServer();
  render(tree());
  const env = await screen.findByRole("region", { name: "Environment" });
  await within(env).findByText("API_KEY");
  fireEvent.click(within(env).getByRole("button", { name: "Remove API_KEY" }));
  expect(within(env).getByText("Will be removed")).toBeInTheDocument();
  fireEvent.change(within(env).getByLabelText("Secret key"), { target: { value: "TOKEN" } });
  fireEvent.change(within(env).getByLabelText("Secret value"), { target: { value: "s3cr3t" } });
  expect(screen.queryByText("s3cr3t")).toBeNull();
  expect(server.writes()).toEqual([]);
  save();
  await waitFor(() => expect(server.writes()).toEqual(["DELETE secrets?key=API_KEY", "POST secrets"]));
  expect(server.calls.find((c) => c.method === "POST" && c.path.endsWith("/secrets"))?.body).toEqual({ key: "TOKEN", value: "s3cr3t" });
  expect(server.calls.some((c) => c.url.includes("s3cr3t"))).toBe(false);
  await waitFor(() => expect(within(env).getByLabelText("Secret value")).toHaveValue(""));
  expect(within(env).getByText("TOKEN")).toBeInTheDocument();
  expect(within(env).queryByText("API_KEY")).not.toBeInTheDocument();
});

it("requires a secret key when a value is entered", async () => {
  const server = fakeServer();
  render(tree());
  const env = await screen.findByRole("region", { name: "Environment" });
  await within(env).findByText("API_KEY");
  fireEvent.change(within(env).getByLabelText("Secret value"), { target: { value: "s3cr3t" } });
  save();
  expect(await within(env).findByText("Enter a secret key.")).toBeInTheDocument();
  expect(server.writes()).toEqual([]);
});

// --- Retention -------------------------------------------------------------------

it("saves the retention override with Save and shows the server defaults", async () => {
  const server = fakeServer();
  render(tree());
  const retention = await screen.findByRole("region", { name: "Retention and cleanup" });
  const days = await within(retention).findByLabelText("Keep days");
  expect(within(retention).getByText(/0 = server default \(7\)/)).toBeInTheDocument();
  fireEvent.change(days, { target: { value: "14" } });
  save();
  await waitFor(() => expect(server.writes()).toEqual(["POST retention"]));
  expect(server.calls.find((c) => c.path.endsWith("/retention") && c.method === "POST")?.body).toEqual({ "keep-iterations": 5, "keep-days": 14 });
});

it("keeps prune and preview as confirmed, immediate actions outside Save", async () => {
  const server = fakeServer();
  render(tree());
  const retention = await screen.findByRole("region", { name: "Retention and cleanup" });
  await within(retention).findByLabelText("Keep days");
  fireEvent.click(within(retention).getByRole("button", { name: "Preview cleanup" }));
  await waitFor(() => expect(server.writes()).toEqual(["POST prune"]));
  expect(server.calls.at(-1)?.body).toEqual({ "dry-run": true });
  fireEvent.click(within(retention).getByRole("button", { name: "Prune now" }));
  expect(server.writes()).toHaveLength(1);
  fireEvent.click(await screen.findByRole("button", { name: "Prune retained data" }));
  await waitFor(() => expect(server.writes()).toEqual(["POST prune", "POST prune"]));
  expect(server.calls.filter((c) => c.method === "POST").at(-1)?.body).toEqual({ "dry-run": false });
  expect(screen.queryByRole("region", { name: "Unsaved changes" })).not.toBeInTheDocument();
});

it("cancelling the prune confirmation writes nothing", async () => {
  const server = fakeServer();
  render(tree());
  const retention = await screen.findByRole("region", { name: "Retention and cleanup" });
  fireEvent.click(within(retention).getByRole("button", { name: "Prune now" }));
  fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(screen.queryByRole("button", { name: "Prune retained data" })).not.toBeInTheDocument());
  expect(server.writes()).toEqual([]);
});

it("reports the server's reason for a failed prune beside the action", async () => {
  fakeServer({ fail: { "POST prune": "prune is not permitted while the agent runs" } });
  render(tree());
  const retention = await screen.findByRole("region", { name: "Retention and cleanup" });
  fireEvent.click(within(retention).getByRole("button", { name: "Prune now" }));
  fireEvent.click(await screen.findByRole("button", { name: "Prune retained data" }));
  expect(await within(retention).findByRole("alert")).toHaveTextContent("prune is not permitted while the agent runs");
  expect(within(retention).getByRole("button", { name: "Prune now" })).toBeEnabled();
});

it("mounts no prune dialog buttons while its confirmation is closed", async () => {
  fakeServer();
  render(tree());
  await ready();
  expect(screen.queryByRole("button", { name: "Prune retained data" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Cancel" })).not.toBeInTheDocument();
});

// --- Schedule ----------------------------------------------------------------

it("shows the next scheduled run and saves or clears the RRULE schedule", async () => {
  const rule = "DTSTART:20261018T000000\nRRULE:FREQ=DAILY;BYHOUR=9;BYMINUTE=30;BYSECOND=0";
  const nextAt = "2026-10-19T09:30:00+03:00";
  const server = fakeServer({ view: { schedule: rule, schedule_next_at: nextAt } });
  render(tree());
  await ready();
  const loop = block("Loop and timeouts");
  expect(within(loop).getByLabelText("Schedule (RRULE)")).toHaveValue(rule);
  expect(within(loop).getByTestId("schedule-next-run")).toHaveTextContent(`Next run: ${new Date(nextAt).toLocaleString()}`);

  fireEvent.change(within(loop).getByLabelText("Schedule (RRULE)"), { target: { value: " RRULE:FREQ=WEEKLY;BYDAY=MO;BYHOUR=8;BYMINUTE=0 " } });
  save();
  await waitFor(() => expect(saveBar()).toHaveTextContent("All changes saved"));
  expect(server.calls.find((c) => c.path.endsWith("/loop/schedule"))?.body).toEqual({ value: "RRULE:FREQ=WEEKLY;BYDAY=MO;BYHOUR=8;BYMINUTE=0" });

  fireEvent.change(screen.getByLabelText("Schedule (RRULE)"), { target: { value: "" } });
  save();
  await waitFor(() => expect(server.calls.filter((c) => c.path.endsWith("/loop/schedule"))).toHaveLength(2));
  expect(server.calls.filter((c) => c.path.endsWith("/loop/schedule"))[1].body).toEqual({ clear: true });
  await waitFor(() => expect(screen.getByTestId("schedule-next-run")).toHaveTextContent("No scheduled run"));
});
