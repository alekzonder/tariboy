import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { expect, it } from "vitest";

// The lint policy: correctness errors everywhere, no per-file rule overrides,
// and one suppression form, an oxlint comment that names its rule and a reason.
const ui = resolve(__dirname, "../..");
const config = JSON.parse(readFileSync(join(ui, ".oxlintrc.json"), "utf8")) as {
  categories: Record<string, string>;
  rules: Record<string, unknown>;
  overrides?: Array<{ rules?: Record<string, unknown> }>;
};

function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    if (entry.name === "node_modules" || entry.name === "dist") return [];
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return sources(path);
    return /\.(ts|tsx|mjs|js)$/.test(entry.name) ? [path] : [];
  });
}

const files = ["src", "store", ".design-sync"].flatMap((dir) => sources(join(ui, dir)));

it("turns on the correctness category and overrides no rule per file", () => {
  expect(config.categories.correctness).toBe("error");
  expect((config.overrides ?? []).filter((override) => override.rules)).toEqual([]);
  expect(Object.entries(config.rules).filter(([, level]) => level === "warn")).toEqual([]);
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
