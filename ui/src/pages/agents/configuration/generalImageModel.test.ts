import { describe, expect, it } from "vitest";
import { imageHref, shortDigest } from "./generalImageModel";

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
