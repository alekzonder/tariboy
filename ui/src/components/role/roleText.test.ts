import { describe, expect, it } from "vitest";
import { roleFragment, roleText } from "./roleText";

describe("roleText", () => {
  it("returns the ROLE.md text", () => {
    expect(roleText({ role: { text: "You review code." } })).toBe("You review code.");
  });

  it("treats a missing file, a missing manifest and whitespace as no ROLE.md", () => {
    expect(roleText({})).toBeNull();
    expect(roleText(null)).toBeNull();
    expect(roleText({ role: { text: " \n\t \n" } })).toBeNull();
  });
});

describe("roleFragment", () => {
  it("takes the first paragraph that is not a heading", () => {
    expect(roleFragment("# Reviewer\n\nYou review\ncode.\n\nSecond paragraph.")).toEqual({
      text: "You review code.",
      more: true,
    });
  });

  it("reports no other blocks for a single paragraph", () => {
    expect(roleFragment("You review code.")).toEqual({ text: "You review code.", more: false });
  });

  it("skips setext headings, code fences, tables and rules", () => {
    const text = [
      "Reviewer",
      "========",
      "",
      "```",
      "not this",
      "",
      "nor this",
      "```",
      "",
      "| a | b |",
      "| - | - |",
      "",
      "---",
      "",
      "The role.",
    ].join("\n");
    expect(roleFragment(text).text).toBe("The role.");
  });

  it("falls back to the first list item", () => {
    expect(roleFragment("# Duties\n\n- Review *every* PR\n- Merge nothing")).toEqual({
      text: "Review every PR",
      more: true,
    });
    expect(roleFragment("1. First duty\n2. Second duty").text).toBe("First duty");
  });

  it("falls back to the heading text when nothing else is there", () => {
    expect(roleFragment("## Reviewer")).toEqual({ text: "Reviewer", more: false });
  });

  it("removes inline Markdown", () => {
    expect(
      roleFragment(
        "> **Review** [pull requests](https://x.test) with `gh`, ![logo](a.png) " +
          "_carefully_ and ~~fast~~ <b>now</b> <https://y.test> snake_case_name.",
      ).text,
    ).toBe(
      "Review pull requests with gh, logo carefully and fast now https://y.test snake_case_name.",
    );
  });
});
