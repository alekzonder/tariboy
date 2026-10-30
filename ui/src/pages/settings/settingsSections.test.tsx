import {
  fireEvent,
  render as rtlRender,
  screen,
  waitFor,
  within,
  type RenderOptions,
} from "@testing-library/react";
import type { ReactElement } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  AgentDefaultsSettings,
  IntegrationsSettings,
  MaintenanceSettings,
  UsageSettings,
} from "./settingsSections";
import type { ApiClient } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";
import { apiWrapper, fakeApi } from "@/test/api";

vi.mock("@/pages/UsagePage", () => ({ default: () => <h2>Usage</h2> }));
vi.mock("@/pages/BudgetsPage", () => ({ default: () => <h2>Budgets</h2> }));
vi.mock("@/pages/PluginsPage", () => ({ default: () => <p>installed plugins list</p> }));
vi.mock("@/pages/OpsPage", () => ({ default: () => <h2>Backup and restore</h2> }));
vi.mock("@/pages/settings/operations/OperationsPage", () => ({
  default: () => <h2>Operations log</h2>,
}));
vi.mock("@/pages/settings/PluginSettings", () => ({
  default: ({ name }: { name: string }) => <p>settings of {name}</p>,
}));
vi.mock("@/components/SupportBundle", () => ({ SupportBundle: () => <h2>Support bundle</h2> }));

const getGlobalAgentShellScript = vi.fn();
const setGlobalAgentShellScript = vi.fn();
const getPluginContributions = vi.fn();

function client(target: Daemon | null = null): ApiClient {
  return fakeApi(
    {
      harnesses: { getCodexApproval: vi.fn().mockResolvedValue({ mode: "yolo" }) },
      daemon: {
        getGlobalAgentShellScript,
        setGlobalAgentShellScript,
        getIterationRetentionDefault: vi
          .fn()
          .mockResolvedValue({ keep_iterations: 0, keep_days: 0, max_bytes: 0 }),
        getMaintenance: vi.fn().mockResolvedValue({
          settings: {
            enabled: true,
            time: "03:00",
            keep_backups: 7,
            retention_days: 90,
            chat_retention_days: 7,
            compact: true,
            compact_threshold_pct: 10,
          },
          last_run: null,
        }),
      },
      plugins: { getPluginContributions },
    },
    target,
  );
}

function render(element: ReactElement, options: RenderOptions & { api?: ApiClient } = {}) {
  const { api = client(), ...rest } = options;
  return rtlRender(element, { ...rest, wrapper: apiWrapper(api) });
}

function renderPage(path: string, route: string, page: ReactElement, api?: ApiClient) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path={route} element={page} />
      </Routes>
    </MemoryRouter>,
    { api },
  );
}

const remote = { id: "remote-1", label: "Remote", baseURL: "https://remote.test", token: "secret" };

beforeEach(() => {
  vi.clearAllMocks();
  getGlobalAgentShellScript.mockResolvedValue({ script: "export OK=1" });
  getPluginContributions.mockResolvedValue({ plugins: [], count: 0 });
  setGlobalAgentShellScript.mockResolvedValue({ saved: true });
});

describe("Usage & budgets", () => {
  it("shows Usage and Budgets as two sections of one page", () => {
    renderPage("/settings/usage", "/settings/usage", <UsageSettings />);

    expect(screen.getByRole("heading", { level: 1, name: "Usage & budgets" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 2, name: "Usage" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 2, name: "Budgets" })).toBeInTheDocument();
  });
});

describe("Maintenance", () => {
  it("holds backup and restore, the operations log and the support bundle", () => {
    renderPage("/settings/maintenance", "/settings/maintenance", <MaintenanceSettings />);

    expect(screen.getByRole("heading", { level: 1, name: "Maintenance" })).toBeInTheDocument();
    expect(
      screen.getAllByRole("heading", { level: 2 }).map((heading) => heading.textContent),
    ).toEqual(["Backup and restore", "Operations log", "Support bundle"]);
  });
});

