import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThinkingRow } from "./ThinkingRow";
import type { AuditLogEvent } from "@/lib/audit";

const event: AuditLogEvent = {
  seq: 7,
  kind: "harness_output",
  source: "harness",
  at: "2026-07-09T09:50:01.000Z",
  data: "{}",
  iteration_id: "it-1",
};

describe("ThinkingRow", () => {
  it("shows the token count and toggles the row key", async () => {
    const onToggle = vi.fn();
    render(
      <ThinkingRow
        row={{ kind: "thinking", key: 7, events: [event], tokens: 2100 }}
        open={false}
        onToggle={onToggle}
      />,
    );
    expect(screen.getByRole("button").textContent).toContain("thinking… (2.1k tokens)");
    await userEvent.click(screen.getByRole("button"));
    expect(onToggle).toHaveBeenCalledWith(7);
  });

  it("omits a zero token count and lists the raw events when open", () => {
    render(
      <ThinkingRow
        row={{ kind: "thinking", key: 7, events: [event], tokens: 0 }}
        open
        onToggle={() => {}}
      />,
    );
    expect(screen.getByRole("button").textContent).toBe("🧠thinking…");
    expect(screen.getByText(/"seq": 7/)).toBeInTheDocument();
  });
});
