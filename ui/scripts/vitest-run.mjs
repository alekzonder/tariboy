// `npm test`: runs `vitest run` with the given arguments and fails when vitest
// finished fewer test files than it found. A vitest worker thread that runs out
// of memory can end the run early with exit code 0 and no summary, so the exit
// code alone is not proof that every file ran. The ledger comes from the
// reporter in scripts/vitest-file-ledger.mjs.
import { spawn } from "node:child_process";
import fs from "node:fs";
import { createRequire } from "node:module";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

/**
 * Compares the files vitest found with the files it finished.
 * @param {string} ledger the ledger text: "found N" and "done <file>" lines
 * @param {{ code: number | null, signal: string | null }} exit how vitest exited
 * @returns {{ ok: boolean, exitCode: number, message: string }}
 */
export function ledgerReport(ledger, { code, signal }) {
  let found = null;
  const done = new Set();
  for (const line of ledger.split("\n")) {
    if (line.startsWith("found ")) found = Number(line.slice("found ".length));
    else if (line.startsWith("done ")) done.add(line.slice("done ".length));
  }
  const failCode = code || 1;
  const how = signal ? `vitest was killed by ${signal}` : `vitest exited with code ${code}`;
  if (found === null) {
    return { ok: false, exitCode: failCode, message: `ui-test: ${how} before it reported the test files it found` };
  }
  const counts = `vitest finished ${done.size} of ${found} test files`;
  if (signal) return { ok: false, exitCode: 1, message: `ui-test: ${counts}; ${how}` };
  if (done.size < found) {
    return { ok: false, exitCode: failCode, message: `ui-test: ${counts}; ${found - done.size} did not run` };
  }
  return { ok: code === 0, exitCode: code, message: `ui-test: ${counts}` };
}

function main(args) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "vitest-ledger-"));
  const ledger = path.join(dir, "ledger");
  fs.writeFileSync(ledger, "");
  const vitest = path.join(path.dirname(createRequire(import.meta.url).resolve("vitest/package.json")), "vitest.mjs");
  const child = spawn(process.execPath, [vitest, "run", ...args], {
    stdio: "inherit",
    env: { ...process.env, TARIBOY_VITEST_LEDGER: ledger },
  });
  for (const sig of ["SIGINT", "SIGTERM"]) process.on(sig, () => child.kill(sig));
  child.on("exit", (code, signal) => {
    const report = ledgerReport(fs.readFileSync(ledger, "utf8"), { code, signal });
    fs.rmSync(dir, { recursive: true, force: true });
    (report.ok ? console.log : console.error)(report.message);
    process.exit(report.exitCode);
  });
}

if (process.argv[1] === fileURLToPath(import.meta.url)) main(process.argv.slice(2));
