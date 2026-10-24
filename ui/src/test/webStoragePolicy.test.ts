import { readFileSync, readdirSync } from "node:fs";
import { join, resolve } from "node:path";
import { expect, it } from "vitest";

// Production code reaches Web Storage only through src/lib/storage.ts and never
// signals other pages with window events. Tests and fixtures may seed raw
// storage: they pin the stored formats. A source scan rather than an oxlint
// rule, because the lint policy allows no per-file overrides.
const ui = resolve(__dirname, "../..");
const owner = join("src", "lib", "storage.ts");

function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return entry.name === "test" ? [] : sources(path);
    return /\.(ts|tsx)$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name) ? [path] : [];
  });
}

function violations(files: string[]): string[] {
  const found: string[] = [];
  for (const file of files) {
    const where = file.slice(ui.length + 1);
    if (where === owner) continue;
    readFileSync(file, "utf8")
      .split("\n")
      .forEach((line, index) => {
        const code = line.trim();
        if (code.startsWith("//") || code.startsWith("*") || code.startsWith("/*")) return;
        if (/\b(localStorage|sessionStorage)\b|window\.dispatchEvent\b/.test(code))
          found.push(`${where}:${index + 1}`);
      });
  }
  return found;
}

it("keeps Web Storage and window events out of production code", () => {
  expect(violations(["src", "store"].flatMap((dir) => sources(join(ui, dir))))).toEqual([]);
});
