import { describe, expect, it } from "vitest";
import { imageDetailBodyState } from "./imageDetailState";

describe("imageDetailBodyState", () => {
  it.each([
    ["boom", true, "error"],
    ["boom", false, "error"],
    ["", true, "loaded"],
    ["", false, "loading"],
  ] as const)("error %j with detail %s is %s", (error, hasDetail, state) => {
    expect(imageDetailBodyState(error, hasDetail)).toBe(state);
  });
});
