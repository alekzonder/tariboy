import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { CodexDetails } from "./CodexDetails";

describe("CodexDetails", () => {
  it("labels command details and shows status and output", () => {
    render(
      <CodexDetails
        codex={{
          type: "command",
          label: "command",
          icon: "⌨",
          text: "npm test",
          status: "completed",
          output: "ok",
        }}
      />,
    );
    const details = screen.getByLabelText("Command details");
    expect(details.textContent).toContain("command: npm test");
    expect(details.textContent).toContain("Status: completed");
    expect(details.textContent).toContain("Output:\nok");
  });

  it("labels other activity and omits a missing status and output", () => {
    render(<CodexDetails codex={{ type: "skill", label: "Skill", icon: "✦", text: "tdd" }} />);
    const details = screen.getByLabelText("Activity details");
    expect(details.textContent).toBe("Skill: tdd");
  });

  it("shows an empty output when it is present", () => {
    render(
      <CodexDetails
        codex={{ type: "tool", label: "Tool", icon: "🔧", text: "a · b", output: "" }}
      />,
    );
    expect(screen.getByLabelText("Activity details").textContent).toContain("Output:");
  });
});
