import { afterEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { render, screen, within } from "@/test/queryRender";
import { ServerContextBar } from "./ServerContextBar";

vi.mock("@/hooks/useMailSocket", () => ({ useMailSocket: () => "open" }));
afterEach(() => vi.unstubAllGlobals());

describe("ServerContextBar", () => {
  it("keeps server-owned navigation scoped to the selected server", () => {
    render(
      <MemoryRouter initialEntries={["/agents/remote-1/worker/console"]}>
        <ServerContextBar hostId="remote-1" label="Production" />
      </MemoryRouter>,
    );

    expect(screen.getByRole("navigation", { name: "Server workspace" })).toBeInTheDocument();
    expect(screen.getByText("Server:")).toBeInTheDocument();
    expect(screen.getByText("Production")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Mailbox" })).toHaveAttribute(
      "href",
      "/servers/remote-1/mail",
    );
    expect(screen.getByRole("link", { name: "Tasks" })).toHaveAttribute(
      "href",
      "/servers/remote-1/tasks",
    );
    expect(screen.getByRole("link", { name: "Images" })).toHaveAttribute(
      "href",
      "/servers/remote-1/images",
    );
    expect(screen.getByRole("link", { name: "Workflow images" })).toHaveAttribute(
      "href",
      "/servers/remote-1/workflows",
    );
    expect(screen.getAllByRole("link").map((link) => link.textContent)).toEqual([
      "Mailbox",
      "Tasks",
      "Activity",
      "Images",
      "Workflow images",
      "Stores",
      "Settings",
    ]);
    expect(screen.getByRole("link", { name: "Activity" })).toHaveAttribute(
      "href",
      "/servers/remote-1/activity",
    );
    expect(screen.getByRole("link", { name: "Settings" })).toHaveAttribute(
      "href",
      "/servers/remote-1/settings",
    );
  });

  it("shows the customer's open letter count on Mailbox", async () => {
    const fetch = vi.fn(async () => ({
      ok: true,
      status: 200,
      text: async () =>
        JSON.stringify({
          ok: true,
          result: { as: "user:customer", open: 3, unread: 1, by_sender: [] },
        }),
    }));
    vi.stubGlobal("fetch", fetch);
    render(
      <MemoryRouter>
        <ServerContextBar hostId="" label="This daemon (local)" />
      </MemoryRouter>,
    );

    const link = screen.getByRole("link", { name: /Mailbox/ });
    expect(await within(link).findByRole("img", { name: "3 open letters" })).toHaveTextContent("3");
    expect(fetch).toHaveBeenCalledWith("/api/mail/unread", expect.anything());
  });
});
