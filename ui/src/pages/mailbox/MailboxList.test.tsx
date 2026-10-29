import { describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import userEvent from "@testing-library/user-event";
import { render, screen, within } from "@/test/queryRender";
import { fakeApi } from "@/test/api";
import { letter } from "@/test/fixtures/mailFixtures";
import { MailboxLetter } from "./MailboxLetter";
import { MailboxList } from "./MailboxList";

const dead = letter({
  id: "d1",
  subject: "Over the cap",
  dead_at: "2026-10-26T09:00:00Z",
  dead_reason: "queue_limit",
  answered_at: "2026-10-26T09:00:00Z",
});

describe("MailboxList", () => {
  it("tags a dead letter in Archive, and the agent mailbox offers Requeue", async () => {
    const onSelect = vi.fn();
    render(<MailboxList box="archive" letters={[dead]} selected="" onSelect={onSelect} />);

    const row = screen.getByRole("button", { name: /Over the cap/ });
    expect(within(row).getByText("dead · queue_limit")).toBeVisible();
    await userEvent.click(row);
    expect(onSelect).toHaveBeenCalledWith(dead);

    const onRequeue = vi.fn();
    render(
      <MemoryRouter>
        <MailboxLetter
          letter={dead}
          box="archive"
          mode="agent"
          as="agent:dev"
          handlers={{
            onReply: vi.fn(),
            onArchive: vi.fn(),
            onRequeue,
            onOpenTask: vi.fn(),
            onAnswer: vi.fn(),
            busy: false,
          }}
        />
      </MemoryRouter>,
      { api: fakeApi() },
    );
    await userEvent.click(screen.getByRole("button", { name: "Requeue" }));
    expect(onRequeue).toHaveBeenCalledWith(dead);
  });

  it("marks unread inbox letters and names the recipient in Sent", () => {
    const view = render(
      <MailboxList box="inbox" letters={[letter()]} selected="" onSelect={vi.fn()} />,
    );
    expect(screen.getByRole("img", { name: "Unread" })).toBeVisible();
    expect(screen.getByText("dev")).toBeVisible();

    view.rerender(
      <MailboxList
        box="sent"
        letters={[letter({ to: "agent:ops,agent:qa" })]}
        selected=""
        onSelect={vi.fn()}
      />,
    );
    expect(screen.queryByRole("img", { name: "Unread" })).toBeNull();
    expect(screen.getByText("ops, qa")).toBeVisible();
  });

  it("says when a box is empty", () => {
    render(<MailboxList box="archive" letters={[]} selected="" onSelect={vi.fn()} />);
    expect(screen.getByText("No archived letters.")).toBeVisible();
  });
});
