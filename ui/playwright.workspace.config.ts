import { defineConfig } from "playwright/test";

export default defineConfig({
  testDir: "./tests",
  testMatch: ["workspace-layout.pw.ts", "images-layout.pw.ts"],
  fullyParallel: false,
  workers: 1,
  reporter: "line",
  use: {
    baseURL: "http://127.0.0.1:4174",
    headless: true,
    viewport: { width: 1200, height: 800 },
  },
  webServer: {
    command: "npx vite --config vite.workspace-test.config.ts",
    url: "http://127.0.0.1:4174/tests/workspace-fixture.html",
    reuseExistingServer: false,
    timeout: 30_000,
  },
});
