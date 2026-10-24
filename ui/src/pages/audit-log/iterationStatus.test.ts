import { describe, expect, it } from "vitest";
import { statusVariant } from "./iterationStatus";

describe("statusVariant", () => {
  it.each([
    ["done", "default"],
    ["harness_error", "destructive"],
    ["timeout", "destructive"],
    ["killed", "destructive"],
    ["running", "secondary"],
    ["no_i_am_done", "secondary"],
    ["something_new", "secondary"],
  ])("maps %s to %s", (status, variant) => {
    expect(statusVariant(status)).toBe(variant);
  });
});
