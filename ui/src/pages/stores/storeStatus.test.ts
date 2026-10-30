import { describe, expect, it } from "vitest";
import { type StoreImageView, type StoreWorkflowView, type WorkflowImageListing } from "@/lib/api";
import { catalogRow, hasTariboyStore, latestWorkflowVersion, workflowStatus } from "./storeStatus";

const image = (fields: Partial<StoreImageView>): StoreImageView => ({
  name: "agent",
  version: "1.0.0",
  built_version: "1.0.0",
  update_needed: false,
  latest_status: "built",
  ...fields,
});

describe("catalogRow", () => {
  it.each<[string, Partial<StoreImageView>, ReturnType<typeof catalogRow>]>([
    ["built and up to date", {}, { status: "Built", tone: "muted", action: "build" }],
    [
      "built with an update",
      { built_version: "1.0.0", version: "1.2.0", update_needed: true },
      { status: "Update available (1.0.0 → 1.2.0)", tone: "update", action: "update" },
    ],
    [
      "built without a comparable version",
      { latest_status: "unversioned", built_version: "" },
      { status: "Built", tone: "muted", action: "build" },
    ],
    [
      "not built",
      { latest_status: "missing", built_version: "" },
      { status: "Not built", tone: "muted", action: "build" },
    ],
    [
      "an unreadable built image",
      { latest_status: "error", latest_error: "lstat /x: no such file" },
      {
        status: "Could not read the built image",
        detail: "lstat /x: no such file",
        tone: "error",
        action: "build",
      },
    ],
    [
      "an invalid Store entry",
      { error: "invalid Tariboyfile", latest_status: "missing" },
      {
        status: "The Store entry is invalid",
        detail: "invalid Tariboyfile",
        tone: "error",
        action: null,
      },
    ],
  ])("reports %s", (_, fields, expected) => {
    expect(catalogRow(image(fields))).toEqual(expected);
  });

  it("reports a failed build of this session first", () => {
    expect(catalogRow(image({}), { id: 7, reason: "exit status 1" })).toEqual({
      status: "Build failed: exit status 1",
      tone: "error",
      action: "build",
      operation: 7,
    });
  });
});

describe("workflowStatus", () => {
  const workflow: StoreWorkflowView = { name: "flow", version: "3" };

  it.each<[StoreWorkflowView, string | null | undefined, string]>([
    [{ ...workflow, error: "broken" }, "3", "Source error: broken"],
    [workflow, null, "Comparison unavailable"],
    [workflow, undefined, "Latest not built"],
    [workflow, "3", "Up to date"],
    [workflow, "2", "Update needed"],
  ])("reports %j with latest %s as %s", (subject, latest, expected) => {
    expect(workflowStatus(subject, latest)).toBe(expected);
  });
});

describe("latestWorkflowVersion", () => {
  const images = [
    { name: "flow", tag: "1", version: "1" },
    { name: "flow", tag: "latest", version: "2" },
    { name: "other", tag: "latest", version: "9" },
  ] as WorkflowImageListing[];

  it("returns the latest tag version", () => {
    expect(latestWorkflowVersion(images, "flow")).toBe("2");
  });

  it("returns undefined when the workflow has no latest tag", () => {
    expect(latestWorkflowVersion(images, "absent")).toBeUndefined();
  });

  it("returns null when the registry could not be read", () => {
    expect(latestWorkflowVersion(null, "flow")).toBeNull();
  });
});

describe("hasTariboyStore", () => {
  const defaultStore = { name: "tariboy", source: "https://example.com/store.git" };

  it("matches the default Store by name or by source", () => {
    const store = (name: string, source: string) => [{ name, source, path: "/s" }];
    expect(hasTariboyStore(store("tariboy", "elsewhere"), defaultStore)).toBe(true);
    expect(hasTariboyStore(store("mine", defaultStore.source), defaultStore)).toBe(true);
    expect(hasTariboyStore(store("mine", "elsewhere"), defaultStore)).toBe(false);
    expect(hasTariboyStore([], defaultStore)).toBe(false);
  });
});
