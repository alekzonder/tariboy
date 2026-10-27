import { describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import userEvent from "@testing-library/user-event";
import { render, screen } from "@/test/queryRender";
import { fakeApi } from "@/test/api";
import { letter } from "@/test/fixtures/mailFixtures";
import type { Letter, MailBox } from "@/lib/api";
import { MailboxLetter } from "./MailboxLetter";

const handlers = {
  onReply: vi.fn(),
  onArchive: vi.fn(),
  onRequeue: vi.fn(),
  onOpenTask: vi.fn(),
  busy: false,
};

function show(item: Letter, box: MailBox = "inbox", mode: "customer" | "agent" = "customer") {
  const show = vi.fn(async () => ({ letter: letter({ subject: "The question" }) }));
  render(
    <MemoryRouter>
      <MailboxLetter letter={item} box={box} mode={mode} as="" handlers={handlers} />
    </MemoryRouter>,
    { api: fakeApi({ mail: { show } }) },
  );
  return show;
}

describe("MailboxLetter", () => {
  it("answers a task question on its task, not by Reply", async () => {
    show(letter({ kind: "task.question", data: { task_key: "DEV-7" } }));

    await userEvent.click(screen.getByRole("button", { name: "Answer on the task" }));
    expect(handlers.onOpenTask).toHaveBeenCalledWith("DEV-7");
    expect(screen.queryByRole("button", { name: "Reply" })).toBeNull();
  });

  it("opens the task of a letter about a task", async () => {
    show(letter({ kind: "task.done", needs_reply: false, data: { task_key: "DEV-8" } }));

    await userEvent.click(screen.getByRole("button", { name: "Open DEV-8" }));
    expect(handlers.onOpenTask).toHaveBeenCalledWith("DEV-8");
    expect(screen.queryByRole("button", { name: "Reply" })).toBeNull();
  });

  it("offers Reply and Archive on an open request and shows its thread", async () => {
    const thread = show(letter({ in_reply_to: "m0", body: "**Bold** answer" }));

    expect(screen.getByRole("button", { name: "Reply" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Archive" })).toBeEnabled();
    expect(screen.getByText("Bold").tagName).toBe("STRONG");
    expect(await screen.findByText("In reply to “The question”")).toBeVisible();
    expect(thread).toHaveBeenCalledWith("m0", undefined, expect.any(AbortSignal));
  });

  it("has no actions on a sent letter", () => {
    show(letter(), "sent");

    expect(screen.queryByRole("button")).toBeNull();
  });
});
