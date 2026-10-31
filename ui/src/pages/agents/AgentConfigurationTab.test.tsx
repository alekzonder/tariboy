import { afterEach, expect, it, vi } from "vitest";
// The query client is for Clear inbox under Advanced → Mail.
import { act, fireEvent, render, screen, waitFor, within } from "@/test/queryRender";
import { MemoryRouter, useLocation } from "react-router-dom";
import { AgentNameContext, AgentStatusContext } from "@/lib/agent";
import type { Daemon } from "@/lib/daemons";
import type { AgentImageStatus, AgentStatus, AgentView } from "@/lib/api";
import { clearHarnessCatalogCache } from "@/hooks/useHarnessCatalog";
import { RUNTIME_PRESETS_STORAGE_KEY } from "@/lib/runtimePresets";
import { notifyImagesChanged } from "@/lib/imageEvents";
import { ApiProvider } from "@/components/ApiProvider";
import { routeFetch, type FetchRequest } from "@/test/fetchRoutes";
import AgentConfigurationTab from "./AgentConfigurationTab";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  clearHarnessCatalogCache();
  localStorage.clear();
});

const remote: Daemon = {
  id: "remote-1",
  label: "Remote",
  baseURL: "https://remote.example",
  token: "secret",
};

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
  group: "",
  alias: "",
  notes: "",
  configured_cwd: "",
  color: "",
  schedule: "",
  schedule_next_at: "",
  budget: {
    hour_usd: 1,
    day_usd: 0,
    week_usd: 0,
    month_usd: 0,
    hour_spent_usd: 0.25,
    day_spent_usd: 0.25,
    week_spent_usd: 0.25,
    month_spent_usd: 0.25,
    exhausted: [],
  },
};

const projection = {
  name: "worker",
  current: { ref: "worker:latest", digest: "old-digest", image_version: "1.0.0", error: "" },
  pending: { ref: "", digest: "", error: "" },
  next: {
    ref: "worker:latest",
    digest: "new-digest",
    image_version: "1.1.0",
    reason: "ref_moved",
    error: "",
  },
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

type FakeState = ReturnType<typeof initialState>;
type Body = FetchRequest["body"];

function initialState(view?: Partial<AgentView>) {
  return {
    view: { ...stopped, ...view } as AgentView,
    pending: { ref: "", digest: "", error: "" },
    script: "echo hi\n",
    retention: {
      keep_iterations: 5,
      keep_days: 7,
      max_bytes: 1024,
      override: { keep_iterations: 5, keep_days: 0, max_bytes: 0 },
      default: { keep_iterations: 10, keep_days: 7, max_bytes: 1024 },
    },
    secrets: ["API_KEY"],
  };
}

// How an accepted POST to an agent setting changes the view a reload reads.
function writeView(view: AgentView, rest: string, body: Body): AgentView {
  if (rest === "loop/schedule")
    return {
      ...view,
      schedule: body.clear ? "" : body.value,
      schedule_next_at: body.clear ? "" : "2026-10-26T08:00:00Z",
    };
  if (rest === "budget")
    return {
      ...view,
      budget: {
        ...view.budget!,
        hour_usd: Number(body.hour_usd),
        day_usd: Number(body.day_usd),
        week_usd: Number(body.week_usd),
        month_usd: Number(body.month_usd),
      },
    };
  if (!VIEW_FIELD[rest]) return view;
  const [field, key] = VIEW_FIELD[rest];
  return { ...view, [field]: body[key] };
}

// The agent endpoints with their own state, by the path after /api/agents/NAME/.
function agentRoutes(
  state: FakeState,
  image?: () => unknown,
): Record<string, (req: FetchRequest) => Promise<Response>> {
  return {
    "": () => response(state.view),
    image: ({ method, body }) => {
      if (method === "POST") state.pending = { ref: body.image, digest: "sha256:v2", error: "" };
      if (method === "DELETE") state.pending = { ref: "", digest: "", error: "" };
      return response(image?.() ?? { ...projection, pending: state.pending });
    },
    "shell-script": ({ method, body }) => {
      if (method !== "POST") return response({ script: state.script });
      state.script = body.script;
      return response({ saved: true });
    },
    retention: ({ method, body }) => {
      if (method === "POST")
        state.retention = {
          ...state.retention,
          override: {
            ...state.retention.override,
            keep_iterations: body["keep-iterations"],
            keep_days: body["keep-days"],
          },
        };
      return response(state.retention);
    },
    secrets: ({ method, body, search }) => {
      if (method === "POST") state.secrets = [...state.secrets, body.key];
      if (method === "DELETE")
        state.secrets = state.secrets.filter((key) => key !== search.get("key"));
      return response({ keys: state.secrets, count: state.secrets.length });
    },
  };
}

const unexpected = ({ method, url }: FetchRequest) =>
  response({ code: "unexpected", message: `${method} ${url}` }, false, 500);

// fakeServer answers every request the page makes from one mutable state.
// `fail` rejects a write by "METHOD rest" with the given message.
function fakeServer(
  opts: {
    view?: Partial<AgentView>;
    image?: () => unknown;
    images?: () => Promise<Response>;
    fail?: Record<string, string>;
  } = {},
) {
  const state = initialState(opts.view);
  const agent = agentRoutes(state, opts.image);
  const route = routeFetch(
    {
      "* /api/fs/list*": () =>
        response({ path: "/srv", parent: "/", entries: [{ name: "new", dir: true }] }),
      "* /api/images": () =>
        opts.images?.() ??
        response({
          images: [
            { name: "worker", tag: "latest" },
            { name: "reviewer", tag: "v2" },
          ],
        }),
      "* /api/harnesses/*": () =>
        response({
          harness: "codex",
          models: [{ id: "gpt-x" }],
          efforts: ["low", "high"],
          error: "",
        }),
      "* /api/agents/*": (req) => {
        const rest = req.path.replace(/^\/api\/agents\/[^/]+\/?/, "");
        const failure = opts.fail?.[`${req.method} ${rest}`];
        if (failure) return response({ code: "invalid", message: failure }, false, 400);
        if (agent[rest]) return agent[rest](req);
        if (req.method !== "POST") return unexpected(req);
        state.view = writeView(state.view, rest, req.body);
        return response({});
      },
    },
    unexpected,
  );
  const calls: Call[] = [];
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    const parsed = new URL(url, "http://local");
    calls.push({
      method: init?.method ?? "GET",
      path: parsed.pathname + parsed.search,
      url,
      body: init?.body ? JSON.parse(String(init.body)) : undefined,
      headers: init?.headers,
    });
    return route(url, init);
  });
  vi.stubGlobal("fetch", fetchMock);
  const writes = () =>
    calls
      .filter((c) => c.method !== "GET")
      .map((c) => `${c.method} ${c.path.replace("/api/agents/worker/", "")}`);
  return { state, calls, writes, fetchMock };
}

