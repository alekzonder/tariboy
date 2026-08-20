import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Input } from "./input";
import { Textarea } from "./textarea";
import { Command, CommandInput } from "./command";

describe("shared text controls", () => {
  it("disable macOS text assistance by default", () => {
    vi.stubGlobal("__TAURI_INTERNALS__", {});
    render(<><Input aria-label="input" /><Textarea aria-label="textarea" /><Command><CommandInput aria-label="command" /></Command></>);

    for (const control of [
      screen.getByLabelText("input"),
      screen.getByLabelText("textarea"),
      screen.getByLabelText("command"),
    ]) {
      expect(control).toHaveAttribute("spellcheck", "false");
      expect(control).toHaveAttribute("autocorrect", "off");
      expect(control).toHaveAttribute("autocapitalize", "off");
    }
  });

  it("leaves browser text assistance unchanged", () => {
    render(<Input aria-label="input" />);

    expect(screen.getByLabelText("input")).not.toHaveAttribute("spellcheck");
    expect(screen.getByLabelText("input")).not.toHaveAttribute("autocorrect");
    expect(screen.getByLabelText("input")).not.toHaveAttribute("autocapitalize");
  });

  it("allows a documented field to override the defaults", () => {
    render(
      <Input
        aria-label="override"
        spellCheck
        autoCorrect="on"
        autoCapitalize="sentences"
      />,
    );

    const control = screen.getByLabelText("override");
    expect(control).toHaveAttribute("spellcheck", "true");
    expect(control).toHaveAttribute("autocorrect", "on");
    expect(control).toHaveAttribute("autocapitalize", "sentences");
  });
});

afterEach(() => vi.unstubAllGlobals());
