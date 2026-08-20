import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import SettingsPage from "./SettingsPage";

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
    expect(screen.getByRole("link", { name: "Advanced" }))
      .toHaveAttribute("href", "/servers/remote-1/settings/advanced");
    expect(screen.getByRole("link", { name: "Usage" }))
      .toHaveAttribute("href", "/servers/remote-1/settings/advanced/usage");
  });
});
