import { describe, expect, it } from "vitest";
import { autopilotMode, rememberedMinutes, toMinutes, toSeconds } from "./autopilotModel";

describe("autopilotMode", () => {
  it.each([
    ["0", "on-demand"],
    ["", "every"],
    ["abc", "every"],
    ["30", "every"],
    ["1800", "every"],
  ] as const)("reads %j seconds as %s", (seconds, mode) => {
    expect(autopilotMode(seconds)).toBe(mode);
  });
});

describe("toMinutes", () => {
  it.each([
    ["0", "0"],
    ["90", "1.5"],
    ["1800", "30"],
    ["7200", "120"],
    ["", ""],
    ["abc", "abc"],
  ])("shows %j seconds as %j minutes", (seconds, minutes) => {
    expect(toMinutes(seconds)).toBe(minutes);
  });
});

describe("toSeconds", () => {
  it.each([
    ["0", "0"],
    ["30", "1800"],
    ["2.5", "150"],
    [" 1.5 ", "90"],
    ["0.01", "1"],
  ])("sends %j minutes as %j seconds", (minutes, seconds) => {
    expect(toSeconds(minutes)).toBe(seconds);
  });

  it.each(["", "abc", "-1"])("passes %j through so validation rejects it", (text) => {
    expect(toSeconds(text)).toBe(text);
  });

  it("round-trips the minutes it shows", () => {
    for (const seconds of ["90", "1800", "7200"])
      expect(toSeconds(toMinutes(seconds))).toBe(seconds);
  });
});

describe("rememberedMinutes", () => {
  it("keeps a saved positive interval and defaults to 30 minutes", () => {
    expect(rememberedMinutes("600")).toBe("10");
    expect(rememberedMinutes("0")).toBe("30");
    expect(rememberedMinutes("")).toBe("30");
  });
});
