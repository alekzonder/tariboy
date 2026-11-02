import { beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { fireEvent, render, screen, waitFor } from "@/test/queryRender";
import { fakeApi } from "@/test/api";
import { letter } from "@/test/fixtures/mailFixtures";
import Mailbox from "./Mailbox";

vi.mock("@/hooks/useMailSocket", () => ({ useMailSocket: () => "open" }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const DELIVER = "Deliver to the current iteration";
const HINT =
  "The agent sees this letter after its current tool call. If no iteration is running, the letter starts one, like any other letter.";
const WARNING =
  "Over 8,000 characters. The agent sees the first 8,000 in this iteration and reads the full letter with tmail show.";

const empty = { as: "", box: "inbox", letters: [], count: 0, next_before: "" };
const mail = {
  list: vi.fn(async () => empty),
  unread: vi.fn(async () => ({ as: "", open: 0, unread: 0, by_sender: [] })),
  send: vi.fn(async () => ({ letters: [letter()] })),
};

function renderMailbox(fixedTo?: string) {
  render(
    <MemoryRouter>
      <Mailbox
        as={fixedTo ?? ""}
        mode={fixedTo ? "agent" : "customer"}
        fixedTo={fixedTo}
        recipients={["agent:dev", "agent:ops"]}
      />
    </MemoryRouter>,
    { api: fakeApi({ mail }) },
  );
}

async function write(subject: string, text: string) {
  await userEvent.click(await screen.findByRole("button", { name: "Compose" }));
  await userEvent.type(screen.getByLabelText("Subject"), subject);
  await userEvent.type(screen.getByLabelText("Text"), text);
}

beforeEach(() => vi.clearAllMocks());

describe("MailboxCompose", () => {
  it("sends a request by default", async () => {
    renderMailbox();
    await write("Status", "How is it going?");
    await userEvent.selectOptions(screen.getByLabelText("To"), "agent:ops");
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(mail.send).toHaveBeenCalledWith({
        to: ["agent:ops"],
        subject: "Status",
        text: "How is it going?",
        notice: false,
        instant: false,
      }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("sends a notice with the switch, to the fixed recipient, as the customer", async () => {
    renderMailbox("agent:dev");
    await write("FYI", "Deploy is done");
    expect(screen.queryByLabelText("To")).toBeNull();
    await userEvent.click(screen.getByRole("switch", { name: "No reply needed" }));
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(mail.send).toHaveBeenCalledWith({
        to: ["agent:dev"],
        subject: "FYI",
        text: "Deploy is done",
        notice: true,
        instant: false,
      }),
    );
  });

  it("offers instant delivery under No reply needed, off by default, with its hint", async () => {
    renderMailbox();
    await userEvent.click(await screen.findByRole("button", { name: "Compose" }));

    const instant = screen.getByRole("switch", { name: DELIVER });
    expect(instant).not.toBeChecked();
    const notice = screen.getByRole("switch", { name: "No reply needed" });
    expect(notice.compareDocumentPosition(instant)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
    expect(instant).toHaveAccessibleDescription(HINT);
    expect(screen.getByText(HINT)).toBeVisible();
  });

  it("sends an instant letter: Space turns the switch on", async () => {
    renderMailbox();
    await write("Stop", "Wrong branch");
    screen.getByRole("switch", { name: DELIVER }).focus();
    await userEvent.keyboard(" ");
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(mail.send).toHaveBeenCalledWith({
        to: ["agent:dev"],
        subject: "Stop",
        text: "Wrong branch",
        notice: false,
        instant: true,
      }),
    );
  });

  it("offers instant delivery in Agent → Mailbox → Compose", async () => {
    renderMailbox("agent:dev");
    await userEvent.click(await screen.findByRole("button", { name: "Compose" }));
    expect(screen.getByRole("switch", { name: DELIVER })).toBeVisible();
  });

  it("counts the trimmed text in code points only while the switch is on", async () => {
    renderMailbox();
    await userEvent.click(await screen.findByRole("button", { name: "Compose" }));
    const text = screen.getByLabelText("Text");
    fireEvent.change(text, { target: { value: `  ${"a".repeat(1232)}😀😀 \n` } });
    expect(screen.queryByText(/\/ 8,000$/)).toBeNull();

    await userEvent.click(screen.getByRole("switch", { name: DELIVER }));
    expect(screen.getByText("1,234 / 8,000")).toBeVisible();
    expect(text).toHaveAccessibleDescription("1,234 / 8,000");

    await userEvent.click(screen.getByRole("switch", { name: DELIVER }));
    expect(screen.queryByText("1,234 / 8,000")).toBeNull();
    expect(text).toHaveValue(`  ${"a".repeat(1232)}😀😀 \n`);
  });

  it("warns once over 8,000 characters and still sends", async () => {
    renderMailbox();
    await write("Long", "x");
    await userEvent.click(screen.getByRole("switch", { name: DELIVER }));
    const live = screen.getByTestId("instant-limit");
    expect(live).toHaveAttribute("aria-live", "polite");
    expect(live).toBeEmptyDOMElement();

    const text = screen.getByLabelText("Text");
    fireEvent.change(text, { target: { value: "x".repeat(8001) } });
    expect(screen.getByText("8,001 / 8,000")).toBeVisible();
    expect(live).toHaveTextContent(WARNING);
    const announced = live.innerHTML;

    // More typing changes the counter, outside the live region, never the warning.
    fireEvent.change(text, { target: { value: "x".repeat(8412) } });
    expect(screen.getByText("8,412 / 8,000")).toBeVisible();
    expect(live.innerHTML).toBe(announced);
    expect(live).not.toContainElement(screen.getByText("8,412 / 8,000"));
    expect(screen.getByRole("button", { name: "Send" })).toBeEnabled();
  });

  it("keeps the dialog and every value when sending fails", async () => {
    mail.send.mockRejectedValueOnce(new Error("instant delivery is not allowed from this sender"));
    renderMailbox();
    await write("Stop", "Wrong branch");
    await userEvent.click(screen.getByRole("switch", { name: "No reply needed" }));
    await userEvent.click(screen.getByRole("switch", { name: DELIVER }));
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        "send failed: Error: instant delivery is not allowed from this sender",
      ),
    );
    expect(screen.getByRole("dialog")).toBeVisible();
    expect(screen.getByLabelText("Subject")).toHaveValue("Stop");
    expect(screen.getByLabelText("Text")).toHaveValue("Wrong branch");
    expect(screen.getByRole("switch", { name: "No reply needed" })).toBeChecked();
    expect(screen.getByRole("switch", { name: DELIVER })).toBeChecked();
  });

  it("opens each new dialog with instant delivery off", async () => {
    renderMailbox();
    await userEvent.click(await screen.findByRole("button", { name: "Compose" }));
    await userEvent.click(screen.getByRole("switch", { name: DELIVER }));
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await userEvent.click(screen.getByRole("button", { name: "Compose" }));
    expect(screen.getByRole("switch", { name: DELIVER })).not.toBeChecked();
  });

  it("moves focus To → Subject → text → both switches → Cancel → Send", async () => {
    renderMailbox();
    await userEvent.click(await screen.findByRole("button", { name: "Compose" }));
    await userEvent.click(screen.getByRole("switch", { name: DELIVER }));
    fireEvent.change(screen.getByLabelText("Text"), { target: { value: "x" } });
    screen.getByLabelText("To").focus();
    const order = [
      screen.getByLabelText("Subject"),
      screen.getByLabelText("Text"),
      screen.getByRole("switch", { name: "No reply needed" }),
      screen.getByRole("switch", { name: DELIVER }),
      screen.getByRole("button", { name: "Cancel" }),
      screen.getByRole("button", { name: "Send" }),
    ];
    for (const element of order) {
      await userEvent.tab();
      expect(element).toHaveFocus();
    }
  });

  it("does not send an empty letter", async () => {
    renderMailbox();
    await userEvent.click(await screen.findByRole("button", { name: "Compose" }));
    expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
  });
});
