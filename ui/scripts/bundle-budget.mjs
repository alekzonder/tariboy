#!/usr/bin/env node
// JavaScript budget of the Desktop/Android bundle. Builds the production UI into
// a temporary directory, reads Vite's manifest, and checks three sizes: the
// entry chunk; the initial JavaScript, which is the entry chunk plus every chunk
// it imports statically, what the WebView must download and parse before the
// first screen; and all JavaScript, lazily loaded chunks included. Sizes are
// minified bytes before compression.
import { mkdtempSync, readFileSync, rmSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

// The sizes measured when the limits were set, plus 5% and rounded up to 1,000
// bytes. Lower a limit when the code shrinks; raise one only with a reason in
// the same change.
export const LIMITS = { entry: 351_000, initial: 723_000, total: 3_215_000 };

export function budgetReport(manifest, sizeOf, limits) {
  const entryKey = Object.keys(manifest).find((key) => manifest[key].isEntry);
  if (!entryKey) throw new Error("bundle budget: the Vite manifest has no entry chunk");
  const seen = new Set();
  const files = [];
  const visit = (key) => {
    if (seen.has(key)) return;
    seen.add(key);
    const chunk = manifest[key];
    if (!chunk) throw new Error(`bundle budget: import ${key} is missing from the Vite manifest`);
    const size = sizeOf(chunk.file);
    if (size === undefined) throw new Error(`bundle budget: ${chunk.file} is missing from the build output`);
    if (chunk.file.endsWith(".js")) files.push({ file: chunk.file, size });
    for (const dep of chunk.imports ?? []) visit(dep);
  };
  visit(entryKey);
  const [entry, ...deps] = files;
  const sorted = [entry, ...deps.sort((a, b) => a.file.localeCompare(b.file))];
  const total = sorted.reduce((sum, f) => sum + f.size, 0);
  const allJs = [...new Set(Object.values(manifest).map((chunk) => chunk.file))]
    .filter((file) => file.endsWith(".js"))
    .reduce((sum, file) => {
      const size = sizeOf(file);
      if (size === undefined) throw new Error(`bundle budget: ${file} is missing from the build output`);
      return sum + size;
    }, 0);
  const problems = [];
  if (entry.size > limits.entry) problems.push(`entry chunk ${entry.file} is ${entry.size} bytes, limit ${limits.entry} bytes`);
  if (total > limits.initial) problems.push(`initial JavaScript (entry and static imports) is ${total} bytes, limit ${limits.initial} bytes`);
  if (allJs > limits.total) problems.push(`all JavaScript (every chunk, lazy ones included) is ${allJs} bytes, limit ${limits.total} bytes`);
  const listing = sorted.map((f) => `  ${String(f.size).padStart(9)}  ${f.file}`).join("\n");
  const message = problems.length
    ? `UI bundle budget exceeded: ${problems.join("; ")}.\n` +
      `Load the new code with React.lazy or move a library into a lazily loaded chunk. ` +
      `If only all JavaScript is over, make the lazy code smaller, or raise LIMITS.total with a reason.\n${listing}`
    : `UI bundle budget ok: entry ${entry.size} bytes, limit ${limits.entry} bytes; ` +
      `initial JavaScript ${total} bytes, limit ${limits.initial} bytes; ` +
      `all JavaScript ${allJs} bytes, limit ${limits.total} bytes.\n${listing}`;
  return { ok: problems.length === 0, entry, total, allJs, files: sorted, message };
}

async function main() {
  const { build } = await import("vite");
  const uiDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
  const outDir = mkdtempSync(path.join(tmpdir(), "tariboy-ui-budget-"));
  try {
    await build({
      root: uiDir,
      configFile: path.join(uiDir, "vite.desktop.config.ts"),
      logLevel: "warn",
      build: { outDir, emptyOutDir: true, manifest: true },
    });
    const manifest = JSON.parse(readFileSync(path.join(outDir, ".vite", "manifest.json"), "utf8"));
    const sizeOf = (file) => {
      try {
        return statSync(path.join(outDir, file)).size;
      } catch {
        return undefined;
      }
    };
    const report = budgetReport(manifest, sizeOf, LIMITS);
    (report.ok ? console.log : console.error)(report.message);
    if (!report.ok) process.exitCode = 1;
  } finally {
    rmSync(outDir, { recursive: true, force: true });
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch((error) => {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  });
}
