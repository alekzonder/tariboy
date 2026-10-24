import { fireEvent, render as rtlRender, screen, waitFor, type RenderOptions } from "@testing-library/react";
import type { ReactElement } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SettingsPage, { GeneralSettings } from "./SettingsPage";
import type { ApiClient } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";
import { apiWrapper, fakeApi } from "@/test/api";

const getGlobalAgentShellScript = vi.fn();
const setGlobalAgentShellScript = vi.fn();
const getPluginContributions = vi.fn();

function client(target: Daemon | null = null): ApiClient {
  return fakeApi({
    harnesses: { getCodexApproval: vi.fn().mockResolvedValue({ mode: "yolo" }) },
    daemon: {
      getGlobalAgentShellScript,
      setGlobalAgentShellScript,
      getIterationRetentionDefault: vi.fn().mockResolvedValue({ keep_iterations: 0, keep_days: 0, max_bytes: 0 }),
      getMaintenance: vi.fn().mockResolvedValue({
        settings: { enabled: true, time: "03:00", keep_backups: 7, retention_days: 90, chat_retention_days: 7, compact: true, compact_threshold_pct: 10 },
        last_run: null,
      }),
    },
    plugins: { getPluginContributions },
  }, target);
}

function render(element: ReactElement, options: RenderOptions & { api?: ApiClient } = {}) {
  const { api = client(), ...rest } = options;
  return rtlRender(element, { ...rest, wrapper: apiWrapper(api) });
}

beforeEach(() => {
  vi.clearAllMocks();
  getGlobalAgentShellScript.mockResolvedValue({ script: "export OK=1" });
  getPluginContributions.mockResolvedValue({ plugins: [], count: 0 });
  setGlobalAgentShellScript.mockResolvedValue({ saved: true });
});

describe("SettingsPage", () => {
  it("keeps settings navigation inside the explicit server base path", () => {
    render(
      <MemoryRouter initialEntries={["/servers/remote-1/settings/advanced"]}>
        <Routes>
          <Route
            path="/servers/remote-1/settings/*"
            element={<SettingsPage basePath="/servers/remote-1/settings" />}
          />
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByRole("link", { name: "General" }))
      .toHaveAttribute("href", "/servers/remote-1/settings");
    expect(screen.queryByRole("link", { name: "Task reminders" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Rules" })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Advanced" }))
      .toHaveAttribute("href", "/servers/remote-1/settings/advanced");
    expect(screen.getByRole("link", { name: "Usage" }))
      .toHaveAttribute("href", "/servers/remote-1/settings/advanced/usage");
    expect(screen.queryByRole("link", { name: "Evals" })).not.toBeInTheDocument();
  });

  it("adds settings contributions under the selected server", async () => {
    getPluginContributions.mockResolvedValue({
      plugins: [{ name: "telegram", settings: { title: "Telegram" } }], count: 1,
    });
    render(
      <MemoryRouter initialEntries={["/servers/remote-1/settings"]}>
        <Routes>
          <Route
            path="/servers/remote-1/settings/*"
            element={<SettingsPage basePath="/servers/remote-1/settings" />}
          />
        </Routes>
      </MemoryRouter>,
    );

    expect(await screen.findByRole("link", { name: "Telegram" }))
      .toHaveAttribute("href", "/servers/remote-1/settings/integrations/telegram");
    expect(getPluginContributions).toHaveBeenCalledWith();
  });

  it("keeps an invalid global shell draft and shows Bash stderr", async () => {
    const target = {
      id: "remote-1",
      label: "Remote",
      baseURL: "https://remote.test",
      token: "secret",
    };
    setGlobalAgentShellScript.mockRejectedValue(
      new Error("line 1: syntax error"),
    );
    render(
      <MemoryRouter>
        <Routes>
          <Route index element={<GeneralSettings />} />
        </Routes>
      </MemoryRouter>,
      { api: client(target) },
    );

    const script = await screen.findByLabelText("Global Agent Shell Script");
    await waitFor(() => expect(script).toBeEnabled());
    fireEvent.change(
      script,
      {
        target: { value: "if then" },
      },
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Save global agent shell script" }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "line 1: syntax error",
    );
    expect(screen.getByLabelText("Global Agent Shell Script")).toHaveValue(
      "if then",
    );
    expect(getGlobalAgentShellScript).toHaveBeenCalledWith();
    expect(setGlobalAgentShellScript).toHaveBeenCalledWith("if then");
  });

  it("blocks shell-script editing and saving until its initial load succeeds", async () => {
    const target = {
      id: "remote-1",
      label: "Remote",
      baseURL: "https://remote.test",
      token: "secret",
    };
    let resolveLoad!: (value: { script: string }) => void;
    getGlobalAgentShellScript.mockReturnValue(
      new Promise((resolve) => { resolveLoad = resolve; }),
    );
    render(
      <MemoryRouter>
        <Routes>
          <Route index element={<GeneralSettings />} />
        </Routes>
      </MemoryRouter>,
      { api: client(target) },
    );

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
    render(
      <MemoryRouter>
        <Routes>
          <Route index element={<GeneralSettings />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(await screen.findByRole("alert")).toHaveTextContent("load failed");
    expect(screen.getByLabelText("Global Agent Shell Script")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Save global agent shell script" })).toBeDisabled();
  });
});
