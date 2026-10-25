import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { expect, it } from "vitest";

// The lint policy: correctness errors everywhere, the complexity and size
// thresholds of development.mdx, three kinds of rule override, and one
// suppression form, an oxlint comment that names its rule and a reason.
const ui = resolve(__dirname, "../..");
const raw = readFileSync(join(ui, ".oxlintrc.json"), "utf8");
type Override = { files: string[]; rules?: Record<string, unknown> };
const config = JSON.parse(raw.replace(/^\s*\/\/.*$/gm, "")) as {
  categories: Record<string, string>;
  rules: Record<string, unknown>;
  overrides?: Override[];
};

const thresholds = {
  complexity: ["error", 15],
  "max-depth": ["error", 4],
  "max-lines": ["error", { max: 400, skipBlankLines: true, skipComments: true }],
  "max-lines-per-function": ["error", { max: 150, skipBlankLines: true, skipComments: true }],
  "max-nested-callbacks": ["error", 4],
  "max-params": ["error", 5],
};
const testOverride = {
  files: ["**/*.test.{ts,tsx}", "**/*.pw.ts", "src/test/**", "tests/**"],
  rules: { "max-lines": "off", "max-lines-per-function": "off" },
};
const designSyncOverride = {
  files: [".design-sync/**"],
  rules: Object.fromEntries(Object.keys(thresholds).map((rule) => [rule, "off"])),
};
const temporaryComment = "// Temporary: task 20, must be empty before the next release.";

function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    if (entry.name === "node_modules" || entry.name === "dist") return [];
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return sources(path);
    return /\.(ts|tsx|mjs|js)$/.test(entry.name) ? [path] : [];
  });
}

const files = ["src", "store", ".design-sync"].flatMap((dir) => sources(join(ui, dir)));

it("turns on the correctness category and no warning", () => {
  expect(config.categories.correctness).toBe("error");
  expect(Object.entries(config.rules).filter(([, level]) => level === "warn")).toEqual([]);
});

it("turns on the complexity and size thresholds as errors", () => {
  for (const [rule, setting] of Object.entries(thresholds))
    expect(config.rules[rule]).toEqual(setting);
});

it("overrides rules only for test files, .design-sync, and the temporary list", () => {
  const overrides = (config.overrides ?? []).filter((override) => override.rules);
  expect(overrides.slice(0, 2)).toEqual([testOverride, designSyncOverride]);
  const temporary = overrides.slice(2);
  if (temporary.length === 0) return;
  // The temporary list follows its comment and only raises a threshold, so a
  // listed function cannot grow and the rule stays on for the rest of the file.
  expect(raw).toContain(temporaryComment);
  expect(raw.indexOf(temporaryComment)).toBeLessThan(
    raw.indexOf(JSON.stringify(temporary[0].files[0])),
  );
  for (const { rules } of temporary)
    for (const [rule, setting] of Object.entries(rules ?? {})) {
      expect(Object.keys(thresholds)).toContain(rule);
      expect(Array.isArray(setting) && setting[0] === "error").toBe(true);
    }
});

it("suppresses only with an oxlint comment that names the rule and a reason", () => {
  const eslint: string[] = [];
  const unexplained: string[] = [];
  // Built from parts so this file does not match its own search.
  const disable = ["oxlint", "disable"].join("-");
  for (const file of files) {
    if (file === __filename) continue;
    readFileSync(file, "utf8")
      .split("\n")
      .forEach((line, index) => {
        const where = `${file.slice(ui.length + 1)}:${index + 1}`;
        if (line.includes(["eslint", "disable"].join("-"))) eslint.push(where);
        if (
          line.includes(disable) &&
          !new RegExp(`${disable}(-next-line)? [a-z-]+/[a-z-]+ -- \\S`).test(line)
        )
          unexplained.push(where);
      });
  }
  expect(eslint).toEqual([]);
  expect(unexplained).toEqual([]);
});

it("fails the lint script on a warning", () => {
  const script = (
    JSON.parse(readFileSync(join(ui, "package.json"), "utf8")) as {
      scripts: Record<string, string>;
    }
  ).scripts.lint;
  expect(script.split(" ")).toContain("--deny-warnings");
  const scratch = mkdtempSync(join(tmpdir(), "lint-policy-"));
  try {
    writeFileSync(
      join(scratch, ".oxlintrc.json"),
      JSON.stringify({ rules: { "no-debugger": "warn" } }),
    );
    writeFileSync(join(scratch, "warn.js"), "debugger\n");
    const lint = () =>
      execFileSync(join(ui, "node_modules/.bin/oxlint"), [...script.split(" ").slice(1)], {
        cwd: scratch,
        stdio: "pipe",
      });
    expect(lint).toThrow();
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
});

it("fails the lint script on a complex function, a long function, and a long file", () => {
  const scratch = mkdtempSync(join(tmpdir(), "lint-policy-"));
  try {
    writeFileSync(join(scratch, ".oxlintrc.json"), raw);
    const branches = Array.from({ length: 15 }, (_, i) => `  if (x === ${i}) return ${i};`);
    writeFileSync(
      join(scratch, "complex.ts"),
      `export function complex(x: number) {\n${branches.join("\n")}\n  return -1;\n}\n`,
    );
    const body = Array.from({ length: 147 }, (_, i) => `  v += ${i};`);
    writeFileSync(
      join(scratch, "long.ts"),
      `export function long() {\n  let v = 0;\n${body.join("\n")}\n  return v;\n}\n`,
    );
    const lines = Array.from({ length: 401 }, (_, i) => `export const c${i} = ${i};`);
    writeFileSync(join(scratch, "big.ts"), `${lines.join("\n")}\n`);
    let output = "";
    try {
      execFileSync(join(ui, "node_modules/.bin/oxlint"), ["--deny-warnings", "-f", "unix"], {
        cwd: scratch,
        stdio: "pipe",
      });
    } catch (error) {
      output = String((error as { stdout: Buffer }).stdout);
    }
    expect(output).toContain("complex.ts");
    expect(output).toContain("complexity of 16");
    expect(output).toContain("too many lines (151)");
    expect(output).toContain("long.ts");
    expect(output).toContain("big.ts");
    expect(output).toContain("eslint(max-lines)");
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
});
