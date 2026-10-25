import { describe, expect, it } from "vitest";
import type { ImageManifest, ImageSummary } from "@/lib/api";
import { newAgentDraft } from "../agentCreateDraft";
import { draftFromManifest, manifestState, type ManifestInputs } from "./manifestState";

const manifest = (patch: Partial<ImageManifest>): ImageManifest =>
  ({ schema_version: 1, name: "worker", tag: "v1", plugins: null, ...patch }) as ImageManifest;

const inputs = (patch: Partial<ManifestInputs> = {}): ManifestInputs => ({
  image: "worker:v1",
  images: [{ name: "worker", tag: "v1" } as ImageSummary],
  manifest: manifest({
    harness: { type: "codex", model: "gpt", effort: "high", interactive: true },
  }),
  manifestKey: "rev\u0000worker:v1",
  expectedManifestKey: "rev\u0000worker:v1",
  targetIsCurrent: true,
  targetIsUsable: true,
  draftPlugins: ["context"],
  ...patch,
});

describe("manifestState", () => {
  it("reads defaults from the current schema 1 manifest", () => {
    expect(manifestState(inputs())).toEqual({
      targetImageAvailable: true,
      manifestIsCurrent: true,
      loadingManifest: false,
      bare: false,
      schemaV2: false,
      defaultModel: "gpt",
      defaultEffort: "high",
      displayedPlugins: ["context"],
    });
  });

  it("loads while the manifest key is behind the selected image", () => {
    const state = manifestState(inputs({ manifestKey: "" }));
    expect(state.loadingManifest).toBe(true);
    expect(state.manifestIsCurrent).toBe(false);
    expect(state.defaultModel).toBe("");
  });

  it("does not load for an image the target does not list or an unusable target", () => {
    expect(manifestState(inputs({ manifestKey: "", images: [] })).loadingManifest).toBe(false);
    expect(manifestState(inputs({ manifestKey: "", targetIsUsable: false })).loadingManifest).toBe(
      false,
    );
    expect(manifestState(inputs({ manifestKey: "", targetIsCurrent: false })).loadingManifest).toBe(
      false,
    );
    expect(manifestState(inputs({ image: "" })).manifestIsCurrent).toBe(false);
  });

  it("shows a schema 2 manifest's own plugins and a bare image", () => {
    const state = manifestState(
      inputs({ manifest: manifest({ schema_version: 2, bare: true, plugins: [{ name: "p" }] }) }),
    );
    expect(state).toMatchObject({ schemaV2: true, bare: true, displayedPlugins: ["p"] });
    expect(state.defaultModel).toBe("");
  });
});

describe("draftFromManifest", () => {
  it("takes harness, model and effort from a schema 1 manifest", () => {
    const draft = draftFromManifest(
      newAgentDraft("worker:v1"),
      manifest({ harness: { type: "codex", interactive: true } }),
    );
    expect([draft.harness, draft.model, draft.effort]).toEqual(["codex", "", ""]);
  });

  it("keeps the draft's runtime for other schemas", () => {
    const current = { ...newAgentDraft("worker:v1"), harness: "claude", model: "m", effort: "e" };
    expect(draftFromManifest(current, manifest({ schema_version: 2 }))).toEqual(current);
  });
});
