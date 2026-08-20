import { describe, it, expect, afterEach, vi } from "vitest";
import { configureDesktopInputAssistance, desktopStartHash } from "./main";

afterEach(() => vi.unstubAllGlobals());

describe("desktopStartHash", () => {
  it("returns null outside the desktop app", () => {
    expect(desktopStartHash("")).toBeNull();
  });

  it("sends a fresh desktop window to Agents", () => {
    vi.stubGlobal("__TAURI_INTERNALS__", {});
    expect(desktopStartHash("")).toBe("#/");
  });

  it("leaves an existing route alone (a reload must not bounce home)", () => {
    vi.stubGlobal("__TAURI_INTERNALS__", {});
    expect(desktopStartHash("#/usage")).toBeNull();
  });
});

describe("configureDesktopInputAssistance", () => {
  it("disables all input assistance on the Desktop root", () => {
    vi.stubGlobal("__TAURI_INTERNALS__", {});
    const root = document.createElement("div");

    configureDesktopInputAssistance(root);

    expect(root).toHaveAttribute("spellcheck", "false");
    expect(root).toHaveAttribute("autocorrect", "off");
    expect(root).toHaveAttribute("autocapitalize", "off");
  });

  it("leaves the browser root unchanged", () => {
    const root = document.createElement("div");

    configureDesktopInputAssistance(root);

    expect(root).not.toHaveAttribute("spellcheck");
    expect(root).not.toHaveAttribute("autocorrect");
    expect(root).not.toHaveAttribute("autocapitalize");
  });
});
