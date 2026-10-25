import { describe, expect, it } from "vitest";
import type { AgentImageStatus } from "@/lib/api";
import { serverPath } from "@/lib/terminalsHost";
import {
  imageHref,
  imageSides,
  nextReasonText,
  shortDigest,
  updatesOnNext,
} from "./generalImageModel";

describe("shortDigest", () => {
  it("keeps a short digest whole and elides the middle of a long one", () => {
    expect(shortDigest("sha256:abc")).toBe("sha256:abc");
    const long = `sha256:${"0123456789abcdef".repeat(4)}`;
    expect(shortDigest(long)).toBe(`${long.slice(0, 14)}…${long.slice(-8)}`);
  });
});

describe("imageHref", () => {
  it("links a ref to its image page on the target host", () => {
    const base = serverPath("host-1", "images");
    expect(imageHref("host-1", "team/base:v 1")).toBe(`${base}/team%2Fbase/v%201`);
  });

  it("links a ref without a tag to the image list", () => {
    expect(imageHref("", "base")).toBe(serverPath("", "images"));
  });
});

describe("nextReasonText", () => {
  it.each([
    ["pending", "Explicit pending assignment"],
    ["ref_moved", "Content the active ref points at now"],
    ["current", "Current pinned image"],
  ] as const)("explains %s", (reason, text) => {
    expect(nextReasonText(reason)).toBe(text);
  });
});

const status = (next?: Partial<NonNullable<AgentImageStatus["next"]>>): AgentImageStatus => ({
  name: "worker",
  current: { ref: "base:1", digest: "sha256:a" },
  pending: { ref: "", digest: "", error: "" },
  next: next && { ref: "base:2", digest: "sha256:b", reason: "pending", ...next },
});

describe("updatesOnNext", () => {
  it("announces an update only for a known, different next digest", () => {
    expect(updatesOnNext(status())).toBe(false);
    expect(updatesOnNext(status({}))).toBe(true);
    expect(updatesOnNext(status({ digest: "sha256:a" }))).toBe(false);
    expect(updatesOnNext(status({ digest: "" }))).toBe(false);
    expect(updatesOnNext(status({ error: "gone" }))).toBe(false);
  });
});

describe("imageSides", () => {
  it("labels the current side by run state and keeps the next side", () => {
    const value = status({});
    expect(imageSides(value, false)).toEqual([
      { label: "Current", value: value.current },
      { label: "Next iteration", value: value.next },
    ]);
    expect(imageSides(value, true)[0].label).toBe("Activated version");
  });
});
