import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Input } from "./input";
import { Textarea } from "./textarea";
import { Command, CommandInput } from "./command";

describe("shared text controls", () => {
  it.each([
    ["the browser", () => {}],
    ["Desktop", () => vi.stubGlobal("__TAURI_INTERNALS__", {})],
    ["the Android shell", () => vi.stubEnv("VITE_TARIBOY_SHELL", "android")],
  ])("disable text assistance by default in %s", (_shell, stubShell) => {
    stubShell();
    render(
      <>
        <Input aria-label="input" />
        <Textarea aria-label="textarea" />
        <Command>
          <CommandInput aria-label="command" />
        </Command>
      </>,
    );

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

  it("allows a documented field to override the defaults", () => {
    render(<Input aria-label="override" spellCheck autoCorrect="on" autoCapitalize="sentences" />);

    const control = screen.getByLabelText("override");
    expect(control).toHaveAttribute("spellcheck", "true");
    expect(control).toHaveAttribute("autocorrect", "on");
    expect(control).toHaveAttribute("autocapitalize", "sentences");
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
});
