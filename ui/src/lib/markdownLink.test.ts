import { describe, expect, it } from "vitest";
import { parseLinkURL } from "./markdownLink";

describe("parseLinkURL", () => {
  it("accepts complete http and https URLs", () => {
    expect(parseLinkURL("https://example.com/a?b=1")?.href).toBe("https://example.com/a?b=1");
    expect(parseLinkURL("http://example.com")?.protocol).toBe("http:");
  });

  it("rejects other protocols and incomplete text", () => {
    expect(parseLinkURL("javascript:alert(1)")).toBeNull();
    expect(parseLinkURL("mailto:a@example.com")).toBeNull();
    expect(parseLinkURL("example.com")).toBeNull();
    expect(parseLinkURL("")).toBeNull();
  });
});
