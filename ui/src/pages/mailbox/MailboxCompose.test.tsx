import { beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import userEvent from "@testing-library/user-event";
import { render, screen, waitFor } from "@/test/queryRender";
import { fakeApi } from "@/test/api";
import { letter } from "@/test/fixtures/mailFixtures";
import Mailbox from "./Mailbox";

vi.mock("@/hooks/useMailSocket", () => ({ useMailSocket: () => "open" }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

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
        hostId=""
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
      }),
    );
  });

  it("does not send an empty letter", async () => {
    renderMailbox();
    await userEvent.click(await screen.findByRole("button", { name: "Compose" }));
    expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
  });
});
