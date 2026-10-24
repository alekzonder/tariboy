import { describe, expect, it } from "vitest";
import type { ApiClient, TeamImportPreview } from "@/lib/api";
import { changedImageRefs, previewFromImport } from "./teamImportModel";

const api = {} as ApiClient;

const image = (ref: string) => ({ ref, action: "pull", conflict: false });

describe("previewFromImport", () => {
  it("normalizes a YAML preview", () => {
    const preview: TeamImportPreview = {
      import_id: "imp-1",
      team: "team",
      yaml: "version: 1\n",
      images: [{ ...image("img:1"), message: "note" }],
      agents: [{ name: "alice", action: "create", conflict: false }],
      groups: undefined as unknown as TeamImportPreview["groups"],
    };
    expect(previewFromImport(preview, api)).toEqual({
      import_id: "imp-1",
      team: "team",
      yaml: "version: 1\n",
      images: [
        {
          ref: "img:1",
          action: "pull",
          conflict: false,
          message: "note",
          original: "img:1",
          resolved: "img:1",
        },
      ],
      agents: [{ name: "alice", action: "create", conflict: false }],
      groups: [],
      updateExisting: false,
      api,
    });
  });

  it("normalizes an archive preview without agents or groups", () => {
    const preview = {
      import_id: "imp-2",
      team: "t",
      yaml: "",
      images: [image("a:1"), image("b:2")],
    } as unknown as TeamImportPreview;
    const got = previewFromImport(preview, api);
    expect(got.agents).toEqual([]);
    expect(got.groups).toEqual([]);
    expect(got.images.map((i) => [i.original, i.resolved])).toEqual([
      ["a:1", "a:1"],
      ["b:2", "b:2"],
    ]);
    expect(got.updateExisting).toBe(false);
    expect(got.api).toBe(api);
  });
});

describe("changedImageRefs", () => {
  it("maps only edited refs from original to resolved", () => {
    expect(
      changedImageRefs([
        { original: "a:1", resolved: "a:1" },
        { original: "b:2", resolved: "mirror/b:2" },
      ]),
    ).toEqual({ "b:2": "mirror/b:2" });
  });

  it("is undefined when nothing changed", () => {
    expect(changedImageRefs([{ original: "a:1", resolved: "a:1" }])).toBeUndefined();
    expect(changedImageRefs([])).toBeUndefined();
  });
});
