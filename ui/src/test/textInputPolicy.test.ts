import { readFileSync, readdirSync } from "node:fs";
import { join, resolve } from "node:path";
import { expect, it } from "vitest";

// Production text fields render only through the shared controls in
// src/components/ui (Input, Textarea, CommandInput, SearchInput) or the task
// Markdown editor, which all turn off spell checking, autocorrection and
// autocapitalization. A raw <input> is allowed only for types the user cannot
// type into. A source scan rather than an oxlint rule, because the lint policy
// allows no per-file overrides.
const ui = resolve(__dirname, "../..");
const shared = join("src", "components", "ui") + "/";
const editor = join("src", "pages", "tasks", "TaskMarkdownEditor.tsx");
const untypedInputs = new Set(["checkbox", "radio", "file"]);

function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return entry.name === "test" ? [] : sources(path);
    return /\.(ts|tsx)$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name) ? [path] : [];
  });
}

function lineOf(text: string, offset: number): number {
  return text.slice(0, offset).split("\n").length;
}

function fileViolations(where: string, text: string): string[] {
  const found: string[] = [];
  for (const match of text.matchAll(/<textarea\b/g)) {
    found.push(`${where}:${lineOf(text, match.index)} <textarea>`);
  }
  for (const match of text.matchAll(/<input\b/g)) {
    const end = text.indexOf("/>", match.index);
    const tag = text.slice(match.index, end < 0 ? undefined : end);
    const type = /\btype="([^"]+)"/.exec(tag)?.[1];
    if (!type || !untypedInputs.has(type))
      found.push(`${where}:${lineOf(text, match.index)} <input type=${type ?? "text"}>`);
  }
  if (where !== editor) {
    for (const match of text.matchAll(/\bcontentEditable\b|\buseEditor\(/g)) {
      found.push(`${where}:${lineOf(text, match.index)} ${match[0]}`);
    }
  }
  return found;
}

function violations(files: string[]): string[] {
  return files.flatMap((file) => {
    const where = file.slice(ui.length + 1);
    if (where.startsWith(shared)) return [];
    return fileViolations(where, readFileSync(file, "utf8"));
  });
}

it("renders text fields only through the shared controls", () => {
  expect(violations(["src", "store"].flatMap((dir) => sources(join(ui, dir))))).toEqual([]);
});
