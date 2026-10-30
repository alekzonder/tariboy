import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor, within } from "@/test/queryRender";
import { MemoryRouter, useLocation, useNavigate } from "react-router-dom";
import type { ReactNode } from "react";
import { fetchAllAgents } from "@/lib/aggregate";
import App from "./App";
import { UI_MODE_KEY } from "@/lib/uiMode";
import { UI_VERSION } from "@/lib/onboarding/version";

vi.mock("@/lib/aggregate", () => ({
  fetchAllAgents: vi.fn(),
}));

vi.mock("@/components/CustomerQuestionNotifications", () => ({
  CustomerQuestionNotifications: ({ children }: { children: ReactNode }) => children,
}));

function LocationProbe() {
  const location = useLocation();
  return <output data-testid="location">{location.pathname + location.search}</output>;
}

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <App />
      <LocationProbe />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  // The tour starts on a first run; only its own test wants it.
  localStorage.setItem("onboarding:state:v1", UI_VERSION);
  vi.mocked(fetchAllAgents).mockResolvedValue([
    { host: { id: "", label: "This daemon (local)" }, agents: [] },
  ]);
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      const result = url.includes("/api/usage")
        ? {
            total_requests: 0,
            total_cost_usd: 0,
            total_input_tokens: 0,
            total_output_tokens: 0,
            total_cache_write_tokens: 0,
            total_cache_read_tokens: 0,
            rows: [],
            series: [],
            requests: [],
          }
        : url.includes("/api/groups")
          ? { groups: [], count: 0 }
          : { agents: [], count: 0 };
      return {
        ok: true,
        status: 200,
        text: async () => JSON.stringify({ ok: true, result }),
      } as Response;
    }),
  );
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("product routing", () => {
  it("hides Workspace and keeps the sidebar control in the titlebar", () => {
    renderAt("/");

    const titlebar = screen.getByTestId("app-titlebar");
    for (const label of ["Workspace", "Agents", "Tasks", "Images", "Settings"]) {
      expect(within(titlebar).queryByRole("link", { name: label })).toBeNull();
    }
    expect(within(titlebar).getByRole("button", { name: "Hide agents" })).toBeInTheDocument();
    expect(within(titlebar).getByRole("button", { name: "Toggle theme" })).toBeInTheDocument();
  });

  it("has no gear, no view switch and no workspace dropdown in the titlebar", () => {
    renderAt("/agents/local/builder/console");

    const titlebar = screen.getByTestId("app-titlebar");
    expect(within(titlebar).getByText("Tariboy")).toBeInTheDocument();
    expect(within(titlebar).queryByRole("link", { name: "Application settings" })).toBeNull();
    expect(within(titlebar).queryByRole("radiogroup", { name: "View" })).toBeNull();
    expect(within(titlebar).queryByRole("button", { name: /^Workspace:/ })).toBeNull();
    expect(
      within(titlebar).getByRole("combobox", { name: "Search agents and tasks" }),
    ).toBeVisible();
  });

  it("opens the former All tasks view as Tasks", async () => {
    renderAt("/agents/local/builder/console?view=all");
    await waitFor(() => expect(screen.getByTestId("location").textContent).toBe("/tasks"));
  });

  it("opens daemon-independent application settings from Settings", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("daemon down")));
    renderAt("/servers/local/settings");

    const settings = await screen.findByRole("link", { name: "Application" });
    expect(settings).toHaveAttribute("href", "/app-settings");
    fireEvent.click(settings);

    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent("/app-settings"));
    expect(screen.getByRole("heading", { name: "Application settings" })).toBeInTheDocument();
    expect(screen.getByText("Updates are only available in the Desktop app.")).toBeInTheDocument();
  });

  it("renders Tasks at /tasks with no server bar", async () => {
    renderAt("/tasks");

    expect(await screen.findByLabelText("Search tasks")).toBeInTheDocument();
    expect(screen.getByTestId("tasks-workspace")).toHaveAttribute("data-scope-agent", "");
    expect(screen.queryByRole("navigation", { name: "Server workspace" })).toBeNull();
    expect(screen.getByRole("link", { name: "Tasks" })).toHaveAttribute("aria-current", "page");
  });

  it("starts no tour on the first run and marks the elements the tour explains", async () => {
    localStorage.removeItem("onboarding:state:v1");
    renderAt("/tasks");

    await screen.findByLabelText("Search tasks");
    expect(screen.queryByRole("dialog", { name: "Welcome to Tariboy" })).not.toBeInTheDocument();
    expect(localStorage.getItem("onboarding:state:v1")).not.toBeNull();
    const anchors = [...document.querySelectorAll("[data-tour]")].map((el) =>
      el.getAttribute("data-tour"),
    );
    for (const anchor of [
      "hosts-line",
      "add-agent",
      "add-host",
      "section-tasks",
      "section-images",
      "queue-menu",
      "help-menu",
    ]) {
      expect(anchors).toContain(anchor);
    }
    expect(
      within(screen.getByTestId("app-titlebar")).getByRole("button", { name: "Help" }),
    ).toBeInTheDocument();
  });

  it.each([
    ["/servers/local/tasks?task=X-1", "/tasks?host=local&task=X-1"],
    ["/servers/local/mail", "/inbox?host=local"],
    ["/servers/remote-1/activity?view=agents", "/activity?host=remote-1&view=agents"],
    ["/settings/advanced/usage?range=7d", "/servers/local/settings/advanced/usage?range=7d"],
  ])("redirects legacy server surface %s without losing its tail", async (from, to) => {
    renderAt(from);
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent(to));
  });

  it("preserves the active remote host when redirecting an image detail", async () => {
    localStorage.setItem("tariboy_active_daemon", "remote-1");
    renderAt("/images/worker/v1/template?raw=1");
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent(
        "/servers/remote-1/images/worker/v1/template?raw=1",
      ),
    );
  });

  it("toggles the Agents sidebar from the left side of the window toolbar", () => {
    renderAt("/");

    const toggle = screen.getByRole("button", { name: "Hide agents" });

    fireEvent.click(toggle);

    expect(screen.getByRole("button", { name: "Show agents" })).toBeInTheDocument();
    expect(JSON.parse(localStorage.getItem("terminals:sidebar:v1")!)).toMatchObject({
      hidden: true,
    });
  });

  it("keeps the macOS titlebar safe and draggable when the daemon banner is visible", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("daemon down")));
    renderAt("/");

    const header = screen.getByTestId("app-titlebar");
    const daemonBanner = await screen.findByText("daemon unreachable — is tariboyd running?");
    expect(header).toHaveAttribute("data-tauri-drag-region", "deep");
    expect(screen.getByRole("button", { name: "Hide agents" })).not.toHaveAttribute(
      "data-tauri-drag-region",
    );
    for (const link of header.querySelectorAll("a")) {
      expect(link).not.toHaveAttribute("data-tauri-drag-region");
    }
    expect(
      header.compareDocumentPosition(daemonBanner) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("uses Home as the default route", () => {
    renderAt("/");
    expect(screen.getByTestId("location")).toHaveTextContent(/^\/$/);
    const sidebar = screen.getByRole("complementary", { name: "Navigation" });
    expect(within(sidebar).getByRole("link", { name: "Home" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    for (const name of ["Tasks", "Inbox", "Library", "Settings"]) {
      expect(within(sidebar).getByRole("link", { name })).toBeInTheDocument();
    }
    expect(within(sidebar).getByRole("button", { name: "Agents" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
  });

  it("redirects a legacy terminal route to the Terminal tab", async () => {
    localStorage.setItem(UI_MODE_KEY, "expert");
    renderAt("/terminals/local/worker");
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/agents/local/worker/console"),
    );
  });

  it("redirects a legacy terminal route to Tasks in Simple mode", async () => {
    renderAt("/terminals/local/worker");
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/agents/local/worker/tasks"),
    );
  });

  it("preserves create preselection while canonicalizing old entry points", async () => {
    renderAt("/terminals?new=1&host=remote-1&image=worker%3Av1");
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent(
        "/?new=1&host=remote-1&image=worker%3Av1",
      ),
    );
  });

  it("preserves image and host parameters from /agents/new", async () => {
    renderAt("/agents/new?image=worker%3Av1&host=remote-1");
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent(
        "/?image=worker%3Av1&host=remote-1&new=1",
      ),
    );
  });

  it("resolves legacy agent tabs against the selected host", async () => {
    localStorage.setItem("tariboy_active_daemon", "remote-1");
    renderAt("/agent/worker/logs");
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/agents/remote-1/worker/activity"),
    );
  });

  it.each([
    ["/usage", "/settings/advanced/usage"],
    ["/groups", "/settings/advanced/groups"],
    ["/ops", "/settings/advanced/ops"],
  ])("redirects operator route %s under Settings", async (from, to) => {
    renderAt(from);
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent(to));
  });

  it("keeps an unknown host id fail-closed instead of rewriting it to local", async () => {
    renderAt("/agents/removed-host/worker/console");
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent(
        "/agents/removed-host/worker/console",
      ),
    );
    expect(screen.getByTestId("location")).not.toHaveTextContent("/agents/local/");
  });
});

