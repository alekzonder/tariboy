import { defineConfig } from "playwright/test";

export default defineConfig({
  testDir: "./tests/desktop",
  testMatch: "*.pw.ts",
  // Every spec receives an independent Desktop fixture with its own temporary
  // daemon, runtime, app-data, display, and WebDriver ports. Two workers keep
  // the host load bounded while overlapping that isolated setup and teardown.
  fullyParallel: true,
  workers: 2,
  reporter: "line",
  outputDir: "test-results-desktop",
  timeout: 90_000,
  expect: { timeout: 30_000 },
  use: {
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
});