const BASE = "/agents/remote-1/worker";

function Location() {
  return <span data-testid="location">{useLocation().pathname}</span>;
}

function tree(
  target: Daemon | null = remote,
  name = "worker",
  status: AgentStatus | null = null,
  refresh = vi.fn(),
) {
  return (
    <MemoryRouter initialEntries={[`${BASE}/settings`]}>
      {/* The agent workspace is the boundary that names the host. */}
      <ApiProvider target={target}>
        <AgentNameContext.Provider value={name}>
          <AgentStatusContext.Provider value={{ status, refresh: async () => {} }}>
            <AgentConfigurationTab base={BASE} refresh={refresh} />
            <Location />
          </AgentStatusContext.Provider>
        </AgentNameContext.Provider>
      </ApiProvider>
    </MemoryRouter>
  );
}

const section = (title: string) => screen.getByRole("region", { name: title });
const querySection = (title: string) => screen.queryByRole("region", { name: title });
const saveRow = () => screen.getByRole("region", { name: "Save settings" });
const save = () => fireEvent.click(within(saveRow()).getByRole("button", { name: "Save changes" }));
const ready = () => screen.findByLabelText("Minutes between runs");
const advancedTrigger = () => screen.getByRole("button", { name: /^Advanced/ });
const openAdvanced = async () => {
  const trigger = await screen.findByRole("button", { name: /^Advanced/ });
  if (trigger.getAttribute("aria-expanded") !== "true") fireEvent.click(trigger);
};
const changeFolder = async () => {
  fireEvent.click(await screen.findByRole("button", { name: "Change…" }));
  return screen.getByLabelText("Working directory");
};
// A group is labelled by its heading, so "Model" names the group and its field.
const control = (label: string) =>
  screen.getAllByLabelText(label).find((element) => element.tagName !== "SECTION")!;
const type = (label: string, value: string) =>
  fireEvent.change(control(label), { target: { value } });

// --- The acceptance scenario and the shared save mechanic --------------------

it("saves a model, an interval and a day budget with the same requests as before", async () => {
  const server = fakeServer({ view: { interval_s: 0 } });
  render(tree());
  await ready();
  type("Model", "opus");
  fireEvent.click(screen.getByRole("radio", { name: "Also every" }));
  type("Minutes between runs", "5");
  type("Day", "3");
  expect(saveRow()).toHaveTextContent("3 unsaved changes in Model, Autopilot, Spending limit");

  save();
  await waitFor(() => expect(saveRow()).toHaveTextContent("All changes saved"));
  expect(server.writes()).toEqual(["POST model", "POST loop/interval", "POST budget"]);
  const body = (path: string) => server.calls.find((c) => c.path.endsWith(path))?.body;
  expect(body("/model")).toEqual({ value: "opus" });
  expect(body("/loop/interval")).toEqual({ value: 300 });
  expect(body("/budget")).toEqual({ hour_usd: "1", day_usd: "3", week_usd: "0", month_usd: "0" });
  expect(server.calls.every((c) => c.url.startsWith("https://remote.example/"))).toBe(true);
  expect(within(saveRow()).queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
  expect(screen.getByLabelText("Minutes between runs")).toHaveValue("5");
});

it("shows four groups, then Advanced closed, About and Danger, in the narrow tab page", async () => {
  fakeServer();
  render(tree());
  await ready();
  const page = screen.getByTestId("tab-page");
  expect(page).toHaveClass("max-w-5xl");
  expect(screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent)).toEqual([
    "Model",
    "Working folder",
    "Autopilot",
    "Spending limit",
    "About",
    "Danger",
  ]);
  expect(advancedTrigger()).toHaveAttribute("aria-expanded", "false");
  expect(screen.getByLabelText("Hard timeout")).not.toBeVisible();
});

it("has Settings search without master switch, run actions, Changed only or section chips", async () => {
  fakeServer();
  render(tree());
  await ready();
  expect(screen.queryByText(/Master switch/)).not.toBeInTheDocument();
  expect(screen.getByRole("searchbox", { name: "Search settings" })).toHaveAttribute(
    "placeholder",
    "Search settings…",
  );
  expect(screen.queryByRole("button", { name: /Changed only/ })).not.toBeInTheDocument();
  expect(
    screen.queryByRole("navigation", { name: "Configuration blocks" }),
  ).not.toBeInTheDocument();
  for (const name of ["Start", "Stop", "Restart"])
    expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
});

