import { describe, expect, it } from "vitest";
import { formatReport, reportConfig } from "./complexity-report.mjs";

function diagnostic(rule, message, filename, line) {
  return { code: `eslint(${rule})`, message, filename, labels: [{ span: { line } }] };
}

describe("UI complexity report", () => {
  it("lists violations by rule, largest value first, with a total per rule", () => {
    const report = formatReport([
      diagnostic(
        "max-lines-per-function",
        "The function `A` has too many lines (155). Maximum allowed is 150.",
        "src/A.tsx",
        14,
      ),
      diagnostic(
        "complexity",
        "function `B` has a complexity of 16. Maximum allowed is 15.",
        "src/B.ts",
        3,
      ),
      diagnostic(
        "max-lines-per-function",
        "The function `C` has too many lines (239). Maximum allowed is 150.",
        "src/C.tsx",
        27,
      ),
    ]);
    const lines = report.split("\n");
    expect(lines.findIndex((l) => l.includes("src/B.ts:3"))).toBeLessThan(
      lines.findIndex((l) => l.includes("src/C.tsx:27")),
    );
    expect(lines.findIndex((l) => l.includes("src/C.tsx:27"))).toBeLessThan(
      lines.findIndex((l) => l.includes("src/A.tsx:14")),
    );
    expect(report).toMatch(/complexity\s+16\/15\s+src\/B\.ts:3\s+function `B`/);
    expect(report).toMatch(/max-lines-per-function\s+239\/150\s+src\/C\.tsx:27\s+The function `C`/);
    expect(report).toContain("complexity: 1");
    expect(report).toContain("max-lines-per-function: 2");
  });

  it("says so when nothing is over a threshold", () => {
    expect(formatReport([])).toContain(
      "No UI function or file is over a complexity or size threshold.",
    );
  });

  it("keeps the thresholds and the off overrides, and drops the temporary list", () => {
    const config = reportConfig({
      plugins: ["typescript"],
      categories: { correctness: "error" },
      ignorePatterns: ["dist"],
      rules: { "no-empty": "error", complexity: ["error", 15] },
      overrides: [
        { files: ["**/*.{ts,tsx}"], env: { browser: true } },
        { files: ["tests/**"], rules: { "max-lines": "off" } },
        { files: ["src/A.tsx"], rules: { complexity: ["error", 18] } },
      ],
    });
    expect(config).toEqual({
      plugins: ["typescript"],
      categories: {},
      ignorePatterns: ["dist"],
      rules: { complexity: ["error", 15] },
      overrides: [{ files: ["tests/**"], rules: { "max-lines": "off" } }],
    });
  });
});
