import { describe, expect, it } from "vitest";
import { budgetReport } from "./bundle-budget.mjs";

const LIMIT = 800_000;

function sizes(map) {
  return (file) => map[file];
}

describe("initial JavaScript budget", () => {
  it("accepts an entry of exactly the limit", () => {
    const manifest = { "index.html": { file: "assets/index.js", isEntry: true } };
    const report = budgetReport(manifest, sizes({ "assets/index.js": LIMIT }), LIMIT);
    expect(report.ok).toBe(true);
    expect(report.entry).toEqual({ file: "assets/index.js", size: LIMIT });
    expect(report.total).toBe(LIMIT);
  });

  it("rejects an entry one byte over the limit and names its size", () => {
    const manifest = { "index.html": { file: "assets/index.js", isEntry: true } };
    const report = budgetReport(manifest, sizes({ "assets/index.js": LIMIT + 1 }), LIMIT);
    expect(report.ok).toBe(false);
    expect(report.message).toContain("800001 bytes");
    expect(report.message).toContain("limit 800000 bytes");
    expect(report.message).toContain("assets/index.js");
  });

  it("counts static dependencies of the entry, a shared one once", () => {
    const manifest = {
      "index.html": { file: "assets/index.js", isEntry: true, imports: ["_react.js", "_ui.js"] },
      "_ui.js": { file: "assets/ui.js", imports: ["_react.js"] },
      "_react.js": { file: "assets/react.js" },
    };
    const report = budgetReport(
      manifest,
      sizes({ "assets/index.js": 300_000, "assets/ui.js": 200_000, "assets/react.js": 300_001 }),
      LIMIT,
    );
    expect(report.total).toBe(800_001);
    expect(report.files.map((f) => f.file)).toEqual(["assets/index.js", "assets/react.js", "assets/ui.js"]);
    expect(report.ok).toBe(false);
    expect(report.message).toContain("initial JavaScript");
    expect(report.message).toContain("assets/ui.js");
  });

  it("leaves dynamically imported chunks out of the initial total", () => {
    const manifest = {
      "index.html": { file: "assets/index.js", isEntry: true, dynamicImports: ["src/pages/Big.tsx"] },
      "src/pages/Big.tsx": { file: "assets/Big.js", isDynamicEntry: true },
    };
    const report = budgetReport(
      manifest,
      sizes({ "assets/index.js": 100, "assets/Big.js": 5_000_000 }),
      LIMIT,
    );
    expect(report.ok).toBe(true);
    expect(report.total).toBe(100);
  });

  it("fails when the manifest has no entry", () => {
    expect(() => budgetReport({}, sizes({}), LIMIT)).toThrow(/no entry/);
  });

  it("fails when a listed file is missing", () => {
    const manifest = { "index.html": { file: "assets/index.js", isEntry: true, imports: ["_gone.js"] }, "_gone.js": { file: "assets/gone.js" } };
    expect(() => budgetReport(manifest, sizes({ "assets/index.js": 1 }), LIMIT)).toThrow(/assets\/gone\.js/);
  });

  it("fails when an import is missing from the manifest", () => {
    const manifest = { "index.html": { file: "assets/index.js", isEntry: true, imports: ["_unknown.js"] } };
    expect(() => budgetReport(manifest, sizes({ "assets/index.js": 1 }), LIMIT)).toThrow(/_unknown\.js/);
  });
});
