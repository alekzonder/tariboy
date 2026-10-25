import { describe, expect, it } from "vitest";
import type { ImageManifest } from "@/lib/api";
import { visibleImageSections } from "./imageOverviewSections";

const lean: ImageManifest = {
  schema_version: 2,
  name: "img",
  tag: "v1",
  built_at: "2026-01-01T00:00:00Z",
  parents: null,
  plugins: null,
  skills: null,
  requires_secrets: null,
  env: null,
  layers: null,
};

const titles = (manifest: ImageManifest) =>
  visibleImageSections(manifest).map((section) => section.title);

describe("visibleImageSections", () => {
  it("shows nothing for a lean image", () => {
    expect(titles(lean)).toEqual([]);
  });

  it("shows every section with content in a fixed order", () => {
    expect(
      titles({
        ...lean,
        schema_version: 1,
        bare: true,
        harness: { type: "claude", interactive: true },
        policy: { tools_deny: ["Bash"] },
        plugins: [{ name: "p" }],
        env: { A: "1" },
        parents: ["base:v1"],
        requires_secrets: ["TOKEN"],
        layers: [{ name: "l", sha256: "abc" } as never],
      }),
    ).toEqual([
      "Terminal-only",
      "Harness",
      "Policy",
      "Plugins",
      "Env",
      "Parents",
      "Requires secrets",
      "Layers",
    ]);
  });

  it("shows the harness only for schema version 1", () => {
    const harness = { type: "claude", interactive: false };
    expect(titles({ ...lean, harness })).toEqual([]);
    expect(titles({ ...lean, schema_version: 1 })).toEqual([]);
  });

  it("hides empty lists, maps and policies", () => {
    expect(
      titles({
        ...lean,
        policy: { tools_allow: [], tools_deny: [] },
        plugins: [],
        env: {},
        parents: [],
        requires_secrets: [],
        layers: [],
      }),
    ).toEqual([]);
    expect(titles({ ...lean, policy: { tools_allow: ["Read"] } })).toEqual(["Policy"]);
  });
});