describe("Integrations", () => {
  const telegram = { plugins: [{ name: "telegram", settings: { title: "Telegram" } }], count: 1 };

  it("opens on the installed plugins with a tab per plugin that has settings", async () => {
    getPluginContributions.mockResolvedValue(telegram);
    renderPage(
      "/settings/integrations",
      "/settings/integrations/:plugin?",
      <IntegrationsSettings />,
    );

    const tabs = screen.getByRole("navigation", { name: "Integrations" });
    expect(await within(tabs).findByRole("link", { name: "Telegram" })).toHaveAttribute(
      "href",
      "/settings/integrations/telegram",
    );
    expect(within(tabs).getByRole("link", { name: "Installed plugins" })).toHaveAttribute(
      "href",
      "/settings/integrations",
    );
    expect(screen.getByText("installed plugins list")).toBeInTheDocument();
  });

  it("shows a plugin's settings under its tab", async () => {
    getPluginContributions.mockResolvedValue(telegram);
    renderPage(
      "/settings/integrations/telegram",
      "/settings/integrations/:plugin?",
      <IntegrationsSettings />,
    );

    expect(screen.getByText("settings of telegram")).toBeInTheDocument();
    expect(screen.queryByText("installed plugins list")).not.toBeInTheDocument();
    expect(await screen.findByRole("link", { name: "Telegram" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });
});

describe("Agent defaults", () => {
  it("holds the agent settings of the host and no support bundle", async () => {
    renderPage("/settings/agent-defaults", "/settings/agent-defaults", <AgentDefaultsSettings />);

    expect(screen.getByRole("heading", { level: 1, name: "Agent defaults" })).toBeInTheDocument();
    expect(await screen.findByLabelText("Global Agent Shell Script")).toBeInTheDocument();
    expect(screen.queryByText("Support bundle")).not.toBeInTheDocument();
  });

  it("keeps an invalid global shell draft and shows Bash stderr", async () => {
    setGlobalAgentShellScript.mockRejectedValue(new Error("line 1: syntax error"));
    renderPage("/", "/", <AgentDefaultsSettings />, client(remote));

    const script = await screen.findByLabelText("Global Agent Shell Script");
    await waitFor(() => expect(script).toBeEnabled());
    fireEvent.change(script, {
      target: { value: "if then" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save global agent shell script" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("line 1: syntax error");
    expect(screen.getByLabelText("Global Agent Shell Script")).toHaveValue("if then");
    expect(getGlobalAgentShellScript).toHaveBeenCalledWith();
    expect(setGlobalAgentShellScript).toHaveBeenCalledWith("if then");
  });

  it("blocks shell-script editing and saving until its initial load succeeds", async () => {
    let resolveLoad!: (value: { script: string }) => void;
    getGlobalAgentShellScript.mockReturnValue(
      new Promise((resolve) => {
        resolveLoad = resolve;
      }),
    );
    renderPage("/", "/", <AgentDefaultsSettings />, client(remote));

    const script = await screen.findByLabelText("Global Agent Shell Script");
    const save = screen.getByRole("button", { name: "Save global agent shell script" });
    expect(script).toBeDisabled();
    expect(save).toBeDisabled();
    fireEvent.click(save);
    expect(setGlobalAgentShellScript).not.toHaveBeenCalled();
    fireEvent.change(script, { target: { value: "if then" } });
    expect(script).toHaveValue("");

    resolveLoad({ script: "export READY=1" });
    await waitFor(() => expect(script).toBeEnabled());
    expect(script).toHaveValue("export READY=1");
    expect(save).toBeEnabled();
  });

  it("keeps shell-script editing and saving unavailable after load failure", async () => {
    getGlobalAgentShellScript.mockRejectedValue(new Error("load failed"));
    renderPage("/", "/", <AgentDefaultsSettings />);

    expect(await screen.findByRole("alert")).toHaveTextContent("load failed");
    expect(screen.getByLabelText("Global Agent Shell Script")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Save global agent shell script" })).toBeDisabled();
  });
});
