import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StatusChatHistory } from "./StatusChatHistory";

beforeEach(() => {
  vi.stubGlobal("fetch", vi.fn().mockImplementation(() =>
    Promise.resolve({
      ok: true, status: 200, text: async () => JSON.stringify({
        ok: true, result: { events: [{ ts: "2026-07-09T10:00:00Z", message: "working" }], count: 1 },
      }),
    } as Response)));
});
afterEach(() => vi.restoreAllMocks());

describe("StatusChatHistory", () => {
  it("lazy-loads history when the sheet opens", async () => {
    render(<StatusChatHistory name="foo" />);
    await userEvent.click(screen.getByRole("button", { name: "status history" }));
    await waitFor(() => expect(screen.getByText("working")).toBeInTheDocument());
  });

  it("groups consecutive events by iteration and labels updates outside an iteration", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
      ok: true, status: 200, text: async () => JSON.stringify({
        ok: true, result: { events: [
          { ts: "2026-07-09T10:03:00Z", message: "outside", iteration_id: "" },
          { ts: "2026-07-09T10:02:30Z", message: "legacy" },
          { ts: "2026-07-09T10:02:00Z", message: "second", iteration_id: "iteration-2" },
          { ts: "2026-07-09T10:01:00Z", message: "first", iteration_id: "iteration-2" },
          { ts: "2026-07-09T10:00:00Z", message: "older", iteration_id: "iteration-1" },
        ], count: 4 },
      }),
    } as Response));

    render(<StatusChatHistory name="foo" />);
    await userEvent.click(screen.getByRole("button", { name: "status history" }));

    await waitFor(() => expect(screen.getByText("outside")).toBeInTheDocument());
    expect(screen.getByText("legacy")).toBeInTheDocument();
    expect(screen.getAllByText("Iteration: iteration-2")).toHaveLength(1);
    expect(screen.getByText("Iteration: iteration-1")).toBeInTheDocument();
    expect(screen.getByText("Outside an iteration")).toBeInTheDocument();
    expect(screen.getAllByText(/Iteration:|Outside an iteration/)).toHaveLength(3);
    expect(screen.getByText("second").compareDocumentPosition(screen.getByText("first")) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("shows an empty-state message when there are no status events", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
      ok: true, status: 200, text: async () => JSON.stringify({
        ok: true, result: { events: [], count: 0 },
      }),
    } as Response));

    render(<StatusChatHistory name="foo" />);
    await userEvent.click(screen.getByRole("button", { name: "status history" }));

    await waitFor(() => expect(screen.getByText("No status events.")).toBeInTheDocument());
  });
});
