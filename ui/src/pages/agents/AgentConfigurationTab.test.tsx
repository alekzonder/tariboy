import { afterEach, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { AgentNameContext, AgentStatusContext } from "@/lib/agent";
import { MemoryRouter } from "react-router-dom";
import type { Daemon } from "@/lib/daemons";
import type { AgentStatus, AgentView } from "@/lib/types";
import AgentConfigurationTab from "./AgentConfigurationTab";

vi.mock("@/pages/AgentSettings", () => ({
  default: ({ target }: { target?: Daemon | null }) => (
    <div
      data-testid="agent-settings"
      data-target-id={target?.id ?? ""}
      data-target-base-url={target?.baseURL ?? ""}
      data-target-token={target?.token ?? ""}
    />
  ),
}));

afterEach(() => vi.restoreAllMocks());

const remote = {
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
  loop_enabled: false,
  enabled: false,
  interval_s: 0,
  timeout_s: 60,
  hard_timeout_s: 120,
  on_timeout: "restart",
  on_error: "restart",
  max_idle_iterations: 0,
  user_prompt: "",
  env: {},
  plugins: [],
  goal_enabled: true,
  goal_wait_customer_timeout_s: 300,
  current_goal_task_key: "",
  group: null,
  alias: "",
  notes: "",
};

function response(result: unknown, ok = true, status = 200) {
  const body = ok
    ? { ok: true, result }
    : { ok: false, error: result };
  return Promise.resolve({
    ok,
    status,
    text: async () => JSON.stringify(body),
  } as Response);
}

function renderConfiguration(refresh = vi.fn()) {
  const view = render(
    <AgentNameContext.Provider value="worker">
      <AgentConfigurationTab target={remote} refresh={refresh} />
    </AgentNameContext.Provider>,
  );
  return { ...view, refresh };
}

it("passes the route's remote request target to Agent settings", async () => {
  vi.stubGlobal("fetch", vi.fn((url: string) => {
    if (url.includes("/api/fs/list")) {
      return response({ path: "/srv", parent: "/", entries: [] });
    }
    return response(stopped);
  }));

  renderConfiguration();

  await screen.findByTestId("master-switch-state");
  expect(screen.getByTestId("agent-settings")).toHaveAttribute("data-target-id", "remote-1");
  expect(screen.getByTestId("agent-settings")).toHaveAttribute("data-target-base-url", "https://remote.example");
  expect(screen.getByTestId("agent-settings")).toHaveAttribute("data-target-token", "secret");
});

it("shows both run flags with their controls and the explanation line", async () => {
  vi.stubGlobal("fetch", vi.fn((url: string) => {
    if (url.includes("/api/fs/list")) {
      return response({ path: "/srv", parent: "/", entries: [] });
    }
    return response({ ...stopped, enabled: false, loop_enabled: true });
  }));

  renderConfiguration();

  expect(
    await screen.findByText(
      "The master switch permits the agent to run; Loop schedules new autonomous iterations.",
    ),
  ).toBeInTheDocument();
  // Both flags, together: the master switch is off while Loop is on.
  expect(screen.getByTestId("master-switch-state")).toHaveTextContent("Disabled");
  expect(screen.getByTestId("loop-state")).toHaveTextContent("Enabled");
  // Each keeps its own existing control...
  expect(screen.getByRole("button", { name: "Start" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Stop" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Restart" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Disable" })).toBeInTheDocument();
  // ...and neither drags along the destructive actions of AgentControls.
  expect(screen.queryByRole("button", { name: "Kill" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Remove" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Exec" })).not.toBeInTheDocument();
});

it("enables the loop from the run state strip and reloads the agent", async () => {
  let current = { ...stopped, loop_enabled: false };
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    if (url.endsWith("/api/agents/worker/loop/enable") && init?.method === "POST") {
      current = { ...current, loop_enabled: true };
      return response({});
    }
    if (url.includes("/api/fs/list")) {
      return response({ path: "/srv", parent: "/", entries: [] });
    }
    return response(current);
  });
  vi.stubGlobal("fetch", fetchMock);

  renderConfiguration();

  fireEvent.click(await screen.findByRole("button", { name: "Enable" }));

  await waitFor(() =>
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/agents/worker/loop/enable",
      expect.objectContaining({ method: "POST" }),
    ),
  );
  expect(await screen.findByRole("button", { name: "Disable" })).toBeInTheDocument();
  expect(screen.getByTestId("loop-state")).toHaveTextContent("Enabled");
});

it("truncates the image digest while exposing the complete value, and copies it", async () => {
  const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.assign(navigator, { clipboard: { writeText } });
  vi.stubGlobal("fetch", vi.fn((url: string) => {
    if (url.includes("/api/fs/list")) {
      return response({ path: "/srv", parent: "/", entries: [] });
    }
    return response({ ...stopped, digest });
  }));

  renderConfiguration();

  // The visible value is shortened...
  const shown = await screen.findByTestId("image-digest");
  expect(shown.textContent).not.toBe(digest);
  expect(shown.textContent).toContain("sha256:0123");
  expect(shown.textContent).toContain("abcdef");
  // ...while assistive technology still gets the whole digest: the shortened
  // value is hidden from the accessibility tree and the full one is announced.
  expect(shown).toHaveAttribute("aria-hidden", "true");
  const full = screen.getByText(digest);
  expect(full).toHaveClass("sr-only");

  fireEvent.click(screen.getByRole("button", { name: "Copy digest" }));
  await waitFor(() => expect(writeText).toHaveBeenCalledWith(digest));
  expect(await screen.findByText("Digest copied")).toBeInTheDocument();
});

it("selects a built image for the next iteration and links current and pending images", async () => {
  let pending = { ref: "", digest: "", error: "" };
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    if (url === "https://remote.example/api/agents/worker/image") {
      if (init?.method === "POST") {
        expect(JSON.parse(String(init.body))).toEqual({ image: "reviewer:v2" });
        pending = { ref: "reviewer:v2", digest: "sha256:v2", error: "" };
      }
      return response({
        name: "worker",
        current: { ref: "worker:v1", digest: "sha256:abc", error: "" },
        pending,
      });
    }
    if (url === "https://remote.example/api/images") {
      return response({ images: [
        { name: "worker", tag: "v1", digest: "sha256:abc" },
        { name: "reviewer", tag: "v2", digest: "sha256:v2" },
      ] });
    }
    if (url.includes("/api/fs/list")) {
      return response({ path: "/srv", parent: "/", entries: [] });
    }
    return response(stopped);
  });
  vi.stubGlobal("fetch", fetchMock);

  render(
    <MemoryRouter>
      <AgentNameContext.Provider value="worker">
        <AgentConfigurationTab target={remote} refresh={vi.fn()} />
      </AgentNameContext.Provider>
    </MemoryRouter>,
  );

  expect(await screen.findByRole("link", { name: "worker:v1" }))
    .toHaveAttribute("href", "/servers/remote-1/images/worker/v1");
  fireEvent.click(screen.getByRole("combobox", { name: "Agent image" }));
  fireEvent.click(await screen.findByRole("option", { name: "reviewer:v2" }));
  fireEvent.click(screen.getByRole("button", { name: "Use next iteration" }));

  expect(await screen.findByText("Pending:")).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "reviewer:v2" }))
    .toHaveAttribute("href", "/servers/remote-1/images/reviewer/v2");
  expect(fetchMock).toHaveBeenCalledWith(
    "https://remote.example/api/agents/worker/image",
    expect.objectContaining({ method: "POST" }),
  );
});

