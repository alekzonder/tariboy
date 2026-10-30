import { render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import SettingsPage from "./SettingsPage";

function renderAt(path: string, hostControl = <button type="button">Host picker</button>) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route
          path="/settings"
          element={
            <SettingsPage
              hostControl={hostControl}
              boundary={(page) => <div data-testid="host-boundary">{page}</div>}
            />
          }
        >
          <Route path="hosts" element={<p>hosts page</p>} />
          <Route path="usage" element={<p>usage page</p>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

describe("SettingsPage", () => {
  it("lists the seven Settings pages in one flat list, Application last", () => {
    renderAt("/settings/hosts");

    const nav = screen.getByRole("navigation", { name: "Settings" });
    const links = within(nav).getAllByRole("link");
    expect(links.map((link) => [link.textContent, link.getAttribute("href")])).toEqual([
      ["Hosts", "/settings/hosts"],
      ["Teams", "/settings/teams"],
      ["Usage & budgets", "/settings/usage"],
      ["Agent defaults", "/settings/agent-defaults"],
      ["Integrations", "/settings/integrations"],
      ["Maintenance", "/settings/maintenance"],
      ["Application", "/app-settings"],
    ]);
    expect(screen.getAllByRole("navigation")).toHaveLength(1);
    for (const gone of ["Operator tools", "General", "CLI", "Appearance", "Advanced", "Daemons"]) {
      expect(screen.queryByText(gone)).not.toBeInTheDocument();
    }
  });

  it("keeps the Hosts page outside the host boundary and without the host selector", () => {
    renderAt("/settings/hosts");

    expect(screen.getByText("hosts page")).toBeInTheDocument();
    expect(screen.queryByTestId("host-boundary")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Host picker" })).not.toBeInTheDocument();
  });

  it("puts a host page inside the host boundary and its host selector above it", () => {
    renderAt("/settings/usage");

    const boundary = screen.getByTestId("host-boundary");
    expect(within(boundary).getByText("usage page")).toBeInTheDocument();
    // An unreachable host replaces the page, never the way to pick another.
    const picker = screen.getByRole("button", { name: "Host picker" });
    expect(boundary).not.toContainElement(picker);
  });
});
