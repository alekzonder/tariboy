import { describe, expect, it } from "vitest";
import {
  type StoreImageView,
  type StoreWorkflowView,
  TARIBOY_STORE,
  type WorkflowImage,
} from "@/lib/api";
import {
  hasTariboyStore,
  imageStatuses,
  latestImageVersionLabel,
  latestWorkflowVersion,
  workflowStatus,
} from "./storeStatus";

const image = (fields: Partial<StoreImageView>): StoreImageView => ({
  name: "agent",
  version: "1.0.0",
  built_version: "1.0.0",
  update_needed: false,
  latest_status: "built",
  ...fields,
});

describe("imageStatuses", () => {
  it.each<[string, Partial<StoreImageView>, string[]]>([
    ["up to date", {}, ["Up to date"]],
    ["update needed", { update_needed: true }, ["Update needed"]],
    [
      "source version missing",
      { version: "" },
      ["Source image_version missing; comparison unavailable"],
    ],
    [
      "latest unversioned",
      { latest_status: "unversioned" },
      ["Latest image_version missing; comparison unavailable"],
    ],
    [
      "latest built without version",
      { built_version: "" },
      ["Latest image_version missing; comparison unavailable"],
    ],
    ["latest missing", { latest_status: "missing" }, ["Latest not built"]],
    ["latest inspection error", { latest_status: "error" }, ["Latest error: inspection failed"]],
    [
      "latest error with detail",
      { latest_status: "error", latest_error: "denied" },
      ["Latest error: denied"],
    ],
    ["source error on built", { error: "bad manifest" }, ["Source error: bad manifest"]],
    [
      "source error and missing latest",
      { error: "bad", latest_status: "missing" },
      ["Source error: bad", "Latest not built"],
    ],
    [
      "no latest status",
      { latest_status: undefined as unknown as StoreImageView["latest_status"] },
      ["Comparison unavailable"],
    ],
  ])("reports %s", (_, fields, expected) => {
    expect(imageStatuses(image(fields))).toEqual(expected);
  });
});

describe("latestImageVersionLabel", () => {
  it.each<[Partial<StoreImageView>, string]>([
    [{ latest_status: "missing" }, "Not built"],
    [{ latest_status: "error" }, "Inspection failed"],
    [{ built_version: "2.0.0" }, "2.0.0"],
    [{ built_version: "" }, "Not specified"],
  ])("labels %j as %s", (fields, expected) => {
    expect(latestImageVersionLabel(image(fields))).toBe(expected);
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
  ] as WorkflowImage[];

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
  it("matches the Tariboy Store by name or by source", () => {
    expect(hasTariboyStore([{ name: TARIBOY_STORE.name, source: "elsewhere", path: "/s" }])).toBe(
      true,
    );
    expect(hasTariboyStore([{ name: "mine", source: TARIBOY_STORE.source, path: "/s" }])).toBe(
      true,
    );
    expect(hasTariboyStore([{ name: "mine", source: "elsewhere", path: "/s" }])).toBe(false);
    expect(hasTariboyStore([])).toBe(false);
  });
});