it("edits a stopped agent CWD using suggestions and save requests from its route host", async () => {
  let current = stopped;
  const refresh = vi.fn();
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    if (url === "https://remote.example/api/agents/worker" && init?.method === "GET") {
      return response(current);
    }
    if (url === "https://remote.example/api/fs/list?path=%2Fsrv%2F" && init?.method === "GET") {
      return response({
        path: "/srv",
        parent: "/",
        entries: [{ name: "new", dir: true }],
      });
    }
    if (url === "https://remote.example/api/agents/worker/cwd" && init?.method === "POST") {
      expect(init.headers).toMatchObject({ Authorization: "Bearer secret" });
      expect(JSON.parse(String(init.body))).toEqual({ value: "/srv/new" });
      current = { ...current, cwd: "/srv/new" };
      return response({ name: "worker", cwd: "/srv/new" });
    }
    return response({ code: "unexpected", message: `${init?.method} ${url}` }, false, 500);
  });
  vi.stubGlobal("fetch", fetchMock);

  renderConfiguration(refresh);

  const input = await screen.findByLabelText("Working directory");
  expect(input).toHaveValue("/srv/old");
  await waitFor(() =>
    expect(fetchMock).toHaveBeenCalledWith(
      "https://remote.example/api/fs/list?path=%2Fsrv%2F",
      expect.objectContaining({ method: "GET" }),
    ),
  );

  fireEvent.change(input, { target: { value: "/srv/new" } });
  expect(await screen.findByRole("option", { name: "new" })).toBeInTheDocument();
  fireEvent.change(input, { target: { value: "  /srv/new  " } });
  fireEvent.click(screen.getByRole("button", { name: "Save working directory" }));

  await waitFor(() => expect(refresh).toHaveBeenCalledOnce());
  expect(await screen.findByLabelText("Working directory")).toHaveValue("/srv/new");
});

