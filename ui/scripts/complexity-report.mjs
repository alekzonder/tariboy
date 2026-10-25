#!/usr/bin/env node
// Prints every UI function and file over a complexity or size threshold of
// ui/.oxlintrc.json, for planning refactoring. The temporary overrides that
// raise a threshold for listed files are left out, so their debt shows. Never
// fails on a violation, like `make gocyclo-report`; fails only when oxlint
// does not run.
import { spawnSync } from "node:child_process";
import { readFileSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const RULES = [
  "complexity",
  "max-depth",
  "max-lines",
  "max-lines-per-function",
  "max-nested-callbacks",
  "max-params",
];

// The quality rules only, with the overrides that turn them off (test files,
// .design-sync) and without the overrides that raise a threshold.
export function reportConfig(config) {
  const rules = Object.fromEntries(
    Object.entries(config.rules).filter(([rule]) => RULES.includes(rule)),
  );
  const overrides = (config.overrides ?? []).filter(
    (override) =>
      override.rules && Object.values(override.rules).every((setting) => setting === "off"),
  );
  return {
    plugins: config.plugins,
    categories: {},
    ignorePatterns: config.ignorePatterns,
    rules,
    overrides,
  };
}

export function formatReport(diagnostics) {
  if (diagnostics.length === 0)
    return "No UI function or file is over a complexity or size threshold.";
  const rows = diagnostics.map((d) => {
    const rule = d.code.replace(/^eslint\((.*)\)$/, "$1");
    const value = Number(d.message.match(/\d+/)?.[0] ?? 0);
    const max = d.message.match(/Maximum allowed is (\d+)/)?.[1];
    return {
      rule,
      value,
      metric: max ? `${value}/${max}` : String(value),
      where: `${d.filename}:${d.labels?.[0]?.span.line ?? 0}`,
      text: d.message.replace(/\.? Maximum allowed is \d+\.$/, ""),
    };
  });
  rows.sort(
    (a, b) => a.rule.localeCompare(b.rule) || b.value - a.value || a.where.localeCompare(b.where),
  );
  const width = (key) => Math.max(...rows.map((row) => row[key].length));
  const [ruleWidth, metricWidth, whereWidth] = [width("rule"), width("metric"), width("where")];
  const lines = rows.map(
    (row) =>
      `${row.rule.padEnd(ruleWidth)}  ${row.metric.padStart(metricWidth)}  ${row.where.padEnd(whereWidth)}  ${row.text}`,
  );
  const totals = RULES.map((rule) => [rule, rows.filter((row) => row.rule === rule).length]).filter(
    ([, n]) => n > 0,
  );
  return `${lines.join("\n")}\n\n${totals.map(([rule, n]) => `${rule}: ${n}`).join("\n")}\ntotal: ${rows.length}`;
}

function main() {
  const uiDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
  const source = readFileSync(path.join(uiDir, ".oxlintrc.json"), "utf8");
  const config = reportConfig(JSON.parse(source.replace(/^\s*\/\/.*$/gm, "")));
  // Next to the original: oxlint resolves override globs from the config directory.
  const configPath = path.join(uiDir, ".oxlintrc.report.json");
  writeFileSync(configPath, JSON.stringify(config));
  try {
    const oxlint = path.join(uiDir, "node_modules", ".bin", "oxlint");
    const run = spawnSync(oxlint, ["-c", configPath, "-f", "json", "."], {
      cwd: uiDir,
      encoding: "utf8",
    });
    let diagnostics;
    try {
      diagnostics = JSON.parse(run.stdout).diagnostics;
    } catch {
      throw new Error(
        `complexity report: oxlint did not run\n${run.error?.message ?? ""}${run.stderr}${run.stdout}`,
      );
    }
    console.log(formatReport(diagnostics));
  } finally {
    rmSync(configPath, { force: true });
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  }
}
