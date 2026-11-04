// vitestMaxWorkers turns TARIBOY_VITEST_MAX_WORKERS into the vitest worker
// count. An empty or non-numeric value keeps the default of 4. A number below 2
// becomes 2, with a warning: one vmThreads worker runs every test file in one
// VM context, so the run fails and takes about 30 minutes.
export function vitestMaxWorkers(raw: string | undefined, warn: (message: string) => void): number {
  const workers = Number(raw);
  if ((raw ?? "").trim() === "" || Number.isNaN(workers)) return 4;
  if (workers >= 2) return workers;
  warn(
    `ui-test: TARIBOY_VITEST_MAX_WORKERS=${raw} is below 2; using 2 workers, because one vmThreads worker runs all test files in one VM context`,
  );
  return 2;
}
