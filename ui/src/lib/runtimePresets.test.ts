import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  RUNTIME_PRESETS_STORAGE_KEY,
  rememberRuntimePreset,
  runtimePresetGroups,
  runtimePresetOptions,
  type HarnessCatalogState,
} from "./runtimePresets";
import type { HarnessCatalog } from "./types";

beforeEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
});

describe("runtime presets", () => {
  it("merges learned values and extras without duplicates", () => {
    rememberRuntimePreset("codex", "models", " private-model ");

    expect(
      runtimePresetOptions("codex", "models", ["o3", "private-model", " "]),
    ).toEqual(["private-model", "o3"]);
  });

  it("isolates learned values by harness and field", () => {
    rememberRuntimePreset("codex", "models", "private-model");
    rememberRuntimePreset("codex", "efforts", "ultra");

    expect(runtimePresetOptions("claude", "models")).not.toContain("private-model");
    expect(runtimePresetOptions("codex", "models")).not.toContain("ultra");
    expect(runtimePresetOptions("codex", "efforts")).toEqual(["ultra"]);
  });

  it("ignores malformed storage", () => {
    localStorage.setItem(RUNTIME_PRESETS_STORAGE_KEY, "{broken");

    expect(runtimePresetOptions("codex", "models")).toEqual([]);
  });

  it("continues when storage access throws", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("storage unavailable");
    });

    expect(runtimePresetOptions("codex", "models")).toEqual([]);
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

  it("does not store empty values", () => {
    rememberRuntimePreset("codex", "models", " ");

    expect(localStorage.getItem(RUNTIME_PRESETS_STORAGE_KEY)).toBeNull();
  });

  it("stores any typed value, including ones the harness reports", () => {
    rememberRuntimePreset("codex", "models", "gpt-5");

    expect(runtimePresetOptions("codex", "models")).toEqual(["gpt-5"]);
  });
});

const codexCatalog: HarnessCatalog = {
  harness: "codex",
  models: [
    { id: "gpt-6-astra", label: "GPT-6-Astra", efforts: ["low", "ultra"] },
    { id: "gpt-5.6-luna", label: "GPT-5.6-Luna", efforts: ["low", "max"] },
  ],
  efforts: ["low", "max", "ultra"],
  error: "",
};
const ready: HarnessCatalogState = { loading: false, catalog: codexCatalog, error: "" };

describe("runtime preset groups", () => {
  it("separates harness-reported values from saved ones", () => {
    rememberRuntimePreset("codex", "models", "gpt-6-astra");
    rememberRuntimePreset("codex", "models", "private-model");

    expect(
      runtimePresetGroups({
        harness: "codex",
        field: "models",
        catalog: ready,
        extras: ["image-default"],
      }),
    ).toEqual([
      { label: "From harness", options: ["gpt-6-astra", "gpt-5.6-luna"] },
      { label: "Saved", options: ["private-model", "image-default"] },
    ]);
  });

  it("offers the selected model efforts, else the harness efforts", () => {
    const efforts = (model: string) =>
      runtimePresetGroups({ harness: "codex", field: "efforts", catalog: ready, model })[0]
        .options;

    expect(efforts("gpt-5.6-luna")).toEqual(["low", "max"]);
    expect(efforts("private-model")).toEqual(["low", "max", "ultra"]);
  });

  it("explains an empty harness group", () => {
    const group = (catalog: HarnessCatalogState, harness = "codex") =>
      runtimePresetGroups({ harness, field: "models", catalog })[0];

    expect(group({ loading: true, catalog: null, error: "" })).toEqual({
      label: "From harness",
      options: [],
      note: "Loading…",
    });
    expect(
      group({ loading: false, catalog: null, error: "codex not found on PATH" }).note,
    ).toBe("codex not found on PATH");
    expect(group({ loading: false, catalog: null, error: "" }, "").note).toBe(
      "Choose a harness to list its values",
    );
  });
});
