import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { ServerContextBar } from "./ServerContextBar";

describe("ServerContextBar", () => {
  it("keeps server-owned navigation scoped to the selected server", () => {
    render(
      <MemoryRouter initialEntries={["/agents/remote-1/worker/console"]}>
        <ServerContextBar hostId="remote-1" label="Production" />
      </MemoryRouter>,
    );

    expect(screen.getByRole("navigation", { name: "Server workspace" }))
      .toBeInTheDocument();
    expect(screen.getByText("Server:")).toBeInTheDocument();
    expect(screen.getByText("Production")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Tasks" }))
      .toHaveAttribute("href", "/servers/remote-1/tasks");
    expect(screen.getByRole("link", { name: "Images" }))
      .toHaveAttribute("href", "/servers/remote-1/images");
    expect(screen.getByRole("link", { name: "Settings" }))
      .toHaveAttribute("href", "/servers/remote-1/settings");
  });
});
