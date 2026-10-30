import { describe, expect, it } from "vitest";
import type { ImageSummary } from "@/lib/api";
import { libraryImageRows, unusedCount, visibleImageRows } from "./libraryImages";

function tag(name: string, tagName: string, extra: Partial<ImageSummary> = {}): ImageSummary {
  return {
    name,
    tag: tagName,
    bare: false,
    built_at: "2026-10-01T00:00:00Z",
    current_agents: null,
    pending_agents: null,
    digest: `${name}-${tagName}`,
    exportable: true,
    schema_version: 1,
    ...extra,
  };
}

describe("libraryImageRows", () => {
  it("gives one row per name and hides bare", () => {
    const images: ImageSummary[] = [];
    for (let n = 0; n < 40; n++) {
      for (let t = 0; t < 7; t++) images.push(tag(`image-${n}`, `v${t}`));
    }
    for (let t = 0; t < 20; t++) images.push(tag("bare", `v${t}`, { bare: true }));
    expect(images).toHaveLength(300);
    const rows = libraryImageRows(images);
    expect(rows).toHaveLength(40);
    expect(rows.find((row) => row.name === "bare")).toBeUndefined();
  });

  it("reads version, source and built time from the newest tag", () => {
    const [row] = libraryImageRows([
      tag("dev", "v1", { built_at: "2026-10-01T00:00:00Z", image_version: "1", source: "old" }),
      tag("dev", "v2", { built_at: "2026-10-03T00:00:00Z", image_version: "2" }),
      tag("dev", "latest", { built_at: "2026-10-02T00:00:00Z", image_version: "1.5" }),
    ]);
    expect(row).toMatchObject({
      name: "dev",
      tag: "v2",
      version: "2",
      source: "old",
      builtAt: "2026-10-03T00:00:00Z",
      runRef: "dev:latest",
    });
  });

  it("runs the newest tag when there is no latest", () => {
    const [row] = libraryImageRows([
      tag("dev", "v1", { built_at: "2026-10-01T00:00:00Z" }),
      tag("dev", "v2", { built_at: "2026-10-03T00:00:00Z" }),
    ]);
    expect(row.runRef).toBe("dev:v2");
  });

  it("counts each agent once across current and pending of every tag", () => {
    const [row] = libraryImageRows([
      tag("dev", "v1", { current_agents: ["a", "b"] }),
      tag("dev", "v2", { pending_agents: ["a", "c"] }),
    ]);
    expect(row.usedBy).toBe(3);
  });
});

describe("visibleImageRows", () => {
  const rows = libraryImageRows([
    tag("zeta", "v1", { current_agents: ["a"] }),
    tag("alpha", "v1", { current_agents: ["a"] }),
    tag("Beta", "v1", { current_agents: ["a", "b"] }),
    tag("unused", "v1"),
  ]);

  it("sorts by used-by, then name, and hides unused images by default", () => {
    expect(visibleImageRows(rows, { query: "", showUnused: false }).map((row) => row.name)).toEqual(
      ["Beta", "alpha", "zeta"],
    );
    expect(unusedCount(rows)).toBe(1);
  });

  it("shows unused images on request", () => {
    expect(visibleImageRows(rows, { query: "", showUnused: true }).map((row) => row.name)).toEqual([
      "Beta",
      "alpha",
      "zeta",
      "unused",
    ]);
  });

  it("filters by name, ignoring case", () => {
    expect(
      visibleImageRows(rows, { query: "BET", showUnused: false }).map((row) => row.name),
    ).toEqual(["Beta"]);
  });
});
