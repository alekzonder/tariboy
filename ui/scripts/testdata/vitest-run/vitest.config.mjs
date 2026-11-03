import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

const here = path.dirname(fileURLToPath(import.meta.url));

// A run that vitest cannot finish: kill.fixture.mjs kills the vitest process.
// The *.fixture.mjs names keep these files out of the main ui-test suite.
export default defineConfig({
  root: here,
  cacheDir: path.join(here, ".vite"),
  test: {
    include: ["*.fixture.mjs"],
    pool: "threads",
    maxWorkers: 1,
    reporters: ["dot", path.join(here, "../../vitest-file-ledger.mjs")],
  },
});
