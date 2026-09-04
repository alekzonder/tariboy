import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { DaemonProvider } from "@/components/DaemonProvider";
import { HostSwitcher } from "@/components/HostSwitcher";
import { AgentNameContext } from "@/lib/agent";
import { addDaemon } from "@/lib/daemons";
import type { AgentView, AgentStatus } from "@/lib/types";
import AgentOverview from "./AgentOverview";
import { useTerminalSocket } from "@/hooks/useTerminalSocket";

vi.mock("@/hooks/useTerminalSocket", () => ({
  useTerminalSocket: vi.fn(() => ({
    status: "closed",
    absent: false,
    send: vi.fn(),
    sendResize: vi.fn(),
    attachTerm: vi.fn(),
    name: "foo",
    reconnect: vi.fn(),
  })),
}));

// Spy EventSource: records every URL a subscription opens against, so we can
// see whether a SECOND stream was opened after a host switch.
class SpyES {
  static urls: string[] = [];
  constructor(url: string) {
    SpyES.urls.push(url);
  }
  addEventListener() {}
  removeEventListener() {}
  close() {}
}

const view: AgentView = {
  name: "foo",
  image: "img",
  digest: "sha256:x",
  state: "running",
  cwd: "/",
  harness: "claude",
  model: "sonnet",
  effort: "medium",
  interactive: false,
  loop_enabled: true,
  interval_s: 60,
  timeout_s: 300,
  hard_timeout_s: 600,
  max_idle_iterations: 0,
  on_timeout: "skip",
  on_error: "skip",
  user_prompt: "",
  env: {},
  plugins: [],
  group: null,
  goal_enabled: true,
  goal_wait_customer_timeout_s: 300,
  goal_delivery_cooldown_s: 60,
  current_goal_task_key: "",
  alias: "",
  notes: "",
};
const status: AgentStatus = {
  name: "foo",
  state: "running",
  loop_enabled: true,
  iterations: 1,
  last_iteration: null,
  last_iteration_id: null,
  status_message: "",
  status_updated: "",
};

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  SpyES.urls = [];
  vi.stubGlobal("EventSource", SpyES as unknown as typeof EventSource);
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((path: string) => {
      const u = path as string;
      // Path-aware: the non-interactive Overview body mounts AuditLogPage (and
      // the InboxComposer), whose fetches must return their own shapes (a bare
      // `view` would break the /logs consumer and unmount the tree).
      let result: unknown;
      if (u.includes("/status/history")) result = { events: [], count: 0 };
      else if (u.endsWith("/status")) result = status;
      else if (u.includes("/subscriptions") || u.includes("/channels"))
        result = { channels: [] };
      else if (u.includes("/logs")) result = { events: [] };
      else if (u.includes("/alias")) result = { name: "foo", alias: "" };
      else if (u.includes("/notes")) result = { name: "foo", notes: "" };
      else if (u.includes("/screen")) result = { screen: "" };
      else result = view;
      return Promise.resolve({
        ok: true,
        status: 200,
        text: async () => JSON.stringify({ ok: true, result }),
      } as Response);
    }),
  );
});
afterEach(() => vi.restoreAllMocks());

describe("AgentOverview re-targets its live SSE stream on a real host switch", () => {
  it("re-opens the SSE stream against the newly-selected daemon after switching via HostSwitcher/context (not the direct setActiveDaemon bypass)", async () => {
    await addDaemon({
      label: "prod",
      baseURL: "https://prod:8765",
      token: "tp",
    });

    // Mirrors App.tsx's structure exactly: AgentOverview and HostSwitcher are
    // both plain (stable) children of DaemonProvider — App.tsx does not make
    // AgentOverview itself a context consumer at the JSX call-site, it relies
    // on the component's own hooks to subscribe. This is the shape that hid
    // the dead-code bug: a provider state change only re-renders children that
    // actually call useDaemons()/useContext internally.
    render(
      <MemoryRouter initialEntries={["/agent/foo"]}>
        <DaemonProvider>
          <HostSwitcher />
          <AgentNameContext.Provider value="foo">
            <AgentOverview />
          </AgentNameContext.Provider>
        </DaemonProvider>
      </MemoryRouter>,
    );

    await waitFor(() => expect(SpyES.urls.length).toBeGreaterThan(0));
    expect(SpyES.urls[0]).toBe(
      "/api/agents/foo/events?types=iteration%2Caudit",
    );

    // Switch the active daemon the way a real user does: through HostSwitcher,
    // which calls the DaemonProvider context's select() — NOT api.ts's
    // setActiveDaemon directly (that bypass is what sse_switch.test.ts uses,
    // and it does not exercise React's re-render path at all).
    // The HostSwitcher combobox is the first in DOM order (rendered before the
    // Overview's model/effort ComboFields, which are also comboboxes).
    await userEvent.click(screen.getAllByRole("combobox")[0]);
    await userEvent.click(await screen.findByText("prod"));

    // A fixed live-SSE re-target must open a SECOND EventSource against the
    // newly active daemon. Pre-fix, AgentOverview reads getActiveDaemon() at
    // render time but never re-renders (it isn't a context consumer), so its
    // SSE effect never re-runs and this never happens — the assertion below
    // is exactly what catches that dead code.
    await waitFor(() => expect(SpyES.urls.length).toBeGreaterThan(1));
    const last = SpyES.urls[SpyES.urls.length - 1];
    expect(last).toContain("https://prod:8765/api/agents/foo/events?");
    expect(last).toContain("types=iteration%2Caudit");
    expect(last).toContain("token=tp");
    expect(vi.mocked(useTerminalSocket).mock.calls.at(-1)?.[2]).toMatchObject({
      label: "prod",
      baseURL: "https://prod:8765",
      token: "tp",
    });
  });
});
