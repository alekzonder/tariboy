import { describe, expect, it } from "vitest";
import { MARKS, applyMark } from "./chatComposerModel";

const mark = (title: string) => MARKS.find((item) => item.title === title)!;

describe("applyMark", () => {
  it("wraps the selection and puts the caret after the closing mark", () => {
    expect(applyMark({ value: "say hi now", start: 4, end: 6, mark: mark("Bold") })).toEqual({
      next: "say **hi** now",
      caret: 10,
    });
  });

  it("prefixes the line the selection starts on", () => {
    expect(applyMark({ value: "one\ntwo", start: 5, end: 6, mark: mark("Quote") })).toEqual({
      next: "one\n> two",
      caret: 8,
    });
  });

  it("writes a link around the selection or a placeholder text", () => {
    expect(applyMark({ value: "go here", start: 3, end: 7, mark: mark("Link") })).toEqual({
      next: "go [here](url)",
      caret: 10,
    });
    expect(applyMark({ value: "", start: 0, end: 0, mark: mark("Link") })).toEqual({
      next: "[text](url)",
      caret: 7,
    });
  });
});
