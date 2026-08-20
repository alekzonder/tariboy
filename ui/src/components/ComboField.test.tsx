import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ComboField, EFFORT_PRESETS } from "./ComboField";

describe("ComboField", () => {
  it("commits typed value on Enter", async () => {
    const onCommit = vi.fn();
    render(<ComboField label="model" value="" presets={[]} onCommit={onCommit} />);
    await userEvent.type(screen.getByPlaceholderText("model"), "opus{Enter}");
    expect(onCommit).toHaveBeenCalledWith("opus");
  });

  it("commits on blur only when the value changed", async () => {
    const onCommit = vi.fn();
    render(<ComboField label="model" value="sonnet" presets={[]} onCommit={onCommit} />);
    const input = screen.getByPlaceholderText("model");
    await userEvent.click(input);
    await userEvent.tab(); // blur with no change
    expect(onCommit).not.toHaveBeenCalled();
  });

  it("exposes effort presets", () => {
    expect(EFFORT_PRESETS).toContain("high");
  });
});
