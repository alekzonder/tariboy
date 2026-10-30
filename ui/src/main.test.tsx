import { describe, it, expect, afterEach, vi } from "vitest";
import { desktopStartHash, usesHashRouter } from "./main";

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

describe("usesHashRouter", () => {
  afterEach(() => vi.unstubAllEnvs());

  it("is on in both Tauri shells, whose asset protocol has no SPA fallback", () => {
    expect(usesHashRouter()).toBe(false);
    vi.stubEnv("VITE_TARIBOY_SHELL", "android");
    expect(usesHashRouter()).toBe(true);
    vi.unstubAllEnvs();
    vi.stubGlobal("__TAURI_INTERNALS__", {});
    expect(usesHashRouter()).toBe(true);
  });
});
