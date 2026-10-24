import { readFileSync, readdirSync } from "node:fs";
import { join, resolve } from "node:path";
import { expect, it } from "vitest";

// Production classes use the theme: named text sizes and radii from
// src/index.css and the 4px spacing scale (h-6.5 is 26px), never a bare
// `utility-[12px]`. Composite values such as `grid-cols-[76px_1fr]` stay.
// The shadcn primitives under src/components/ui stay as upstream ships them,
// so they can be updated. A source scan, like webStoragePolicy.test.ts.
const ui = resolve(__dirname, "../..");
const generated = join("src", "components", "ui");
const arbitraryPx = /\b[a-z][a-z0-9-]*-\[-?\d+(?:\.\d+)?px\]/g;

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
    if (where.startsWith(generated)) continue;
    readFileSync(file, "utf8")
      .split("\n")
      .forEach((line, index) => {
        for (const match of line.matchAll(arbitraryPx))
          found.push(`${where}:${index + 1} ${match[0]}`);
      });
  }
  return found;
}

it("keeps arbitrary px values out of production classes", () => {
  expect(violations(["src", "store"].flatMap((dir) => sources(join(ui, dir))))).toEqual([]);
});