it("keeps an unsaved CWD draft when its route host is re-rendered", async () => {
  const fetchMock = vi.fn((url: string) => {
    if (url.includes("/api/fs/list")) {
      return response({ path: "/srv", parent: "/", entries: [] });
    }
    return response(stopped);
  });
  vi.stubGlobal("fetch", fetchMock);

  const { rerender } = renderConfiguration();
  const input = await screen.findByLabelText("Working directory");
  fireEvent.change(input, { target: { value: "/srv/new" } });

  rerender(
    <AgentNameContext.Provider value="worker">
      <AgentConfigurationTab target={{ ...remote }} refresh={vi.fn()} />
    </AgentNameContext.Provider>,
  );

  await act(async () => {
    await Promise.resolve();
  });
  expect(fetchMock.mock.calls.filter(
    ([url]) => url === "https://remote.example/api/agents/worker",
  )).toHaveLength(1);
  expect(input).toHaveValue("/srv/new");
});

it("keeps CWD read-only while the agent is active", async () => {
  vi.stubGlobal("fetch", vi.fn(() => response({ ...stopped, state: "running", enabled: true })));

  renderConfiguration();

  expect(await screen.findByText("/srv/old")).toBeInTheDocument();
  expect(screen.queryByLabelText("Working directory")).not.toBeInTheDocument();
  expect(screen.getByText("Stop the agent before changing its working directory.")).toBeInTheDocument();
});

it("switches to read-only when live status reports that the agent started", async () => {
  const status: AgentStatus = {
    name: "worker",
    state: "running",
    loop_enabled: false,
    iterations: 1,
    last_iteration: null,
    last_iteration_id: null,
    status_message: "",
    status_updated: "",
  };
  vi.stubGlobal("fetch", vi.fn((url: string) => {
    if (url.includes("/api/fs/list")) {
      return response({ path: "/srv", parent: "/", entries: [] });
    }
    return response(stopped);
  }));

  render(
    <AgentNameContext.Provider value="worker">
      <AgentStatusContext.Provider value={{ status, refresh: async () => {} }}>
        <AgentConfigurationTab target={remote} refresh={vi.fn()} />
      </AgentStatusContext.Provider>
    </AgentNameContext.Provider>,
  );

  expect(await screen.findByText("/srv/old")).toBeInTheDocument();
  expect(screen.queryByLabelText("Working directory")).not.toBeInTheDocument();
  expect(screen.getByText("Stop the agent before changing its working directory.")).toBeInTheDocument();
});

