import { defineConfig } from "playwright/test";

export default defineConfig({
  testDir: "./tests",
  testMatch: "tasks-e2e.pw.ts",
  fullyParallel: false,
  workers: 1,
  reporter: "line",
  use: {
    baseURL: "http://127.0.0.1:4175",
    headless: true,
    viewport: { width: 1440, height: 900 },
  },
  webServer: [
    {
      command: "node tests/tasks-e2e-daemon.mjs",
      url: "http://127.0.0.1:4176/api/daemon/status",
      reuseExistingServer: false,
      timeout: 30_000,
      gracefulShutdown: { signal: "SIGTERM", timeout: 10_000 },
    },
    {
      command: "npx vite --config vite.tasks-test.config.ts",
      url: "http://127.0.0.1:4175/tests/tasks-fixture.html",
      reuseExistingServer: false,
      timeout: 30_000,
    },
  ],
});
