import { beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import userEvent from "@testing-library/user-event";
import { render, screen, waitFor, within } from "@/test/queryRender";
import { fakeApi } from "@/test/api";
import { letter } from "@/test/fixtures/mailFixtures";
import { AgentStatusContext } from "@/lib/agent";
import type { AgentStatus, MailListQuery } from "@/lib/api";
import AgentMailboxTab from "./AgentMailboxTab";

vi.mock("@/hooks/useMailSocket", () => ({ useMailSocket: () => "open" }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const request = letter({ id: "q1", from: "user:customer", to: "agent:dev", subject: "Ping" });
const mail = {
  list: vi.fn(async ({ box }: MailListQuery) => ({
    as: "agent:dev",
    box,
    letters: box === "inbox" ? [request] : [],
    count: box === "inbox" ? 1 : 0,
    next_before: "",
  })),
  unread: vi.fn(async () => ({ as: "agent:dev", open: 1, unread: 1, by_sender: [] })),
  read: vi.fn(async () => ({ read: 1, closed_notices: 0 })),
  reply: vi.fn(async () => ({ closed: true, letter: letter() })),
  send: vi.fn(),
  clear: vi.fn(async () => ({ deleted: 3 })),
};
const refreshStatus = vi.fn(async () => {});

function renderTab() {
  const status = { messages_pending: 3, messages_max_queue: 100 } as AgentStatus;
  render(
    <MemoryRouter>
      <AgentStatusContext.Provider value={{ status, refresh: refreshStatus }}>
        <AgentMailboxTab name="dev" batch={5} base="/agents/remote/dev" />
      </AgentStatusContext.Provider>
    </MemoryRouter>,
    { api: fakeApi({ mail }) },
  );
}

beforeEach(() => vi.clearAllMocks());

describe("AgentMailboxTab", () => {
  it("links back to Diagnostics and Work without sending or clearing mail", async () => {
    renderTab();
    expect(await screen.findByRole("link", { name: "Diagnostics" })).toHaveAttribute(
      "href",
      "/agents/remote/dev/advanced",
    );
    expect(screen.getByRole("link", { name: "Back to Work" })).toHaveAttribute(
      "href",
      "/agents/remote/dev/work",
    );
    await userEvent.click(screen.getByRole("link", { name: "Diagnostics" }));
    await userEvent.click(screen.getByRole("link", { name: "Back to Work" }));
    expect(mail.clear).not.toHaveBeenCalled();
    expect(mail.reply).not.toHaveBeenCalled();
    expect(mail.send).not.toHaveBeenCalled();
  });

  it("shows the batch and the inbox against its cap", async () => {
    renderTab();
    expect(await screen.findByText("batch 5 · inbox 3/100")).toBeVisible();
  });

  it("keeps the destructive confirmation on Clear inbox", async () => {
    renderTab();
    await userEvent.click(await screen.findByRole("button", { name: "Clear inbox" }));
    let dialog = screen.getByRole("dialog", { name: "Clear inbox" });
    await userEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(mail.clear).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole("button", { name: "Clear inbox" }));
    dialog = screen.getByRole("dialog", { name: "Clear inbox" });
    await userEvent.click(within(dialog).getByRole("button", { name: "Clear inbox" }));
    await waitFor(() => expect(mail.clear).toHaveBeenCalledWith("dev"));
    await waitFor(() => expect(refreshStatus).toHaveBeenCalled());
  });

  it("replies on behalf of the agent", async () => {
    renderTab();
    await userEvent.click(await screen.findByRole("button", { name: /Ping/ }));
    expect(mail.read).toHaveBeenCalledWith(["q1"], "agent:dev");
    await userEvent.click(screen.getByRole("button", { name: "Reply" }));
    await userEvent.type(screen.getByLabelText("Text"), "On it");
    await userEvent.click(screen.getByRole("button", { name: "Send reply" }));

    await waitFor(() =>
      expect(mail.reply).toHaveBeenCalledWith("q1", { text: "On it", as: "agent:dev" }),
    );
  });
});
