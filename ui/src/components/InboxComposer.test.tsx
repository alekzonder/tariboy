import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { InboxComposer } from "./InboxComposer";

let lastUrl = "";
let lastBody: any = null;

beforeEach(() => {
  lastUrl = "";
  lastBody = null;
  vi.stubGlobal("fetch", vi.fn().mockImplementation((url: string, init?: RequestInit) => {
    lastUrl = url;
    lastBody = init?.body ? JSON.parse(init.body as string) : null;
    return Promise.resolve({
      ok: true, status: 200,
      text: async () => JSON.stringify({ ok: true, result: { sent: true } }),
    } as Response);
  }));
});
afterEach(() => vi.restoreAllMocks());

describe("InboxComposer", () => {
  it("disables Send when the input is empty", () => {
    render(<InboxComposer name="dev-worker" />);
    expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
  });

  it("posts the text to the agent inbox channel with type=message and clears", async () => {
    render(<InboxComposer name="dev-worker" />);
    const input = screen.getByPlaceholderText("message to inbox…");
    await userEvent.type(input, "do the thing");
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(lastUrl).toContain("/api/messages"));
    expect(lastBody).toMatchObject({
      channel: "agent:dev-worker:inbox",
      type: "message",
      text: "do the thing",
    });
    await waitFor(() => expect((input as HTMLInputElement).value).toBe(""));
  });
});
