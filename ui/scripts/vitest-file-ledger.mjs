// Vitest reporter that records the run in a ledger file for scripts/vitest-run.mjs:
// "found N" when the run starts, then "done <file>" as each test file ends.
// Writes are synchronous so the lines survive a killed vitest process.
// Without TARIBOY_VITEST_LEDGER it does nothing (watch mode, a plain vitest).
import fs from "node:fs";

export default class VitestFileLedger {
  constructor() {
    this.path = process.env.TARIBOY_VITEST_LEDGER;
  }

  write(line) {
    if (this.path) fs.appendFileSync(this.path, `${line}\n`);
  }

  onTestRunStart(specifications) {
    this.write(`found ${specifications.length}`);
  }

  onTestModuleEnd(testModule) {
    this.write(`done ${testModule.moduleId}`);
  }
}
