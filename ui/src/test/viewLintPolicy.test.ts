import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { expect, it } from "vitest";

// The view-layer policy (spec 3): oxlint forbids data access in view files,
// stories, mockups and fixtures; this test forbids raw colors, which no oxlint
// rule can see. Both use the globs of the one override in .oxlintrc.json.
const ui = resolve(__dirname, "../..");
const raw = readFileSync(join(ui, ".oxlintrc.json"), "utf8");
type Override = { files: string[]; rules?: Record<string, unknown> };
const overrides = (JSON.parse(raw) as { overrides: Override[] }).overrides;
const globs = overrides.find((o) => o.rules?.["no-restricted-imports"])?.files ?? [];

function paletteNames(): string[] {
  const css = readFileSync(join(ui, "node_modules/tailwindcss/theme.css"), "utf8");
  return [...new Set([...css.matchAll(/--color-([a-z]+)-\d{2,3}:/g)].map((m) => m[1]))].sort();
}

// One alternation, so a color that two forms describe (`-[#ff0000]`) is one finding.
function findColorViolations(files: string[], palettes: string[]): string[] {
  const names = palettes.join("|");
  const forms = [
    "-\\[#",
    "#[0-9a-fA-F]{3,8}\\b",
    `--color-(?:${names})-\\d{2,3}`,
    `\\b[a-z-]*-(?:${names})-\\d{2,3}\\b`,
  ];
  const found: string[] = [];
  for (const file of files) {
    readFileSync(file, "utf8")
      .split("\n")
      .forEach((line, index) => {
        for (const match of line.matchAll(new RegExp(forms.join("|"), "g")))
          found.push(`${file}:${index + 1}: ${match[0]}`);
      });
  }
  return found;
}

// A glob of the override as a matcher over a path under ui/.
function matches(glob: string, path: string): boolean {
  const source = glob
    .replace(/[.+^${}()|[\]\\]/g, "\\$&")
    .replace(/\*\*/g, "@@GLOBSTAR@@")
    .replace(/\*/g, "[^/]*")
    .replace(/@@GLOBSTAR@@/g, ".*");
  return new RegExp(`^${source}$`).test(path);
}

function walk(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    if (entry.name === "node_modules") return [];
    const path = join(dir, entry.name);
    return entry.isDirectory() ? walk(path) : [path];
  });
}

const viewFiles = () =>
  walk(join(ui, "src")).filter((file) => {
    const rel = file.slice(ui.length + 1);
    return /\.tsx?$/.test(rel) && globs.some((glob) => matches(glob, rel));
  });

function scratch<T>(run: (dir: string) => T): T {
  const dir = mkdtempSync(join(tmpdir(), "view-lint-"));
  try {
    return run(dir);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

function write(dir: string, rel: string, text: string): string {
  const path = join(dir, rel);
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, text);
  return path;
}

function oxlint(dir: string): string {
  try {
    return String(
      execFileSync(join(ui, "node_modules/.bin/oxlint"), ["-f", "unix", "."], {
        cwd: dir,
        stdio: "pipe",
      }),
    );
  } catch (error) {
    return String((error as { stdout: Buffer }).stdout);
  }
}

const lineOf = (output: string, file: string, line: number) =>
  output.split("\n").filter((l) => l.startsWith(`${file}:${line}:`));

it("rejects data access in each view-layer glob", () => {
  scratch((dir) => {
    writeFileSync(join(dir, ".oxlintrc.json"), raw);
    const bad = 'import type { AgentRow } from "@/lib/api";\n';
    const view = [
      'import type { AgentRow } from "@/lib/api";', // 1: import type
      'import { tasksApi } from "@/lib/api/tasks";', // 2: domain module
      'import { useQuery } from "@tanstack/react-query";', // 3
      'import type { QueryClient } from "@tanstack/react-query";', // 4
      'import { x } from "../../lib/api";', // 5: relative path
      'export type { Task } from "@/lib/api";', // 6: export type from
      'export { tasksApi as again } from "@/lib/api";', // 7: export from
      'export * from "@tanstack/react-query";', // 8: export star
      'import type { AgentRow as T } from "@/types";', // 9: allowed
      'void fetch("/a");', // 10
      'void window.fetch("/b");', // 11
      'void globalThis.fetch("/c");', // 12
      'void self.fetch("/d");', // 13
      "",
    ].join("\n");
    write(dir, "src/components/x/A.view.tsx", view);
    write(dir, "src/components/x/A.stories.tsx", bad);
    write(dir, "src/mockups/p/P.stories.tsx", bad);
    write(dir, "src/fixtures/f.ts", bad);
    write(dir, "src/components/x/B.tsx", view);
    const out = oxlint(dir);
    const v = "src/components/x/A.view.tsx";
    for (const line of [1, 2, 3, 4, 5, 6, 7, 8])
      expect(lineOf(out, v, line).join("\n"), `line ${line}`).toContain("no-restricted-imports");
    expect(lineOf(out, v, 9).join("\n")).not.toContain("no-restricted");
    expect(lineOf(out, v, 10).join("\n")).toContain("no-restricted-globals");
    for (const line of [11, 12, 13])
      expect(lineOf(out, v, line).join("\n"), `line ${line}`).toContain("no-restricted-properties");
    for (const file of [
      "src/components/x/A.stories.tsx",
      "src/mockups/p/P.stories.tsx",
      "src/fixtures/f.ts",
    ])
      expect(lineOf(out, file, 1).join("\n"), file).toContain("no-restricted-imports");
    // Not a view file: the same content gets no view-layer rule.
    const other = out.split("\n").filter((l) => l.startsWith("src/components/x/B.tsx"));
    expect(other.filter((l) => /no-restricted/.test(l))).toEqual([]);
  });
});

it("finds no hex color or Tailwind palette in the view layer", () => {
  const files = viewFiles();
  expect(files.length).toBeGreaterThan(40);
  expect(findColorViolations(files, paletteNames())).toEqual([]);
});

it("detects each color form", () => {
  scratch((dir) => {
    const palettes = paletteNames();
    const bad = write(
      dir,
      "src/mockups/p/P.stories.tsx",
      [
        '<div className="bg-red-500" />',
        'cn("border-[#ff0000]")',
        'cva("", { variants: { t: { a: "hover:text-rose-600/50" } } })',
        'const s = { color: "#123456", background: "var(--color-zinc-950)" };',
        "",
      ].join("\n"),
    );
    expect(findColorViolations([bad], palettes)).toHaveLength(5);
    const clean = write(
      dir,
      "src/mockups/p/Clean.stories.tsx",
      '<div className="bg-primary text-muted-foreground text-white" />\n',
    );
    expect(findColorViolations([clean], palettes)).toEqual([]);
  });
});

it("reads 26 palette names from tailwindcss/theme.css", () => {
  expect(paletteNames()).toEqual(
    "amber blue cyan emerald fuchsia gray green indigo lime mauve mist neutral olive orange pink purple red rose sky slate stone taupe teal violet yellow zinc".split(
      " ",
    ),
  );
});