it("shows the autopilot hint once per agent of the host", async () => {
  fakeServer();
  const text =
    "Autopilot: choose whether the agent also runs every few minutes on its own. Everything else is optional.";
  const view = render(tree());
  await ready();
  expect(screen.getByText(text)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Dismiss hint" }));
  expect(screen.queryByText(text)).not.toBeInTheDocument();
  expect(localStorage.getItem(`onboarding:hint:v1:${remote.id}:worker:settings`)).toBe("dismissed");
  view.unmount();
  render(tree());
  await ready();
  expect(screen.queryByText(text)).not.toBeInTheDocument();
});

it("has no save buttons inside the sections", async () => {
  fakeServer();
  render(tree());
  await ready();
  await openAdvanced();
  const sections = screen.getAllByRole("region").filter((r) => r !== saveRow());
  expect(sections.length).toBeGreaterThan(10);
  for (const region of sections) {
    expect(within(region).queryByRole("button", { name: /^save/i })).not.toBeInTheDocument();
  }
});

it("counts unsaved changes inside Advanced and keeps them while it is closed", async () => {
  fakeServer();
  render(tree());
  await ready();
  await openAdvanced();
  type("Hard timeout", "300");
  expect(within(section("Schedule and timeouts")).getByText("Changed")).toBeInTheDocument();
  expect(advancedTrigger()).toHaveTextContent("1 unsaved change");
  fireEvent.click(advancedTrigger());
  expect(screen.getByLabelText("Hard timeout")).not.toBeVisible();
  expect(saveRow()).toHaveTextContent("1 unsaved change in Advanced");
  await openAdvanced();
  expect(screen.getByLabelText("Hard timeout")).toHaveValue("300");
});

it("resets every change on every section without writing", async () => {
  const server = fakeServer();
  render(tree());
  await ready();
  type("Stop a run after (minutes)", "3");
  await openAdvanced();
  type("Hour budget", "9");
  fireEvent.click(within(saveRow()).getByRole("button", { name: "Reset" }));
  expect(screen.getByLabelText("Stop a run after (minutes)")).toHaveValue("1");
  expect(screen.getByLabelText("Hour budget")).toHaveValue(1);
  expect(server.writes()).toEqual([]);
});

it("stops at a failed write, reports it at the field, and keeps the unsent changes", async () => {
  const server = fakeServer({ fail: { "POST cwd": "directory does not exist" } });
  render(tree());
  await ready();
  fireEvent.change(await changeFolder(), { target: { value: "/srv/missing" } });
  type("Minutes between runs", "2");
  save();
  expect(await within(saveRow()).findByText(/Some changes were not saved/)).toBeInTheDocument();
  expect(server.writes()).toEqual(["POST cwd"]);
  expect(within(section("Working folder")).getByRole("alert")).toHaveTextContent(
    "directory does not exist",
  );
  expect(screen.getByLabelText("Working directory")).toHaveValue("/srv/missing");
  expect(saveRow()).toHaveTextContent("2 unsaved changes in Working folder, Autopilot");
});

it("opens Advanced at a field whose write failed and keeps the unsent drafts", async () => {
  const server = fakeServer({ fail: { "POST loop/hard-timeout": "hard timeout too short" } });
  render(tree());
  await ready();
  await openAdvanced();
  type("Hard timeout", "5");
  type("Mail: letters per iteration", "20");
  fireEvent.click(advancedTrigger());
  save();
  const field = await screen.findByLabelText("Hard timeout");
  expect(server.writes()).toEqual(["POST loop/hard-timeout"]);
  expect(within(section("Schedule and timeouts")).getByRole("alert")).toHaveTextContent(
    "hard timeout too short",
  );
  await waitFor(() => expect(field).toHaveFocus());
  expect(screen.getByLabelText("Mail: letters per iteration")).toHaveValue(20);
  expect(saveRow()).toHaveTextContent("2 unsaved changes in Advanced");
});

it("sends nothing while a change is invalid and focuses it inside an opened Advanced", async () => {
  const server = fakeServer();
  render(tree());
  await ready();
  await openAdvanced();
  type("Mail: letters per iteration", "0");
  fireEvent.click(advancedTrigger());
  type("Minutes between runs", "2");
  save();
  const field = await screen.findByLabelText("Mail: letters per iteration");
  await waitFor(() => expect(field).toHaveFocus());
  expect(within(section("Mail")).getByRole("alert")).toHaveTextContent(/positive whole number/);
  expect(server.writes()).toEqual([]);
});

// --- Model -------------------------------------------------------------------

it("saves the coding tool before the model and remembers the model for it", async () => {
  const server = fakeServer();
  render(tree());
  await ready();
  type("Model", "opus");
  type("Coding tool", "claude");
  fireEvent.click(screen.getByRole("switch", { name: "Interactive terminal" }));
  expect(server.writes()).toEqual([]);
  save();
  await waitFor(() =>
    expect(server.writes()).toEqual(["POST harness", "POST model", "POST interactive"]),
  );
  expect(server.calls.find((c) => c.path.endsWith("/interactive"))?.body).toEqual({ value: false });
  await waitFor(() => expect(localStorage.getItem(RUNTIME_PRESETS_STORAGE_KEY)).toContain("opus"));
});

it("offers the operator coding tools and keeps an agent's out-of-list harness", async () => {
  fakeServer({ view: { harness: "stub" } });
  render(tree());
  await ready();
  const options = within(screen.getByLabelText("Coding tool"))
    .getAllByRole("option")
    .map((o) => o.textContent);
  expect(options).toEqual(["claude", "codex", "opencode", "cursor", "stub"]);
  expect(within(section("Model")).getByText("Takes effect on the next run.")).toBeInTheDocument();
});

// --- Autopilot ---------------------------------------------------------------

it("shows a positive interval as Also every N minutes and the timeout in minutes", async () => {
  fakeServer({ view: { interval_s: 1800, timeout_s: 7200 } });
  render(tree());
  await ready();
  expect(screen.getByRole("radio", { name: "Also every" })).toBeChecked();
  expect(screen.getByLabelText("Minutes between runs")).toHaveValue("30");
  expect(screen.getByLabelText("Minutes between runs")).toBeEnabled();
  expect(screen.getByLabelText("Stop a run after (minutes)")).toHaveValue("120");
});

it("runs only on demand with interval 0 and remembers 30 minutes for the second radio", async () => {
  const server = fakeServer({ view: { interval_s: 0 } });
  render(tree());
  await ready();
  expect(screen.getByRole("radio", { name: "Only when given tasks or messages" })).toBeChecked();
  expect(screen.getByLabelText("Minutes between runs")).toBeDisabled();
  expect(screen.getByLabelText("Minutes between runs")).toHaveValue("30");
  fireEvent.click(screen.getByRole("radio", { name: "Also every" }));
  save();
  await waitFor(() => expect(server.writes()).toEqual(["POST loop/interval"]));
  expect(server.calls.find((c) => c.path.endsWith("/loop/interval"))?.body).toEqual({
    value: 1800,
  });
});

it("turns Autopilot off with the first radio and sends interval 0", async () => {
  const server = fakeServer({ view: { interval_s: 600 } });
  render(tree());
  await ready();
  fireEvent.click(screen.getByRole("radio", { name: "Only when given tasks or messages" }));
  expect(screen.getByLabelText("Minutes between runs")).toBeDisabled();
  expect(screen.getByLabelText("Minutes between runs")).toHaveValue("10");
  save();
  await waitFor(() => expect(server.writes()).toEqual(["POST loop/interval"]));
  expect(server.calls.find((c) => c.path.endsWith("/loop/interval"))?.body).toEqual({ value: 0 });
});

it("sends Stop a run after N minutes as seconds", async () => {
  const server = fakeServer();
  render(tree());
  await ready();
  type("Stop a run after (minutes)", "2");
  save();
  await waitFor(() => expect(server.writes()).toEqual(["POST loop/timeout"]));
  expect(server.calls.find((c) => c.path.endsWith("/loop/timeout"))?.body).toEqual({ value: 120 });
});

it("keeps an emptied minutes box and blocks Save with its reason", async () => {
  const server = fakeServer();
  render(tree());
  await ready();
  type("Minutes between runs", "");
  expect(screen.getByRole("radio", { name: "Also every" })).toBeChecked();
  expect(screen.getByLabelText("Minutes between runs")).toHaveValue("");
  save();
  expect(await screen.findByText("Enter the minutes between runs.")).toBeInTheDocument();
  await waitFor(() => expect(screen.getByLabelText("Minutes between runs")).toHaveFocus());
  expect(server.writes()).toEqual([]);
});

it("can enable timed runs again after typing a zero interval", async () => {
  fakeServer({ view: { interval_s: 600 } });
  render(tree());
  await ready();
  type("Minutes between runs", "0");
  fireEvent.click(screen.getByRole("radio", { name: "Also every" }));
  expect(screen.getByRole("radio", { name: "Also every" })).toBeChecked();
  expect(screen.getByLabelText("Minutes between runs")).toBeEnabled();
  expect(screen.getByLabelText("Minutes between runs")).toHaveValue("30");
});

it.each([
  ["Minutes between runs", "Enter the minutes between runs."],
  ["Stop a run after (minutes)", "Enter the minutes, or 0 for no limit."],
])("rejects negative fractional minutes in %s", async (label, error) => {
  const server = fakeServer();
  render(tree());
  await ready();
  type(label, "-0.5");
  save();
  expect(await screen.findByText(error)).toBeInTheDocument();
  expect(screen.getByLabelText(label)).toHaveValue("-0.5");
  expect(server.writes()).toEqual([]);
});

it("says a saved schedule also starts runs", async () => {
  fakeServer({ view: { schedule: "RRULE:FREQ=DAILY;BYHOUR=9;BYMINUTE=0;BYSECOND=0" } });
  render(tree());
  await ready();
  expect(
    within(section("Autopilot")).getByText(
      "A schedule also starts runs: see Advanced → Schedule and timeouts.",
    ),
  ).toBeInTheDocument();
});

it("has no schedule note without a schedule", async () => {
  fakeServer();
  render(tree());
  await ready();
  expect(screen.queryByText(/A schedule also starts runs/)).not.toBeInTheDocument();
});

// --- Advanced → Image ----------------------------------------------------------

it("selects a built image and applies it with Save", async () => {
  const server = fakeServer({ image: undefined });
  render(tree());
  await openAdvanced();
  fireEvent.click(await screen.findByRole("combobox", { name: "Agent image" }));
  fireEvent.click(await screen.findByRole("option", { name: "reviewer:v2" }));
  expect(server.writes()).toEqual([]);
  save();
  expect(await screen.findByText("Pending:")).toBeInTheDocument();
  expect(server.calls.find((c) => c.method === "POST" && c.path.endsWith("/image"))?.body).toEqual({
    image: "reviewer:v2",
  });
  expect(
    within(screen.getByText("Pending:").parentElement!).getByRole("link", { name: "reviewer:v2" }),
  ).toHaveAttribute("href", "/library/images/reviewer/v2");
});

it("cancels an explicit pending image at once, outside Save", async () => {
  let state: AgentImageStatus = {
    ...projection,
    pending: { ref: "explicit:v2", digest: "explicit-digest", error: "" },
  };
  const server = fakeServer({ image: () => state });
  const original = server.fetchMock.getMockImplementation()!;
  server.fetchMock.mockImplementation((url: string, init?: RequestInit) => {
    if (url.endsWith("/image") && init?.method === "DELETE") state = projection;
    return original(url, init);
  });
  render(tree());
  await openAdvanced();
  fireEvent.click(await screen.findByRole("button", { name: "Cancel pending" }));
  await waitFor(() => expect(screen.queryByText("Pending:")).not.toBeInTheDocument());
  expect(screen.queryByRole("button", { name: "Cancel pending" })).not.toBeInTheDocument();
  expect(server.writes()).toEqual(["DELETE image"]);
});

it("shows no Current or Next iteration preview, and the digest once", async () => {
  fakeServer();
  render(tree());
  await openAdvanced();
  await screen.findByTestId("image-digest");
  expect(screen.queryByRole("group", { name: "Next iteration" })).not.toBeInTheDocument();
  expect(screen.queryByRole("group", { name: "Activated version" })).not.toBeInTheDocument();
  expect(screen.queryByText("Update on next iteration")).not.toBeInTheDocument();
  expect(screen.getAllByTestId("image-digest")).toHaveLength(1);
});

it("shows an activation failure with Retry", async () => {
  fakeServer({
    image: () => ({
      ...projection,
      pending: { ref: "worker:v2", digest: "", error: "bridge preparation failed" },
    }),
  });
  render(tree());
  await openAdvanced();
  expect(await screen.findByText("bridge preparation failed")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
});

it("reports an unavailable image endpoint and disables the image choice", async () => {
  const server = fakeServer();
  const original = server.fetchMock.getMockImplementation()!;
  server.fetchMock.mockImplementation((url: string, init?: RequestInit) =>
    url.endsWith("/image")
      ? Promise.reject(new Error("image request failed"))
      : original(url, init),
  );
  render(tree());
  await openAdvanced();
  expect(
    await screen.findByText(/Image status unavailable:.*image request failed/),
  ).toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: "Agent image" })).toBeDisabled();
});