function stubMatchingQueries(...matching: string[]) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: matching.includes(query),
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  }));
}

describe("compact screens", () => {
  beforeEach(() => {
    stubMatchingQueries("(max-width: 767.98px)", "(max-width: 1023.98px)");
    vi.mocked(fetchAllAgents).mockResolvedValue([
      {
        host: { id: "", label: "This daemon (local)" },
        agents: [
          {
            name: "alice",
            image: "bare:latest",
            state: "idle",
            harness: "claude",
            group: null,
            interactive: true,
          },
          {
            name: "bob",
            image: "bare:latest",
            state: "idle",
            harness: "claude",
            group: null,
            interactive: true,
          },
        ],
      },
    ] as never);
  });

  it("keeps the agent list behind a menu button and closes it after a pick", async () => {
    renderAt("/agents/local/alice/tasks");

    expect(screen.queryByTestId("agents-sidebar")).toBeNull();
    const titlebar = screen.getByTestId("app-titlebar");
    expect(within(titlebar).queryByRole("button", { name: "Hide agents" })).toBeNull();
    fireEvent.click(within(titlebar).getByRole("button", { name: "Open menu" }));

    const drawer = await screen.findByRole("dialog", { name: "Navigation" });
    expect(within(drawer).getByTestId("agents-sidebar")).toBeInTheDocument();
    // Focus in the search field would raise the on-screen keyboard on a phone.
    expect(document.activeElement).not.toBe(
      within(drawer).getByRole("searchbox", { name: "Search agents" }),
    );
    expect(within(drawer).queryByRole("separator", { name: "resize sidebar" })).toBeNull();
    fireEvent.click(await within(drawer).findByRole("button", { name: "Open bob" }));

    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Navigation" })).toBeNull());
    expect(screen.getByTestId("location")).toHaveTextContent("/agents/local/bob/tasks");
    // The wide layout's saved state is not touched by the drawer.
    expect(localStorage.getItem("terminals:sidebar:v1")).toBeNull();
  });

  it("closes the drawer on Back", async () => {
    function Back() {
      const navigate = useNavigate();
      return (
        <button type="button" onClick={() => navigate(-1)}>
          Back
        </button>
      );
    }
    render(
      <MemoryRouter
        initialEntries={["/agents/local/bob/tasks", "/agents/local/alice/tasks"]}
        initialIndex={1}
      >
        <App />
        <Back />
        <LocationProbe />
      </MemoryRouter>,
    );
    fireEvent.click(
      within(screen.getByTestId("app-titlebar")).getByRole("button", { name: "Open menu" }),
    );
    await screen.findByRole("dialog", { name: "Navigation" });

    // The drawer is modal, so Back comes from outside it, as Android's does.
    act(() => {
      screen.getByText("Back", { selector: "button" }).click();
    });

    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Navigation" })).toBeNull());
    expect(screen.getByTestId("location")).toHaveTextContent("/agents/local/bob/tasks");
  });

  it("drops the macOS traffic-light gap outside the Desktop app", () => {
    renderAt("/");
    expect(screen.queryByTestId("titlebar-window-controls-gap")).toBeNull();
  });
});

describe("medium screens", () => {
  beforeEach(() => {
    stubMatchingQueries("(max-width: 1023.98px)");
    vi.mocked(fetchAllAgents).mockResolvedValue([
      {
        host: { id: "", label: "This daemon (local)" },
        agents: [
          {
            name: "alice",
            image: "bare:latest",
            state: "idle",
            harness: "claude",
            group: null,
            interactive: true,
          },
        ],
      },
    ] as never);
  });

  it("shows the agent column with a toggle that collapses it", async () => {
    renderAt("/agents/local/alice/tasks");

    expect(await screen.findByTestId("agents-sidebar")).toBeInTheDocument();
    const titlebar = screen.getByTestId("app-titlebar");
    expect(within(titlebar).queryByRole("button", { name: "Open menu" })).toBeNull();

    fireEvent.click(within(titlebar).getByRole("button", { name: "Hide agents" }));
    await waitFor(() => expect(screen.queryByTestId("agents-sidebar")).toBeNull());

    fireEvent.click(within(titlebar).getByRole("button", { name: "Show agents" }));
    expect(await screen.findByTestId("agents-sidebar")).toBeInTheDocument();
  });
});
