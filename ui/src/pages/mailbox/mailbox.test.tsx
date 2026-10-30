import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, useLocation } from "react-router-dom";
import userEvent from "@testing-library/user-event";
import { act, fireEvent, render, screen, waitFor, within } from "@/test/queryRender";
import { fakeApi } from "@/test/api";
import { letter } from "@/test/fixtures/mailFixtures";
import type { CommentResult, Letter, MailBox, MailListQuery, TaskDetail } from "@/lib/api";
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
vi.mock("@/pages/tasks/TaskDrawer", () => ({
  default: ({ taskKey, onClose }: { taskKey: string; onClose: () => void }) => (
    <div data-testid="task-drawer">
      {taskKey}
      <button onClick={onClose}>Close drawer</button>
    </div>
  ),
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
  show: vi.fn(async (id: string) => ({
    letter: boxes.archive.find((item) => item.id === id) ?? letter({ subject: "Earlier" }),
  })),
};

const tasks = {
  getTask: vi.fn(async () => ({ task: { key: "DEV-7", title: "Pick a DB" } }) as TaskDetail),
  // The daemon closes the question in the comment's transaction.
  addTaskComment: vi.fn(async () => {
    boxes.archive = boxes.inbox.map((item) => ({
      ...item,
      answered_at: "x",
      close_note: "task-comment:9",
    }));
    boxes.inbox = [];
    return {} as CommentResult;
  }),
};

function Location() {
  const location = useLocation();
  return <output data-testid="location">{location.pathname + location.search}</output>;
}

function renderMailbox(props: Partial<Parameters<typeof Mailbox>[0]> = {}) {
  return render(
    <MemoryRouter initialEntries={["/servers/h1/mail"]}>
      <Mailbox as="" mode="customer" recipients={["agent:dev"]} {...props} />
      <Location />
    </MemoryRouter>,
    { api: fakeApi({ mail, tasks }) },
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

  it("opens the letter that letterId names", async () => {
    mail.show.mockResolvedValueOnce({ letter: letter({ id: "req", subject: "Need input" }) });
    renderMailbox({ letterId: "req" });
    const pane = screen.getByRole("region", { name: "Letter" });
    expect(await within(pane).findByRole("heading", { name: "Need input" })).toBeVisible();
    expect(mail.show).toHaveBeenCalledWith("req", undefined);
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

  it("keeps a read customer notice in the Inbox until Archive", async () => {
    boxes.inbox = [letter({ id: "note", subject: "FYI", needs_reply: false })];
    renderMailbox();

    await userEvent.click(await screen.findByRole("button", { name: /FYI/ }));
    await waitFor(() => expect(mail.read).toHaveBeenCalledWith(["note"], undefined));
    boxes.inbox = [{ ...boxes.inbox[0], read_at: "now" }];
    await act(async () => socket.onHint?.({ from: "agent:dev", to: "user:customer" }));

    expect(screen.getByRole("button", { name: /FYI/ })).toBeVisible();
    expect(screen.queryByText("No open letters.")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Archive" }));
    expect(mail.close).toHaveBeenCalledWith("note", { as: undefined });
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

  it("answers a question with a comment on its task and refetches the box and the task", async () => {
    boxes.inbox = [
      letter({ id: "q", subject: "Which DB?", kind: "task.question", data: { task_key: "DEV-7" } }),
    ];
    renderMailbox();

    await userEvent.click(await screen.findByRole("button", { name: /Which DB\?/ }));
    await userEvent.click(screen.getByRole("button", { name: "Markdown" }));
    fireEvent.change(await screen.findByLabelText("Answer"), { target: { value: "Postgres" } });
    const listed = mail.list.mock.calls.length;
    const loaded = tasks.getTask.mock.calls.length;
    await userEvent.click(screen.getByRole("button", { name: "Send answer" }));

    expect(tasks.addTaskComment).toHaveBeenCalledWith("DEV-7", "Postgres", expect.any(String));
    await waitFor(() => expect(mail.list.mock.calls.length).toBeGreaterThan(listed));
    await waitFor(() => expect(tasks.getTask.mock.calls.length).toBeGreaterThan(loaded));
    expect(await screen.findByText("archived · task-comment:9")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Send answer" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Which DB\?/ })).toBeNull();
  });

  it("opens the task of a question over the mailbox and keeps the letter", async () => {
    boxes.inbox = [
      letter({ id: "q", subject: "Which DB?", kind: "task.question", data: { task_key: "DEV-7" } }),
    ];
    renderMailbox();

    await userEvent.click(await screen.findByRole("button", { name: /Which DB\?/ }));
    await userEvent.click(screen.getByRole("button", { name: "Open DEV-7" }));

    expect(await screen.findByTestId("task-drawer")).toHaveTextContent("DEV-7");
    expect(screen.getByTestId("location")).toHaveTextContent("/servers/h1/mail");
    await userEvent.click(screen.getByRole("button", { name: "Close drawer" }));
    expect(screen.queryByTestId("task-drawer")).toBeNull();
    expect(screen.getByRole("heading", { name: "Which DB?" })).toBeVisible();
  });
});
