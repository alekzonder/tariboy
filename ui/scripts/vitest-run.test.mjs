import { spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { ledgerReport } from "./vitest-run.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));

describe("ledgerReport", () => {
  it("fails a clean exit that finished fewer files than vitest found", () => {
    const report = ledgerReport("found 3\ndone a\ndone b\n", { code: 0, signal: null });
    expect(report.ok).toBe(false);
    expect(report.exitCode).toBe(1);
    expect(report.message).toContain("finished 2 of 3 test files");
    expect(report.message).toContain("1 did not run");
  });

  it("fails when vitest never reported the files it found", () => {
    const report = ledgerReport("", { code: 0, signal: null });
    expect(report.ok).toBe(false);
    expect(report.exitCode).toBe(1);
    expect(report.message).toContain("before it reported the test files");
  });

  it("fails a run killed by a signal even with a complete ledger", () => {
    const report = ledgerReport("found 2\ndone a\ndone b\n", { code: null, signal: "SIGKILL" });
    expect(report.ok).toBe(false);
    expect(report.exitCode).toBe(1);
    expect(report.message).toContain("SIGKILL");
    expect(report.message).toContain("2 of 2");
  });

  it("counts a file reported twice once", () => {
    const report = ledgerReport("found 2\ndone a\ndone a\n", { code: 0, signal: null });
    expect(report.ok).toBe(false);
    expect(report.message).toContain("finished 1 of 2 test files");
  });

  it("accepts a complete run and names both numbers", () => {
    const report = ledgerReport("found 3\ndone a\ndone b\ndone c\n", { code: 0, signal: null });
    expect(report.ok).toBe(true);
    expect(report.exitCode).toBe(0);
    expect(report.message).toContain("finished 3 of 3 test files");
  });

  it("keeps a non-zero vitest exit code for a complete run", () => {
    const report = ledgerReport("found 1\ndone a\n", { code: 1, signal: null });
    expect(report.ok).toBe(false);
    expect(report.exitCode).toBe(1);
    expect(report.message).toContain("finished 1 of 1 test files");
  });
});

describe("vitest-run wrapper", () => {
  it("fails and names both numbers when vitest dies in the middle of a run", () => {
    // A nested vitest must not inherit this worker's VITEST_* state.
    const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith("VITEST")));
    delete env.TARIBOY_VITEST_LEDGER;
    const result = spawnSync(
      process.execPath,
      [path.join(here, "vitest-run.mjs"), "--config", path.join(here, "testdata/vitest-run/vitest.config.mjs")],
      { cwd: path.join(here, ".."), env, encoding: "utf8", timeout: 60_000 },
    );
    const output = `${result.stdout}\n${result.stderr}`;
    expect(result.status, output).not.toBe(0);
    expect(output).toMatch(/finished [01] of 2 test files/);
  });
});
