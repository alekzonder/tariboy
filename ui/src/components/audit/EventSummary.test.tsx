import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { EventSummary } from "./EventSummary";
import type { Descriptor } from "@/lib/audit";

const d: Descriptor = { icon: "📍", label: "status", preview: "checking", tone: "neutral" };

describe("EventSummary", () => {
  it("renders a Codex command as code with its status", () => {
    const { container } = render(
      <EventSummary
        d={d}
        codex={{
          type: "command",
          label: "command",
          icon: "⌨",
          text: "npm test",
          status: "running",
        }}
      />,
    );
    const code = container.querySelector("code");
    expect(code?.textContent).toBe("npm test");
    expect(code).toHaveAttribute("title", "npm test");
    expect(container.textContent).toBe("npm testrunning");
  });

  it("renders Codex text with an optional status", () => {
    const { container, rerender } = render(
      <EventSummary
        d={d}
        codex={{ type: "message", label: "message", icon: "💬", text: "hi", status: "done" }}
      />,
    );
    expect(container.textContent).toBe("hidone");
    rerender(
      <EventSummary d={d} codex={{ type: "thinking", label: "Thinking", icon: "🧠", text: "x" }} />,
    );
    expect(container.textContent).toBe("x");
    expect(container.querySelector("code")).toBeNull();
  });

  it("falls back to the descriptor preview without Codex", () => {
    const { container } = render(<EventSummary d={d} codex={undefined} />);
    expect(container.textContent).toBe("checking");
  });
});
