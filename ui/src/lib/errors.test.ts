import { describe, expect, it } from "vitest";
import { ApiError } from "./api";
import { errorText } from "./errors";

describe("errorText", () => {
  it("reads the message of an API error", () => {
    expect(errorText(new ApiError(409, "conflict", "revision changed"))).toBe("revision changed");
  });

  it("reads the message of any Error without its class name", () => {
    expect(errorText(new TypeError("Failed to fetch"))).toBe("Failed to fetch");
  });

  it("stringifies a value that is not an Error", () => {
    expect(errorText("boom")).toBe("boom");
    expect(errorText(42)).toBe("42");
  });
});
