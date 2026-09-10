import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Outlet, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SettingsPage, { GeneralSettings } from "./SettingsPage";
import {
  getGlobalAgentShellScriptOn,
  getPluginContributionsOn,
  setGlobalAgentShellScriptOn,
} from "@/lib/api";

vi.mock("@/lib/api", async (importOriginal) => ({
  ...await importOriginal<typeof import("@/lib/api")>(),
  getGlobalAgentShellScriptOn: vi.fn(),
  getPluginContributionsOn: vi.fn(),
  setGlobalAgentShellScriptOn: vi.fn(),
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getGlobalAgentShellScriptOn).mockResolvedValue({ script: "export OK=1" });
  vi.mocked(getPluginContributionsOn).mockResolvedValue({ plugins: [], count: 0 });
  vi.mocked(setGlobalAgentShellScriptOn).mockResolvedValue({ script: "export OK=1" });
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
    expect(screen.getByRole("link", { name: "Advanced" }))
      .toHaveAttribute("href", "/servers/remote-1/settings/advanced");
    expect(screen.getByRole("link", { name: "Usage" }))
      .toHaveAttribute("href", "/servers/remote-1/settings/advanced/usage");
  });

  it("adds settings contributions under the selected server", async () => {
    vi.mocked(getPluginContributionsOn).mockResolvedValue({
      plugins: [{ name: "telegram", settings: { title: "Telegram" } }], count: 1,
    });
    render(
      <MemoryRouter initialEntries={["/servers/remote-1/settings"]}>
        <Routes>
          <Route
            path="/servers/remote-1/settings/*"
            element={<SettingsPage basePath="/servers/remote-1/settings" target={null} />}
          />
        </Routes>
      </MemoryRouter>,
    );

    expect(await screen.findByRole("link", { name: "Telegram" }))
      .toHaveAttribute("href", "/servers/remote-1/settings/integrations/telegram");
    expect(getPluginContributionsOn).toHaveBeenCalledWith(null);
  });

  it("keeps an invalid global shell draft and shows Bash stderr", async () => {
    const target = {
      id: "remote-1",
      label: "Remote",
      baseURL: "https://remote.test",
      token: "secret",
    };
    vi.mocked(setGlobalAgentShellScriptOn).mockRejectedValue(
      new Error("line 1: syntax error"),
    );
    render(
      <MemoryRouter>
        <Routes>
          <Route element={<Outlet context={target} />}>
            <Route index element={<GeneralSettings />} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    fireEvent.change(
      await screen.findByLabelText("Global Agent Shell Script"),
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
    expect(getGlobalAgentShellScriptOn).toHaveBeenCalledWith(target);
    expect(setGlobalAgentShellScriptOn).toHaveBeenCalledWith(target, "if then");
  });
});