it("disables the image choice while the inventory failed", async () => {
  fakeServer({
    images: () => response({ code: "internal", message: "archive is corrupt" }, false, 500),
  });
  render(tree());
  await openAdvanced();
  expect(await screen.findByText(/Image inventory unavailable:/)).toHaveTextContent(
    "archive is corrupt",
  );
  expect(screen.getByRole("combobox", { name: "Agent image" })).toBeDisabled();
});

it.each(["host", "agent", "refresh"])(
  "ignores delayed image responses after a %s change",
  async (change) => {
    let finish!: (value: Response) => void;
    let count = 0;
    const fresh = { ...projection, pending: { ref: "fresh:v2", digest: "d", error: "" } };
    const server = fakeServer({ image: () => fresh });
    const original = server.fetchMock.getMockImplementation()!;
    server.fetchMock.mockImplementation((url: string, init?: RequestInit) => {
      if (url.endsWith("/image") && count++ === 0)
        return new Promise<Response>((resolve) => {
          finish = resolve;
        });
      return original(url, init);
    });
    const { rerender } = render(tree());
    await waitFor(() => expect(finish).toBeDefined());
    if (change === "refresh") act(() => notifyImagesChanged());
    else
      rerender(
        tree(
          change === "host"
            ? { ...remote, id: "remote-2", baseURL: "https://other.example" }
            : remote,
          change === "agent" ? "other" : "worker",
        ),
      );
    await openAdvanced();
    expect(await screen.findByRole("link", { name: "fresh:v2" })).toBeInTheDocument();
    await act(async () => finish(await response(projection)));
    expect(screen.getByRole("link", { name: "fresh:v2" })).toBeInTheDocument();
  },
);

