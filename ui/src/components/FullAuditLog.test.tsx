import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { FullAuditLog } from "./FullAuditLog";

const mk = (seq: number, kind: string, extra: Record<string, unknown> = {}) =>
  ({ seq, kind, source: "system", at: `t${seq}`, data: JSON.stringify(extra), iteration_id: "it" });

// A full recent page (50 events, seq 100..51 newest-first) → older history
// exists, so hasMore=true. before= returns the chronological older window.
const RECENT = Array.from({ length: 50 }, (_, i) =>
  i === 0 ? mk(100, "iteration_finished", { status: "done" }) : mk(100 - i, "harness_output", { line: "x" }));
const OLDER = [mk(40, "harness_output", { line: "a" }), mk(50, "iteration_started", { trigger: "manual" })];

let calls: string[] = [];
let recent = RECENT;
beforeEach(() => {
  calls = [];
  recent = RECENT;
  vi.stubGlobal("EventSource", class { addEventListener() {} removeEventListener() {} close() {} } as unknown as typeof EventSource);
  vi.stubGlobal("fetch", vi.fn().mockImplementation((url: string) => {
    calls.push(url);
    let result: unknown = { events: [] };
    if (url.includes("distinct=types")) result = { types: ["status", "harness_output"] };
    else if (url.includes("type=status")) result = { events: [mk(100, "status", { message: "reviewing" })], capped: false };
    else if (url.includes("q=boom")) result = { events: [mk(3, "harness_output", { line: "boom" })], capped: false };
    else if (url.includes("before=")) result = { events: OLDER };
    else if (url.includes("since=")) result = { events: [] };
    else result = { events: recent };
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }) } as Response);
  }));
});
afterEach(() => vi.restoreAllMocks());

const renderLog = () => render(<FullAuditLog name="dev-worker" />);
const auditRows = () => screen.getAllByRole("button").filter((button) => /event details/i.test(button.getAttribute("aria-label") ?? ""));

describe("FullAuditLog (paged)", () => {
  it("renders the initial recent page chronological (newest at bottom)", async () => {
    renderLog();
    await waitFor(() => expect(screen.getByText("done")).toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Copy audit log" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Export audit log" })).toBeInTheDocument();
    const rows = auditRows();
    expect(rows).toHaveLength(50);
    expect(rows[0].textContent).toContain("t51");             // seq 51, oldest recent, top
    expect(rows[rows.length - 1].textContent).toContain("done"); // seq 100, newest, bottom (iteration ⏹ done)
  });

  it("loads older events with before= when scrolled to the top and prepends them", async () => {
    renderLog();
    await waitFor(() => expect(screen.getByText("done")).toBeInTheDocument());
    const viewport = document.querySelector('[data-slot="scroll-area-viewport"]')!;
    fireEvent.scroll(viewport); // jsdom scrollTop stays 0 → treated as "at top"
    await waitFor(() => expect(calls.some((u) => u.includes("before=51"))).toBe(true));
    await waitFor(() => expect(screen.getByText("manual")).toBeInTheDocument());
    const rows = auditRows();
    expect(rows[0].textContent).toContain("t40"); // seq 40 now at the top
  });

  it("filters by type server-side and shows only the matches", async () => {
    recent = [RECENT[0]];
    renderLog();
    await waitFor(() => expect(screen.getByText("done")).toBeInTheDocument());
    fireEvent.change(screen.getByLabelText("Filter by type"), { target: { value: "status" } });
    await waitFor(() => expect(calls.some((u) => u.includes("type=status"))).toBe(true));
    // The filtered pane shows the single status match and the match count.
    await waitFor(() => expect(screen.getByText("reviewing")).toBeInTheDocument());
    expect(screen.getByText(/1 match/)).toBeInTheDocument();
    // The recent-50 harness rows are gone; only the match remains.
    expect(screen.queryByText("done")).not.toBeInTheDocument();
  });

  it("full-text search composes across fields and clears back to the live view", async () => {
    recent = [RECENT[0]];
    renderLog();
    await waitFor(() => expect(screen.getByText("done")).toBeInTheDocument());
    fireEvent.change(screen.getByLabelText("Search text"), { target: { value: "boom" } });
    await waitFor(() => expect(calls.some((u) => u.includes("q=boom"))).toBe(true));
    await waitFor(() => expect(screen.getByText(/1 match/)).toBeInTheDocument());
    // Clearing restores the paged live view (recent 50 with the done boundary).
    fireEvent.click(screen.getByText("Clear"));
    await waitFor(() => expect(screen.getByText("done")).toBeInTheDocument());
  });
});
