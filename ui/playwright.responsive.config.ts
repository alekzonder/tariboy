import { defineConfig } from "playwright/test";

// The production SPA (through the Tasks fixture's Tauri stub) against a
// daemon with isolated state, at phone and tablet sizes.
export default defineConfig({
  testDir: "./tests",
  testMatch: "responsive.pw.ts",
  fullyParallel: false,
  workers: 1,
  reporter: "line",
  use: {
    baseURL: "http://127.0.0.1:4177",
    headless: true,
    hasTouch: true,
  },
  projects: [
    { name: "phone-360", use: { viewport: { width: 360, height: 800 } } },
    { name: "phone-390", use: { viewport: { width: 390, height: 844 } } },
    { name: "tablet-portrait", use: { viewport: { width: 820, height: 1180 } } },
    { name: "tablet-landscape", use: { viewport: { width: 1180, height: 820 } } },
  ],
  webServer: [
    {
      command: "node tests/tasks-e2e-daemon.mjs",
      url: "http://127.0.0.1:4176/api/daemon/status",
      reuseExistingServer: false,
      timeout: 60_000,
      gracefulShutdown: { signal: "SIGTERM", timeout: 10_000 },
    },
    {
      command: "npx vite --config vite.tasks-test.config.ts --port 4177",
      url: "http://127.0.0.1:4177/tests/tasks-fixture.html",
      reuseExistingServer: false,
      timeout: 30_000,
    },
  ],
});