it("truncates the image digest while exposing the complete value, and copies it", async () => {
  const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.assign(navigator, { clipboard: { writeText } });
  fakeServer({ view: { digest } });
  render(tree());
  await openAdvanced();
  const shown = await screen.findByTestId("image-digest");
  expect(shown.textContent).toContain("sha256:0123");
  expect(shown).toHaveAttribute("aria-hidden", "true");
  expect(screen.getByText(digest)).toHaveClass("sr-only");
  fireEvent.click(screen.getByRole("button", { name: "Copy digest" }));
  await waitFor(() => expect(writeText).toHaveBeenCalledWith(digest));
  expect(await screen.findByText("Digest copied")).toBeInTheDocument();
});

it("refreshes the image projection on status and build events without replacing drafts", async () => {
  const pending = (ref: string) => ({ ...projection, pending: { ref, digest: "", error: "" } });
  let state: AgentImageStatus = pending("a:1");
  fakeServer({ image: () => state });
  const { rerender } = render(tree());
  await ready();
  fireEvent.change(await changeFolder(), { target: { value: "/srv/draft" } });
  await openAdvanced();
  await screen.findByRole("link", { name: "a:1" });
  state = pending("b:2");
  rerender(tree(remote, "worker", { name: "worker", state: "stopped" } as AgentStatus));
  expect(await screen.findByRole("link", { name: "b:2" })).toBeInTheDocument();
  state = pending("c:3");
  act(() => notifyImagesChanged());
  expect(await screen.findByRole("link", { name: "c:3" })).toBeInTheDocument();
  expect(screen.getByLabelText("Working directory")).toHaveValue("/srv/draft");
});

// --- Working folder -------------------------------------------------------------

