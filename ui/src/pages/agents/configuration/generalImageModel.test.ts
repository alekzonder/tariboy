import { describe, expect, it } from "vitest";
import type { AgentImageStatus } from "@/lib/api";
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
  it("links a ref to its Library page, naming the host only with several", () => {
    expect(imageHref("host-1", "team/base:v 1", false)).toBe("/library/images/team%2Fbase/v%201");
    expect(imageHref("host-1", "team/base:v 1", true)).toBe(
      "/library/images/team%2Fbase/v%201?host=host-1",
    );
    expect(imageHref("", "base:latest", true)).toBe("/library/images/base/latest?host=local");
  });

  it("links a ref without a tag to the image list", () => {
    expect(imageHref("", "base", false)).toBe("/library/images");
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
  current: { ref: "base:1", digest: "sha256:a", error: "" },
  pending: { ref: "", digest: "", error: "" },
  next: next
    ? { ref: "base:2", digest: "sha256:b", error: "", reason: "pending", ...next }
    : { ref: "", digest: "", error: "" },
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
