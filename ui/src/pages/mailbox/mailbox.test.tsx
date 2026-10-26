import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import userEvent from "@testing-library/user-event";
import { act, render, screen, waitFor, within } from "@/test/queryRender";
import { fakeApi } from "@/test/api";
import { letter } from "@/test/fixtures/mailFixtures";
import type { Letter, MailBox, MailListQuery } from "@/lib/api";
import Mailbox from "./Mailbox";

const socket = vi.hoisted(() => ({
  onHint: undefined as undefined | ((hint: { from: string; to: string }) => void),
}));
vi.mock("@/hooks/useMailSocket", () => ({
  useMailSocket: (options: { onHint: (hint: { from: string; to: string }) => void }) => {
    socket.onHint = options.onHint;
    return "open";
  },
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

let boxes: Record<MailBox, Letter[]>;
const mail = {
  list: vi.fn(async ({ box }: MailListQuery) => ({
    as: "",
    box,
    letters: boxes[box],
    count: boxes[box].length,
    next_before: "",
  })),
  unread: vi.fn(async () => ({
    as: "",
    open: boxes.inbox.length,
    unread: boxes.inbox.filter((item) => !item.read_at).length,
    by_sender: [],
  })),
  read: vi.fn(async () => ({ read: 1, closed_notices: 0 })),
  close: vi.fn(),
  reply: vi.fn(),
  requeue: vi.fn(),
  send: vi.fn(async () => ({ letters: [] })),
  show: vi.fn(async () => ({ letter: letter({ subject: "Earlier" }) })),
};

function renderMailbox(props: Partial<Parameters<typeof Mailbox>[0]> = {}) {
  return render(
    <MemoryRouter>
      <Mailbox as="" mode="customer" hostId="" recipients={["agent:dev"]} {...props} />
    </MemoryRouter>,
    { api: fakeApi({ mail }) },
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  boxes = {
    inbox: [letter({ id: "req", subject: "Need input" })],
    sent: [letter({ id: "out", subject: "Sent one", from: "user:customer", to: "agent:ops" })],
    archive: [letter({ id: "old", subject: "Done long ago", answered_at: "x", answer_id: "r" })],
  };
});
afterEach(() => vi.restoreAllMocks());

describe("Mailbox", () => {
  it("lists Inbox, Sent and Archive of the principal it acts as", async () => {
    renderMailbox({ as: "agent:dev", mode: "agent" });

    expect(await screen.findByText("Need input")).toBeVisible();
    expect(screen.getByRole("radio", { name: "Inbox 1" })).toBeChecked();
    await userEvent.click(screen.getByRole("radio", { name: "Sent" }));
    expect(await screen.findByText("Sent one")).toBeVisible();
    expect(screen.getByText("ops")).toBeVisible();
    await userEvent.click(screen.getByRole("radio", { name: "Archive" }));
    expect(await screen.findByText("Done long ago")).toBeVisible();
    expect(mail.list.mock.calls.map(([query]) => [query.box, query.as])).toEqual([
      ["inbox", "agent:dev"],
      ["sent", "agent:dev"],
      ["archive", "agent:dev"],
    ]);
  });

  it("shows the list and the open letter side by side", async () => {
    renderMailbox();

    await userEvent.click(await screen.findByRole("button", { name: /Need input/ }));
    const pane = screen.getByRole("region", { name: "Letter" });
    expect(within(pane).getByRole("heading", { name: "Need input" })).toBeVisible();
    expect(screen.getByRole("region", { name: "Letter list" })).toBeVisible();
    expect(within(pane).queryByRole("button", { name: "Back" })).toBeNull();
  });

  it("alternates list and letter on a narrow screen", async () => {
    vi.spyOn(globalThis, "matchMedia").mockImplementation(
      (query: string) =>
        ({
          matches: true,
          media: query,
          addEventListener: () => {},
          removeEventListener: () => {},
        }) as unknown as MediaQueryList,
    );
    renderMailbox();

    await userEvent.click(await screen.findByRole("button", { name: /Need input/ }));
    expect(screen.queryByRole("region", { name: "Letter list" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Back" }));
    expect(screen.getByRole("region", { name: "Letter list" })).toBeVisible();
    expect(screen.queryByRole("region", { name: "Letter" })).toBeNull();
  });

  it("reads an unread letter on open; a read notice moves to Archive but stays open", async () => {
    boxes.inbox = [letter({ id: "note", subject: "FYI", needs_reply: false })];
    renderMailbox();

    await userEvent.click(await screen.findByRole("button", { name: /FYI/ }));
    await waitFor(() => expect(mail.read).toHaveBeenCalledWith(["note"], undefined));
    boxes.archive = [{ ...boxes.inbox[0], read_at: "now", answered_at: "now", close_note: "read" }];
    boxes.inbox = [];
    await act(async () => socket.onHint?.({ from: "agent:dev", to: "user:customer" }));

    await waitFor(() => expect(screen.getByText("No open letters.")).toBeVisible());
    expect(screen.getByRole("heading", { name: "FYI" })).toBeVisible();
  });

  it("keeps a request in the Inbox after reading until Reply or Archive", async () => {
    renderMailbox();

    await userEvent.click(await screen.findByRole("button", { name: /Need input/ }));
    expect(mail.read).toHaveBeenCalledWith(["req"], undefined);
    expect(screen.getByRole("button", { name: /Need input/ })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Archive" }));
    expect(mail.close).toHaveBeenCalledWith("req", { as: undefined });
  });

  it("refetches on a hint for its principal and ignores other agents' mail", async () => {
    renderMailbox({ as: "agent:dev", mode: "agent" });
    await screen.findByText("Need input");
    const calls = mail.list.mock.calls.length;

    await act(async () => socket.onHint?.({ from: "agent:ops", to: "agent:qa" }));
    expect(mail.list).toHaveBeenCalledTimes(calls);
    await act(async () => socket.onHint?.({ from: "user:customer", to: "agent:dev" }));
    await waitFor(() => expect(mail.list.mock.calls.length).toBeGreaterThan(calls));
  });
});