it("edits a stopped agent's working folder in place with suggestions from its route host", async () => {
  const refresh = vi.fn();
  const server = fakeServer();
  render(tree(remote, "worker", null, refresh));
  await ready();
  expect(within(section("Working folder")).getByText("/srv/old")).toBeInTheDocument();
  expect(screen.getByLabelText("Working directory")).not.toBeVisible();
  const input = await changeFolder();
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

it("keeps the working folder read-only while the agent is active", async () => {
  fakeServer({ view: { state: "running", enabled: true } });
  render(tree());
  expect(await screen.findByText("/srv/old")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Change…" })).toBeDisabled();
  expect(screen.queryByLabelText("Working directory")).not.toBeInTheDocument();
  expect(
    screen.getByText("Stop the agent before changing its working directory."),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("switch", { name: "Create directory if missing" }),
  ).not.toBeInTheDocument();
});

it("switches to read-only when live status reports that the agent started", async () => {
  fakeServer();
  render(tree(remote, "worker", { name: "worker", state: "running" } as AgentStatus));
  expect(await screen.findByText("/srv/old")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Change…" })).toBeDisabled();
});

it("keeps drafts when the route host is re-rendered or a reconnect fails", async () => {
  const server = fakeServer();
  const { rerender } = render(tree());
  await ready();
  fireEvent.change(await changeFolder(), { target: { value: "/srv/new" } });
  rerender(tree({ ...remote }));
  await act(async () => {});
  expect(
    server.calls.filter((c) => c.url === "https://remote.example/api/agents/worker"),
  ).toHaveLength(1);
  rerender(tree({ ...remote, baseURL: "" }));
  const unreachable = vi.fn().mockRejectedValue(new TypeError("Failed to fetch"));
  vi.stubGlobal("fetch", unreachable);
  rerender(tree({ ...remote, baseURL: "https://moved.example" }));
  await waitFor(() => expect(unreachable).toHaveBeenCalled());
  await act(async () => {});
  expect(screen.getByLabelText("Working directory")).toHaveValue("/srv/new");
  expect(screen.getByText("worker:v1")).toBeInTheDocument();
});

it("creates a missing working directory only when its switch is on", async () => {
  const refresh = vi.fn();
  const server = fakeServer();
  render(tree(remote, "worker", null, refresh));
  await ready();
  const input = await changeFolder();
  const create = screen.getByRole("switch", { name: "Create directory if missing" });
  expect(create).not.toBeChecked();
  fireEvent.change(input, { target: { value: "/srv/new" } });
  fireEvent.click(create);
  save();
  await waitFor(() => expect(refresh).toHaveBeenCalled());
  const write = server.calls.find((c) => c.method === "POST" && c.path.endsWith("/cwd"))!;
  expect(write.body).toEqual({ value: "/srv/new", create: true });
  await waitFor(() =>
    expect(screen.getByRole("switch", { name: "Create directory if missing" })).not.toBeChecked(),
  );
});

// --- Spending limit and budgets ------------------------------------------------

it("shows day and month spend in Spending limit and exhausted periods", async () => {
  fakeServer({ view: { budget: { ...stopped.budget!, day_spent_usd: 8.56, exhausted: ["day"] } } });
  render(tree());
  await ready();
  const spending = section("Spending limit");
  expect(within(spending).getByText("USD (0 = none) · spent $8.56 today")).toBeInTheDocument();
  expect(within(spending).getByText("USD (0 = none) · spent $0.25 this month")).toBeInTheDocument();
  expect(within(spending).getByText("Out of budget: day")).toBeInTheDocument();
  expect(within(spending).queryByLabelText("Hour budget")).not.toBeInTheDocument();
});

it("edits Week once in Spending limit and keeps the other three budget values", async () => {
  const server = fakeServer({
    view: {
      budget: {
        ...stopped.budget!,
        hour_usd: 1,
        day_usd: 2,
        week_usd: 3,
        month_usd: 4,
        week_spent_usd: 1.75,
      },
    },
  });
  render(tree());
  await ready();
  const spending = section("Spending limit");
  expect(within(spending).getByLabelText("Week")).toHaveValue(3);
  expect(within(spending).getByText("USD (0 = none) · spent $1.75 this week")).toBeInTheDocument();
  expect(screen.getAllByLabelText("Week")).toHaveLength(1);
  type("Week", "7");
  expect(saveRow()).toHaveTextContent("1 unsaved change in Spending limit");
  await openAdvanced();
  const budgets = section("Hour budget (USD)");
  expect(within(budgets).getByText(/Spent 0.25 of 1.00/)).toBeInTheDocument();
  expect(within(budgets).queryByLabelText("Week")).not.toBeInTheDocument();
  save();
  await waitFor(() => expect(server.writes()).toEqual(["POST budget"]));
  expect(server.calls.find((c) => c.path.endsWith("/budget"))?.body).toEqual({
    hour_usd: "1",
    day_usd: "2",
    week_usd: "7",
    month_usd: "4",
  });
});

it("renders a budget projection from an older daemon without exhausted periods", async () => {
  const { exhausted: _, ...legacy } = stopped.budget;
  fakeServer({ view: { budget: legacy as AgentView["budget"] } });
  render(tree());
  await ready();
  expect(within(section("Spending limit")).queryByText(/Out of budget/)).not.toBeInTheDocument();
});

it("leaves out every budget field when the daemon has no budget projection", async () => {
  fakeServer({ view: { budget: undefined } });
  render(tree());
  await ready();
  expect(querySection("Spending limit")).not.toBeInTheDocument();
  await openAdvanced();
  expect(querySection("Hour budget (USD)")).not.toBeInTheDocument();
});

it("saves a zero Week limit as unlimited", async () => {
  const server = fakeServer({ view: { budget: { ...stopped.budget!, week_usd: 3 } } });
  render(tree());
  await ready();
  type("Week", "0");
  save();
  await waitFor(() => expect(saveRow()).toHaveTextContent("All changes saved"));
  expect(server.state.view.budget?.week_usd).toBe(0);
});

it("rejects an empty Week without saving the other edits", async () => {
  const server = fakeServer();
  render(tree());
  await ready();
  type("Week", "");
  type("Model", "opus");
  save();
  expect(await screen.findByText("Enter zero or a positive amount.")).toBeInTheDocument();
  await waitFor(() => expect(screen.getByLabelText("Week")).toHaveFocus());
  expect(server.writes()).toEqual([]);
});

it("keeps Week after a rejected budget save", async () => {
  const server = fakeServer({ fail: { "POST budget": "budget rejected" } });
  render(tree());
  await ready();
  type("Week", "7");
  save();
  expect(await screen.findByText("budget rejected")).toBeVisible();
  expect(screen.getByLabelText("Week")).toHaveValue(7);
  expect(saveRow()).toHaveTextContent("1 unsaved change in Spending limit");
  expect(server.writes()).toEqual(["POST budget"]);
});

// --- Settings search -----------------------------------------------------------

const searchSettings = (value: string) =>
  fireEvent.change(screen.getByRole("searchbox", { name: "Search settings" }), {
    target: { value },
  });
const clearSearch = () => fireEvent.click(screen.getByRole("button", { name: "Clear search" }));

it.each([
  ["MODEL", "Model"],
  ["spending week", "Spending limit"],
  ["hour", "Hour budget (USD)"],
  ["MAIL maximum", "Mail"],
  ["shell script", "Environment"],
  ["Other views prompt", "Other views"],
  ["about host", "About"],
  ["delete agent", "Danger"],
])("searches static section and field names for %s", async (query, title) => {
  const server = fakeServer();
  render(tree());
  await ready();
  searchSettings(query);
  const matches = screen.getAllByRole("region").filter((r) => r !== saveRow());
  expect(matches.map((r) => r.getAttribute("aria-labelledby"))).toEqual([
    section(title).getAttribute("aria-labelledby"),
  ]);
  expect(server.writes()).toEqual([]);
});

it.each([false, true])(
  "restores Advanced disclosure after clearing a search (open=%s)",
  async (open) => {
    const server = fakeServer();
    render(tree());
    await ready();
    if (open) await openAdvanced();
    searchSettings("hard timeout");
    expect(advancedTrigger()).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByLabelText("Hard timeout")).toBeVisible();
    clearSearch();
    expect(advancedTrigger()).toHaveAttribute("aria-expanded", String(open));
    expect(section("Model")).toBeVisible();
    expect(server.writes()).toEqual([]);
  },
);

it("preserves drafts and mounted editors through search and disclosure changes, then saves hidden edits", async () => {
  const server = fakeServer();
  render(tree());
  await ready();
  type("Model", "opus");
  type("Week", "9");
  fireEvent.change(await changeFolder(), { target: { value: "/srv/new" } });
  await openAdvanced();
  type("Hard timeout", "300");
  const editor = screen.getByLabelText("Hard timeout");
  const folder = screen.getByLabelText("Working directory");
  searchSettings("Model");
  expect(editor).toBeInTheDocument();
  expect(editor).not.toBeVisible();
  clearSearch();
  expect(screen.getByLabelText("Hard timeout")).toBe(editor);
  expect(editor).toHaveValue("300");
  expect(screen.getByLabelText("Working directory")).toBe(folder);
  expect(control("Model")).toHaveValue("opus");
  expect(screen.getByLabelText("Week")).toHaveValue(9);
  searchSettings("nothing matches");
  expect(screen.getByText("No settings match your search.")).toBeVisible();
  expect(within(saveRow()).getByRole("button", { name: "Save changes" })).toBeVisible();
  expect(server.writes()).toEqual([]);
  save();
  await waitFor(() => expect(saveRow()).toHaveTextContent("All changes saved"));
  expect(server.writes()).toEqual([
    "POST cwd",
    "POST model",
    "POST loop/hard-timeout",
    "POST budget",
  ]);
  clearSearch();
  expect(screen.getByLabelText("Week")).toHaveValue(9);
});

it("temporarily opens the matching folder editor and restores it when cleared", async () => {
  fakeServer();
  render(tree());
  await ready();
  searchSettings("working directory");
  expect(screen.getByLabelText("Working directory")).toBeVisible();
  clearSearch();
  expect(screen.queryByRole("textbox", { name: "Working directory" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Change…" })).toBeVisible();
});

it("keeps intermediate minutes text when its section is hidden by search", async () => {
  fakeServer({ view: { interval_s: 60 } });
  render(tree());
  await ready();
  type("Minutes between runs", "1.");
  const minutes = screen.getByLabelText("Minutes between runs");
  searchSettings("Model");
  clearSearch();
  expect(screen.getByLabelText("Minutes between runs")).toBe(minutes);
  expect(minutes).toHaveValue("1.");
});

it("ignores whitespace and secret values when searching", async () => {
  const server = fakeServer();
  render(tree());
  await ready();
  await openAdvanced();
  type("Secret key", "TOKEN");
  type("Secret value", "unique-secret-123");
  searchSettings("unique-secret-123");
  expect(querySection("Environment")).not.toBeInTheDocument();
  expect(screen.getByText("No settings match your search.")).toBeVisible();
  clearSearch();
  expect(screen.getByLabelText("Secret value")).toHaveValue("unique-secret-123");
  searchSettings("   ");
  expect(section("Model")).toBeVisible();
  expect(section("Environment")).toBeVisible();
  expect(server.writes()).toEqual([]);
});

it("clears a filter and reveals an invalid hidden field on Save", async () => {
  const server = fakeServer();
  render(tree());
  await ready();
  searchSettings("Mail");
  type("Mail: letters per iteration", "0");
  searchSettings("Model");
  save();
  await waitFor(() => expect(screen.getByLabelText("Mail: letters per iteration")).toHaveFocus());
  expect(screen.getByRole("searchbox", { name: "Search settings" })).toHaveValue("");
  expect(section("Mail")).toBeVisible();
  expect(advancedTrigger()).toHaveAttribute("aria-expanded", "true");
  expect(server.writes()).toEqual([]);
});

it("reveals a hidden failed server field and keeps its draft", async () => {
  const server = fakeServer({ fail: { "POST cwd": "directory does not exist" } });
  render(tree());
  await ready();
  searchSettings("working directory");
  type("Working directory", "/srv/missing");
  searchSettings("Model");
  save();
  await waitFor(() => expect(screen.getByLabelText("Working directory")).toHaveFocus());
  expect(screen.getByLabelText("Working directory")).toHaveValue("/srv/missing");
  expect(section("Working folder")).toBeVisible();
  expect(server.writes()).toEqual(["POST cwd"]);
});

// --- Advanced → Goal, Mail -------------------------------------------------------

it("renders the current Goal task read-only with its help", async () => {
  fakeServer({ view: { current_goal_task_key: "DEV-7" } });
  render(tree());
  await openAdvanced();
  expect(await screen.findByLabelText("Current goal task")).toHaveValue("DEV-7");
  expect(screen.getByLabelText("Current goal task")).toBeDisabled();
});

it("saves the mail limits with Save", async () => {
  const server = fakeServer();
  render(tree());
  await openAdvanced();
  type("Mail: letters per iteration", "20");
  type("Mail: maximum open inbox", "200");
  save();
  await waitFor(() =>
    expect(server.writes()).toEqual(["POST messages/batch", "POST messages/max-queue"]),
  );
});

it("offers Clear inbox under Mail only while letters are open", async () => {
  fakeServer();
  const { rerender } = render(tree());
  await openAdvanced();
  const mail = section("Mail");
  expect(within(mail).getByRole("button", { name: "Clear inbox" })).toBeDisabled();
  rerender(
    tree(remote, "worker", {
      name: "worker",
      state: "stopped",
      messages_pending: 3,
    } as AgentStatus),
  );
  fireEvent.click(within(section("Mail")).getByRole("button", { name: "Clear inbox" }));
  expect(await screen.findByRole("dialog", { name: "Clear inbox" })).toBeInTheDocument();
});

// --- Advanced → Environment --------------------------------------------------------

it("saves the agent shell script with Save", async () => {
  const server = fakeServer();
  render(tree());
  await openAdvanced();
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
  await openAdvanced();
  const env = await screen.findByRole("region", { name: "Environment" });
  await within(env).findByText("API_KEY");
  fireEvent.click(within(env).getByRole("button", { name: "Remove API_KEY" }));
  expect(within(env).getByText("Will be removed")).toBeInTheDocument();
  fireEvent.change(within(env).getByLabelText("Secret key"), { target: { value: "TOKEN" } });
  fireEvent.change(within(env).getByLabelText("Secret value"), { target: { value: "s3cr3t" } });
  expect(screen.queryByText("s3cr3t")).toBeNull();
  expect(server.writes()).toEqual([]);
  save();
  await waitFor(() =>
    expect(server.writes()).toEqual(["DELETE secrets?key=API_KEY", "POST secrets"]),
  );
  expect(
    server.calls.find((c) => c.method === "POST" && c.path.endsWith("/secrets"))?.body,
  ).toEqual({ key: "TOKEN", value: "s3cr3t" });
  expect(server.calls.some((c) => c.url.includes("s3cr3t"))).toBe(false);
  await waitFor(() => expect(within(env).getByLabelText("Secret value")).toHaveValue(""));
  expect(within(env).getByText("TOKEN")).toBeInTheDocument();
  expect(within(env).queryByText("API_KEY")).not.toBeInTheDocument();
});

it("requires a secret key when a value is entered", async () => {
  const server = fakeServer();
  render(tree());
  await openAdvanced();
  const env = await screen.findByRole("region", { name: "Environment" });
  await within(env).findByText("API_KEY");
  fireEvent.change(within(env).getByLabelText("Secret value"), { target: { value: "s3cr3t" } });
  save();
  expect(await within(env).findByText("Enter a secret key.")).toBeInTheDocument();
  expect(server.writes()).toEqual([]);
});

// --- Advanced → Retention -------------------------------------------------------

it("saves the retention override with Save and shows the server defaults", async () => {
  const server = fakeServer();
  render(tree());
  await openAdvanced();
  const retention = await screen.findByRole("region", { name: "Retention and cleanup" });
  const days = await within(retention).findByLabelText("Keep days");
  expect(within(retention).getByText(/0 = server default \(7\)/)).toBeInTheDocument();
  fireEvent.change(days, { target: { value: "14" } });
  save();
  await waitFor(() => expect(server.writes()).toEqual(["POST retention"]));
  expect(
    server.calls.find((c) => c.path.endsWith("/retention") && c.method === "POST")?.body,
  ).toEqual({ "keep-iterations": 5, "keep-days": 14 });
});

it("keeps prune and preview as confirmed, immediate actions outside Save", async () => {
  const server = fakeServer();
  render(tree());
  await openAdvanced();
  const retention = await screen.findByRole("region", { name: "Retention and cleanup" });
  await within(retention).findByLabelText("Keep days");
  fireEvent.click(within(retention).getByRole("button", { name: "Preview cleanup" }));
  await waitFor(() => expect(server.writes()).toEqual(["POST prune"]));
  expect(server.calls.filter((c) => c.method === "POST").at(-1)?.body).toEqual({ "dry-run": true });
  fireEvent.click(within(retention).getByRole("button", { name: "Prune now" }));
  expect(server.writes()).toHaveLength(1);
  fireEvent.click(await screen.findByRole("button", { name: "Prune retained data" }));
  await waitFor(() => expect(server.writes()).toEqual(["POST prune", "POST prune"]));
  expect(server.calls.filter((c) => c.method === "POST").at(-1)?.body).toEqual({
    "dry-run": false,
  });
  expect(within(saveRow()).queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
});

it("cancelling the prune confirmation writes nothing", async () => {
  const server = fakeServer();
  render(tree());
  await openAdvanced();
  const retention = await screen.findByRole("region", { name: "Retention and cleanup" });
  fireEvent.click(within(retention).getByRole("button", { name: "Prune now" }));
  fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "Prune retained data" })).not.toBeInTheDocument(),
  );
  expect(server.writes()).toEqual([]);
});

it("reports the server's reason for a failed prune beside the action", async () => {
  fakeServer({ fail: { "POST prune": "prune is not permitted while the agent runs" } });
  render(tree());
  await openAdvanced();
  const retention = await screen.findByRole("region", { name: "Retention and cleanup" });
  fireEvent.click(within(retention).getByRole("button", { name: "Prune now" }));
  fireEvent.click(await screen.findByRole("button", { name: "Prune retained data" }));
  expect(await within(retention).findByRole("alert")).toHaveTextContent(
    "prune is not permitted while the agent runs",
  );
  expect(within(retention).getByRole("button", { name: "Prune now" })).toBeEnabled();
});

it("mounts no dialog buttons while the confirmations are closed", async () => {
  fakeServer();
  render(tree());
  await openAdvanced();
  await ready();
  expect(screen.queryByRole("button", { name: "Prune retained data" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Cancel" })).not.toBeInTheDocument();
});

// --- Advanced → Schedule and timeouts ------------------------------------------

it("opens a saved rule in its builder mode and leaves it unchanged", async () => {
  const rule = "DTSTART:20261018T000000\nRRULE:FREQ=DAILY;BYHOUR=9;BYMINUTE=30;BYSECOND=0";
  const nextAt = "2026-10-19T09:30:00+03:00";
  fakeServer({ view: { schedule: rule, schedule_next_at: nextAt } });
  render(tree());
  await openAdvanced();
  const loop = section("Schedule and timeouts");
  expect(within(loop).getByLabelText("Schedule")).toHaveValue("daily");
  expect(within(loop).getByLabelText("Time")).toHaveValue("09:30");
  expect(within(loop).getByTestId("schedule-summary")).toHaveTextContent("Every day at 09:30");
  expect(within(loop).getByTestId("schedule-rrule")).toHaveTextContent("FREQ=DAILY;BYHOUR=9");
  expect(within(loop).getByTestId("schedule-next-run")).toHaveTextContent(
    `Next run: ${new Date(nextAt).toLocaleString()}`,
  );
  expect(within(loop).queryByText("Changed")).not.toBeInTheDocument();
});

it("keeps a rule no builder mode fits in Custom RRULE, unchanged", async () => {
  const rule = "DTSTART:20261018T000000\nRRULE:FREQ=DAILY;BYHOUR=9,18;BYMINUTE=0;BYSECOND=0";
  fakeServer({ view: { schedule: rule } });
  render(tree());
  await openAdvanced();
  const loop = section("Schedule and timeouts");
  expect(within(loop).getByLabelText("Schedule")).toHaveValue("custom");
  expect(within(loop).getByLabelText("Custom RRULE")).toHaveValue(rule);
  expect(within(loop).queryByText("Changed")).not.toBeInTheDocument();
});

it("builds and saves a weekly schedule, then clears it with None", async () => {
  const rule = "DTSTART:20261018T000000\nRRULE:FREQ=DAILY;BYHOUR=9;BYMINUTE=30;BYSECOND=0";
  const server = fakeServer({ view: { schedule: rule, schedule_next_at: "2026-10-19T09:30:00Z" } });
  render(tree());
  await openAdvanced();
  const loop = section("Schedule and timeouts");
  fireEvent.change(within(loop).getByLabelText("Schedule"), { target: { value: "weekly" } });
  expect(within(loop).getByRole("button", { name: "Mon" })).toHaveAttribute("aria-pressed", "true");
  fireEvent.change(within(loop).getByLabelText("Time"), { target: { value: "08:00" } });
  expect(within(loop).getByText("Changed")).toBeInTheDocument();
  save();
  await waitFor(() => expect(saveRow()).toHaveTextContent("All changes saved"));
  expect(server.calls.find((c) => c.path.endsWith("/loop/schedule"))?.body).toEqual({
    value: "RRULE:FREQ=WEEKLY;BYDAY=MO;BYHOUR=8;BYMINUTE=0;BYSECOND=0",
  });

  fireEvent.change(screen.getByLabelText("Schedule"), { target: { value: "none" } });
  save();
  await waitFor(() =>
    expect(server.calls.filter((c) => c.path.endsWith("/loop/schedule"))).toHaveLength(2),
  );
  expect(server.calls.filter((c) => c.path.endsWith("/loop/schedule"))[1].body).toEqual({
    clear: true,
  });
  await waitFor(() =>
    expect(screen.getByTestId("schedule-next-run")).toHaveTextContent("No scheduled run"),
  );
});

it("saves the hard timeout and the policies from Advanced", async () => {
  const server = fakeServer();
  render(tree());
  await openAdvanced();
  type("Hard timeout", "300");
  type("On timeout", "stop");
  type("On error", "stop");
  type("Maximum idle iterations", "3");
  type("AI inactivity timeout", "600");
  save();
  await waitFor(() =>
    expect(server.writes()).toEqual([
      "POST loop/hard-timeout",
      "POST loop/on-timeout",
      "POST loop/on-error",
      "POST loop/max-idle",
      "POST loop/ai-stall-timeout",
    ]),
  );
});

// --- Advanced → Other views, About, Danger ---------------------------------------

it("links the mailbox and the raw views under Advanced", async () => {
  fakeServer();
  render(tree());
  await openAdvanced();
  const links = within(screen.getByRole("navigation", { name: "Other views" }))
    .getAllByRole("link")
    .map((link) => [link.textContent, link.getAttribute("href")]);
  expect(links).toEqual([
    ["Open mailbox", `${BASE}/mailbox`],
    ["Prompt preview", `${BASE}/advanced?view=prompt`],
    ["Context", `${BASE}/advanced?view=context`],
    ["Files", `${BASE}/advanced?view=files`],
    ["Scripts", `${BASE}/advanced?view=scripts`],
    ["Full audit", `${BASE}/advanced?view=audit`],
    ["Raw usage", `${BASE}/advanced?view=usage`],
    ["Raw settings", `${BASE}/advanced?view=raw`],
  ]);
});

it("names the image with its Library link and the host in About", async () => {
  fakeServer();
  render(tree());
  await ready();
  const about = section("About");
  expect(within(about).getByRole("link", { name: "worker:v1" })).toHaveAttribute(
    "href",
    "/library/images/worker/v1",
  );
  expect(about).toHaveTextContent("host Remote");
});

it("deletes the agent from Danger after the confirmation and leaves the route", async () => {
  const server = fakeServer();
  render(tree());
  await ready();
  fireEvent.click(within(section("Danger")).getByRole("button", { name: "Delete agent" }));
  const dialog = await screen.findByRole("alertdialog");
  expect(server.writes()).toEqual([]);
  fireEvent.click(within(dialog).getByRole("button", { name: "Delete agent" }));
  await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent(/^\/$/));
  expect(server.calls.find((c) => c.method === "DELETE")?.path).toMatch(
    /\/api\/agents\/worker\/?\?force=true&purge=true$/,
  );
});
