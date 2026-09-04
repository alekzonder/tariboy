import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SettingsPage from "./SettingsPage";
import { getPluginContributionsOn } from "@/lib/api";

vi.mock("@/lib/api", async (importOriginal) => ({
  ...await importOriginal<typeof import("@/lib/api")>(),
  getPluginContributionsOn: vi.fn(),
}));

beforeEach(() => {
  vi.mocked(getPluginContributionsOn).mockResolvedValue({ plugins: [], count: 0 });
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
});
