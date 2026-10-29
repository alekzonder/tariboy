import { describe, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { fireEvent, render, screen, waitFor } from "@/test/queryRender";
import { MailboxTaskAnswer } from "./MailboxTaskAnswer";

async function typeAnswer(text: string) {
  await userEvent.click(screen.getByRole("button", { name: "Markdown" }));
  fireEvent.change(await screen.findByLabelText("Answer"), { target: { value: text } });
}

describe("MailboxTaskAnswer", () => {
  it("sends the answer with one key per draft and clears it after success", async () => {
    const onAnswer = vi.fn(async () => true);
    render(<MailboxTaskAnswer taskKey="DEV-7" busy={false} onAnswer={onAnswer} />);

    expect(screen.getByRole("button", { name: "Send answer" })).toBeDisabled();
    await typeAnswer("Use **Postgres**");
    await userEvent.click(screen.getByRole("button", { name: "Send answer" }));

    expect(onAnswer).toHaveBeenCalledWith("Use **Postgres**", expect.any(String));
    await waitFor(() => expect(screen.getByLabelText("Answer")).toHaveValue(""));
    await typeAnswer("More");
    await userEvent.click(screen.getByRole("button", { name: "Send answer" }));
    const keys = onAnswer.mock.calls.map((call) => (call as unknown as [string, string])[1]);
    expect(keys[1]).not.toBe(keys[0]);
  });

  it("keeps the draft and its key when the answer fails, so a retry is safe", async () => {
    const onAnswer = vi.fn(async () => false);
    render(<MailboxTaskAnswer taskKey="DEV-7" busy={false} onAnswer={onAnswer} />);

    await typeAnswer("Use Postgres");
    await userEvent.click(screen.getByRole("button", { name: "Send answer" }));
    await userEvent.click(screen.getByRole("button", { name: "Send answer" }));

    expect(screen.getByLabelText("Answer")).toHaveValue("Use Postgres");
    const keys = onAnswer.mock.calls.map((call) => (call as unknown as [string, string])[1]);
    expect(keys).toHaveLength(2);
    expect(keys[1]).toBe(keys[0]);
  });
});