it("uses stopped state as a compatibility fallback when enabled is absent", async () => {
  const legacy = { ...stopped };
  delete legacy.enabled;
  vi.stubGlobal("fetch", vi.fn((url: string) => {
    if (url.includes("/api/fs/list")) {
      return response({ path: "/srv", parent: "/", entries: [] });
    }
    return response(legacy);
  }));

  renderConfiguration();

  expect(await screen.findByLabelText("Working directory")).toHaveValue("/srv/old");
  expect(screen.getByRole("button", { name: "Save working directory" })).toBeInTheDocument();
});

it("shows a save error without discarding the entered CWD", async () => {
  vi.stubGlobal("fetch", vi.fn((url: string, init?: RequestInit) => {
    if (url.endsWith("/api/agents/worker/cwd") && init?.method === "POST") {
      return response(
        { code: "bad_cwd", message: "directory does not exist" },
        false,
        400,
      );
    }
    if (url.includes("/api/fs/list")) {
      return response({ path: "/srv", parent: "/", entries: [] });
    }
    return response(stopped);
  }));

  renderConfiguration();
  const input = await screen.findByLabelText("Working directory");
  fireEvent.change(input, { target: { value: "/srv/missing" } });
  fireEvent.click(screen.getByRole("button", { name: "Save working directory" }));

  // The section copy leads, but the server's reason is still shown verbatim —
  // the operator needs to know WHICH path was rejected and why.
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Working directory was not saved. Fix the path and try again.",
  );
  expect(await screen.findByRole("alert")).toHaveTextContent("directory does not exist");
  expect(input).toHaveValue("/srv/missing");
});

it("shows a budget save error without discarding the entered limit", async () => {
  const budget = {
    hour_usd: 1, day_usd: 0, week_usd: 0, month_usd: 0,
    hour_spent_usd: 0.25, day_spent_usd: 0.25, week_spent_usd: 0.25, month_spent_usd: 0.25,
    exhausted: [],
  };
  vi.stubGlobal("fetch", vi.fn((url: string, init?: RequestInit) => {
    if (url.endsWith("/api/agents/worker/budget") && init?.method === "POST") {
      return response({ code: "bad_budget", message: "budget update rejected" }, false, 400);
    }
    if (url.includes("/api/fs/list")) {
      return response({ path: "/srv", parent: "/", entries: [] });
    }
    return response({ ...stopped, budget });
  }));

  renderConfiguration();
  const input = await screen.findByLabelText("Hour budget");
  fireEvent.change(input, { target: { value: "2" } });
  fireEvent.click(screen.getByRole("button", { name: "Save agent budgets" }));

  expect(await screen.findByRole("alert")).toHaveTextContent("Agent budgets were not saved.");
  expect(screen.getByRole("alert")).toHaveTextContent("budget update rejected");
  expect(input).toHaveValue(2);
});

it("renders a budget projection from an older daemon without exhausted periods", async () => {
  // Removing the defensive fallback in AgentConfigurationTab should make this
  // fail by dereferencing the omitted additive field.
  const legacyBudget = {
    hour_usd: 1, day_usd: 0, week_usd: 0, month_usd: 0,
    hour_spent_usd: 0.25, day_spent_usd: 0.25, week_spent_usd: 0.25, month_spent_usd: 0.25,
  };
  vi.stubGlobal("fetch", vi.fn((url: string) => {
    if (url.includes("/api/fs/list")) {
      return response({ path: "/srv", parent: "/", entries: [] });
    }
    return response({ ...stopped, budget: legacyBudget });
  }));

  renderConfiguration();

  expect(await screen.findByRole("heading", { name: "Agent budgets (USD)" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Save agent budgets" })).toBeInTheDocument();
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
});
