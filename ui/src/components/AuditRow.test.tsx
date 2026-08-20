import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AuditRow } from "./AuditRow";
import type { AuditEvent, DisplayRow } from "@/lib/audit";

const ev = (over: Partial<AuditEvent>): AuditEvent =>
  ({ seq: 1, kind: "harness_output", source: "harness", at: "2026-07-09T09:50:01.000Z", data: "{}", iteration_id: "it-1", ...over });
const eventRow = (e: AuditEvent): DisplayRow => ({ kind: "event", key: e.seq, event: e });

describe("AuditRow", () => {
  it("renders a chat-style line: time, icon, label, preview", () => {
    const e = ev({ kind: "status", data: JSON.stringify({ message: "checking state" }) });
    render(<AuditRow row={eventRow(e)} open={false} onToggle={() => {}} />);
    const btn = screen.getByRole("button");
    expect(btn.textContent).toContain("09:50:01");
    expect(btn.textContent).toContain("📍");
    expect(btn.textContent).toContain("status");
    expect(btn.textContent).toContain("checking state");
  });

  it("accents iteration boundary rows (semibold), plain otherwise", () => {
    const { rerender } = render(<AuditRow row={eventRow(ev({ kind: "iteration_started", data: JSON.stringify({ trigger: "manual" }) }))} open={false} onToggle={() => {}} />);
    expect(screen.getByRole("button").className).toContain("font-semibold");
    expect(screen.getByRole("button").className).toContain("bg-muted/60");
    rerender(<AuditRow row={eventRow(ev({ kind: "harness_output" }))} open={false} onToggle={() => {}} />);
    expect(screen.getByRole("button").className).not.toContain("bg-muted/60");
  });

  it("shows pretty JSON only when open and toggles with the row key", async () => {
    const onToggle = vi.fn();
    const e = ev({ seq: 7, kind: "iteration_started", data: JSON.stringify({ trigger: "manual" }) });
    const { rerender } = render(<AuditRow row={eventRow(e)} open={false} onToggle={onToggle} />);
    expect(screen.queryByText(/"trigger": "manual"/)).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button"));
    expect(onToggle).toHaveBeenCalledWith(7);
    rerender(<AuditRow row={eventRow(e)} open={true} onToggle={onToggle} />);
    expect(screen.getByText(/"trigger": "manual"/)).toBeInTheDocument();
  });

  it("renders a thinking marker with the summed token count", async () => {
    const events = [ev({ seq: 2, kind: "harness_output" }), ev({ seq: 3, kind: "harness_output" })];
    const row: DisplayRow = { kind: "thinking", key: 2, events, tokens: 2100 };
    const onToggle = vi.fn();
    const { rerender } = render(<AuditRow row={row} open={false} onToggle={onToggle} />);
    const btn = screen.getByRole("button");
    expect(btn.textContent).toContain("🧠");
    expect(btn.textContent).toContain("2.1k");
    await userEvent.click(btn);
    expect(onToggle).toHaveBeenCalledWith(2);
    rerender(<AuditRow row={row} open={true} onToggle={onToggle} />);
    expect(screen.getByText(/"seq": 2/)).toBeInTheDocument();
  });

  it("renders Codex command executions as labeled commands with readable completion output", () => {
    const command = "rg -n 'AuditRow' ui/src";
    const e = ev({
      kind: "harness_output",
      data: JSON.stringify({ line: JSON.stringify({ type: "item.completed", item: { type: "command_execution", command, status: "completed", aggregated_output: "ui/src/components/AuditRow.tsx: command row" } }) }),
    });
    render(<AuditRow row={eventRow(e)} open onToggle={() => {}} />);
    expect(screen.getByRole("button").textContent).toContain("command");
    expect(screen.getByRole("button").textContent).toContain(command);
    expect(screen.getByRole("button").textContent).toContain("completed");
    expect(screen.getByRole("button")).toHaveAttribute("aria-label", expect.stringContaining("completed"));
    expect(screen.getByLabelText("Command details").textContent).toContain("Output:");
    expect(screen.getByText(/Raw event data/)).toBeInTheDocument();
  });

  it("renders Codex agent messages as their human-readable text", () => {
    const e = ev({
      kind: "harness_output",
      data: JSON.stringify({ line: JSON.stringify({ type: "item.completed", item: { type: "agent_message", text: "I updated the audit row." } }) }),
    });
    render(<AuditRow row={eventRow(e)} open={false} onToggle={() => {}} />);
    expect(screen.getByRole("button").textContent).toContain("message");
    expect(screen.getByRole("button").textContent).toContain("I updated the audit row.");
    expect(screen.getByRole("button").textContent).not.toContain("item.completed");
  });

  it("renders Codex MCP tools, skills, reasoning, and file changes as named activity", () => {
    const cases = [
      {
        item: { type: "mcp_tool_call", server: "codex_apps", tool: "search", arguments: { query: "audit" }, status: "completed" },
        expected: ["Tool", "codex_apps · search", "audit", "completed"],
      },
      {
        item: { type: "skill", name: "brainstorming", status: "completed" },
        expected: ["Skill", "brainstorming", "completed"],
      },
      {
        item: { type: "reasoning", text: "Inspecting the audit implementation" },
        expected: ["Thinking", "Inspecting the audit implementation"],
      },
      {
        item: { type: "file_change", changes: [{ path: "ui/src/lib/audit.ts", kind: "update" }], status: "completed" },
        expected: ["File change", "ui/src/lib/audit.ts", "completed"],
      },
    ];
    const { rerender } = render(<AuditRow row={eventRow(ev({}))} open={false} onToggle={() => {}} />);
    for (const [index, testCase] of cases.entries()) {
      const event = ev({ seq: 20 + index, data: JSON.stringify({ line: JSON.stringify({ type: "item.completed", item: testCase.item }) }) });
      rerender(<AuditRow row={eventRow(event)} open={false} onToggle={() => {}} />);
      const text = screen.getByRole("button").textContent ?? "";
      for (const expected of testCase.expected) expect(text).toContain(expected);
      expect(text).not.toContain("item.completed");
    }
  });

  it("keeps malformed harness JSON on the generic row without throwing", () => {
    render(<AuditRow row={eventRow(ev({ data: JSON.stringify({ line: "{not valid JSON" }) }))} open={false} onToggle={() => {}} />);
    const button = screen.getByRole("button");
    expect(button.textContent).toContain("harness");
    expect(button.textContent).toContain("{not valid JSON");
  });

  it("keeps unknown Codex item and event types on their generic rows", () => {
    const unknownItem = ev({
      data: JSON.stringify({ line: JSON.stringify({ type: "item.completed", item: { type: "file_change", path: "README.md" } }) }),
    });
    const { rerender } = render(<AuditRow row={eventRow(unknownItem)} open={false} onToggle={() => {}} />);
    expect(screen.getByRole("button").textContent).toContain("item.completed");
    rerender(<AuditRow row={eventRow(ev({ kind: "future_event", data: JSON.stringify({ message: "still visible" }) }))} open={false} onToggle={() => {}} />);
    expect(screen.getByRole("button").textContent).toContain("future_event");
    expect(screen.getByRole("button").textContent).toContain("still visible");
  });

  it("uses command fallback statuses for started and non-zero-exit events", () => {
    const started = ev({
      data: JSON.stringify({ line: JSON.stringify({ type: "item.started", item: { type: "command_execution", command: "npm test" } }) }),
    });
    const { rerender } = render(<AuditRow row={eventRow(started)} open={false} onToggle={() => {}} />);
    expect(screen.getByRole("button").textContent).toContain("running");
    expect(screen.getByRole("button")).toHaveAttribute("aria-label", expect.stringContaining("running"));

    const failed = ev({
      data: JSON.stringify({ line: JSON.stringify({ type: "item.completed", item: { type: "command_execution", command: "npm test", exit_code: 1 } }) }),
    });
    rerender(<AuditRow row={eventRow(failed)} open={false} onToggle={() => {}} />);
    expect(screen.getByRole("button").textContent).toContain("failed");
    expect(screen.getByRole("button")).toHaveAttribute("aria-label", expect.stringContaining("failed"));
  });
});
