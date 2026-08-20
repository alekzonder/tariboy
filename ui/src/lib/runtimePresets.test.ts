import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  EFFORT_PRESETS,
  RUNTIME_PRESETS_STORAGE_KEY,
  rememberRuntimePreset,
  runtimePresetOptions,
} from "./runtimePresets";

beforeEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
});

describe("runtime presets", () => {
  it("merges built-ins, learned values, and extras without duplicates", () => {
    rememberRuntimePreset("codex", "models", " private-model ");

    expect(
      runtimePresetOptions("codex", "models", ["o3", "private-model", " "]),
    ).toEqual(["gpt-5", "private-model", "o3"]);
  });

  it("isolates learned values by harness and field", () => {
    rememberRuntimePreset("codex", "models", "private-model");
    rememberRuntimePreset("codex", "efforts", "ultra");

    expect(runtimePresetOptions("claude", "models")).not.toContain("private-model");
    expect(runtimePresetOptions("codex", "models")).not.toContain("ultra");
    expect(runtimePresetOptions("codex", "efforts")).toEqual([
      ...EFFORT_PRESETS,
      "ultra",
    ]);
  });

  it("ignores malformed storage", () => {
    localStorage.setItem(RUNTIME_PRESETS_STORAGE_KEY, "{broken");

    expect(runtimePresetOptions("codex", "models")).toEqual(["gpt-5"]);
  });

  it("continues when storage access throws", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("storage unavailable");
    });

    expect(runtimePresetOptions("codex", "models")).toEqual(["gpt-5"]);
    expect(() =>
      rememberRuntimePreset("codex", "models", "private-model"),
    ).not.toThrow();
  });

  it("keeps only the twenty most recently learned exact values", () => {
    for (let index = 0; index < 21; index += 1) {
      rememberRuntimePreset("codex", "models", `custom-${index}`);
    }
    rememberRuntimePreset("codex", "models", "custom-1");

    const options = runtimePresetOptions("codex", "models");
    expect(options).not.toContain("custom-0");
    expect(options).toContain("custom-20");
    expect(options.filter((value) => value === "custom-1")).toHaveLength(1);

    const learned = JSON.parse(
      localStorage.getItem(RUNTIME_PRESETS_STORAGE_KEY) ?? "{}",
    ) as { codex: { models: string[] } };
    expect(learned.codex.models).toHaveLength(20);
    expect(learned.codex.models.at(-1)).toBe("custom-1");
  });

  it("does not store empty or built-in values", () => {
    rememberRuntimePreset("codex", "models", " ");
    rememberRuntimePreset("codex", "models", "gpt-5");

    expect(localStorage.getItem(RUNTIME_PRESETS_STORAGE_KEY)).toBeNull();
  });
});
