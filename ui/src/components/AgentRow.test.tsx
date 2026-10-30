import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import type { AgentState } from "@/lib/agentState";
import { AgentRow } from "./AgentRow";

const now = new Date("2026-10-30T12:30:00Z");

function row(agentState: AgentState) {
  render(<AgentRow name="bob" agentState={agentState} now={now} />);
  return screen.getByRole("button");
}

describe("AgentRow", () => {
  it.each([
    [{ kind: "needs", needs: { count: 2 } }, "needs you · 2", "attention"],
    [{ kind: "working", since: "2026-10-30T12:18:00Z" }, "working · 12 min", "live"],
    [{ kind: "working" }, "working", "live"],
    [{ kind: "error", reason: "boom" }, "error", "danger"],
    [{ kind: "off" }, "off", "quiet"],
  ] as const)("shows the pill of %o", (agentState, text, tone) => {
    const button = row(agentState as AgentState);
    expect(button).toHaveTextContent(`bob${text}`);
    expect(button.querySelector('[data-slot="agent-row-pill"]')).toHaveAttribute("data-tone", tone);
  });

  it("names the reason of an error", () => {
    row({ kind: "error", reason: "Out of budget: day" });
    expect(screen.getByText("error")).toHaveAttribute("title", "Out of budget: day");
  });

  it("shows nothing on the right of an idle agent", () => {
    const button = row({ kind: "idle" });
    expect(button).toHaveTextContent(/^bob$/);
    expect(button.querySelector('[data-slot="agent-row-pill"]')).toBeNull();
  });
});
