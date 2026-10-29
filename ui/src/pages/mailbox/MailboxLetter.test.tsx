import { describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import userEvent from "@testing-library/user-event";
import { fireEvent, render, screen, waitFor } from "@/test/queryRender";
import { fakeApi } from "@/test/api";
import { letter } from "@/test/fixtures/mailFixtures";
import type { Letter, MailBox, Task, TaskDetail } from "@/lib/api";
import { MailboxLetter } from "./MailboxLetter";

const handlers = {
  onReply: vi.fn(),
  onArchive: vi.fn(),
  onRequeue: vi.fn(),
  onOpenTask: vi.fn(),
  onAnswer: vi.fn(async () => true),
  busy: false,
};

const taskOf = (task: Partial<Task>) => ({ task }) as TaskDetail;

function show(
  item: Letter,
  box: MailBox = "inbox",
  mode: "customer" | "agent" = "customer",
  getTask = vi.fn(async () => taskOf({ key: "DEV-7", title: "Ship it", status: "plan" })),
) {
  const show = vi.fn(async () => ({ letter: letter({ subject: "The question" }) }));
  render(
    <MemoryRouter>
      <MailboxLetter letter={item} box={box} mode={mode} as="" handlers={handlers} />
    </MemoryRouter>,
    { api: fakeApi({ mail: { show }, tasks: { getTask } }) },
  );
  return show;
}

describe("MailboxLetter", () => {
  it("answers a task question in the letter with a comment on its task, not by Reply", async () => {
    const question = letter({ kind: "task.question", data: { task_key: "DEV-7" } });
    show(question);

    await userEvent.click(screen.getByRole("button", { name: "Markdown" }));
    fireEvent.change(await screen.findByLabelText("Answer"), { target: { value: "Yes" } });
    await userEvent.click(screen.getByRole("button", { name: "Send answer" }));
    expect(handlers.onAnswer).toHaveBeenCalledWith(question, "Yes", expect.any(String));
    expect(screen.queryByRole("button", { name: "Answer on the task" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Reply" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Open DEV-7" }));
    expect(handlers.onOpenTask).toHaveBeenCalledWith("DEV-7");
  });

  it("sends an agent's task question to its task: the operator does not answer for the agent", async () => {
    show(letter({ kind: "task.question", data: { task_key: "DEV-7" } }), "inbox", "agent");

    expect(screen.queryByRole("button", { name: "Send answer" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Answer on the task" }));
    expect(handlers.onOpenTask).toHaveBeenCalledWith("DEV-7");
  });

  it("shows the key, current title and status of the letter's task", async () => {
    show(letter({ kind: "task.question", data: { task_key: "DEV-7", task_title: "Old title" } }));

    expect(await screen.findByText("DEV-7 · Ship it · plan")).toBeVisible();
  });

  it("falls back to the title the letter carries when the task does not load", async () => {
    const getTask = vi.fn(async (): Promise<TaskDetail> => {
      throw new Error("gone");
    });
    show(
      letter({ kind: "task.done", data: { task_key: "DEV-7", task_title: "Old title" } }),
      "inbox",
      "customer",
      getTask,
    );

    await waitFor(() => expect(getTask).toHaveBeenCalled());
    expect(await screen.findByText("DEV-7 · Old title")).toBeVisible();
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
