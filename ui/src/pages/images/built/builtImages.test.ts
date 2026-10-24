import { describe, expect, it } from "vitest";
import type { ImageRow } from "@/lib/api";
import { archiveFilename, groupBuiltImages, sourceLabel, splitImageRef } from "./builtImages";

const image = (overrides: Partial<ImageRow>) =>
  ({ name: "demo", tag: "1", ...overrides }) as ImageRow;

describe("sourceLabel", () => {
  it("names an imported artifact without a source directory", () => {
    expect(sourceLabel(image({}))).toBe("Source CWD unavailable — imported artifact");
  });

  it("marks a source directory that is no longer available", () => {
    expect(sourceLabel(image({ source_cwd: "/src/demo", source_available: false }))).toBe(
      "Source CWD unavailable — /src/demo",
    );
  });

  it("shows an available source directory", () => {
    expect(sourceLabel(image({ source_cwd: "/src/demo", source_available: true }))).toBe(
      "/src/demo",
    );
    expect(sourceLabel(image({ source_cwd: "/src/demo" }))).toBe("/src/demo");
  });
});

describe("splitImageRef", () => {
  it("splits at the last colon", () => {
    expect(splitImageRef("registry:5000/team/demo:v2")).toEqual({
      name: "registry:5000/team/demo",
      tag: "v2",
    });
  });

  it("keeps a ref without a colon as the tag", () => {
    expect(splitImageRef("demo")).toEqual({ name: "dem", tag: "demo" });
  });
});

describe("archiveFilename", () => {
  it("replaces slashes in the name and appends the tag", () => {
    expect(archiveFilename("team/sub/demo:v2")).toBe("team_sub_demo-v2.tariboy-image.tar.gz");
  });
});

describe("groupBuiltImages", () => {
  it("groups by name, sorts names and tags, and picks latest", () => {
    const rows = [
      image({ name: "b", tag: "old", built_at: "2026-01-01T00:00:00Z" }),
      image({ name: "a", tag: "latest", built_at: "2026-01-01T00:00:00Z" }),
      image({ name: "b", tag: "new", built_at: "2026-02-01T00:00:00Z" }),
    ];
    const groups = groupBuiltImages(rows);
    expect(groups.map((group) => group.name)).toEqual(["a", "b"]);
    expect(groups[0].latest?.tag).toBe("latest");
    expect(groups[1].tags.map((tag) => tag.tag)).toEqual(["new", "old"]);
    expect(groups[1].latest).toBeUndefined();
  });
});
