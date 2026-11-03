/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";
import v8 from "node:v8";
import { fsAllow } from "./vite.fsAllow";

// Shared dev/test config: `npm run dev` in a browser and the vitest suite. It no
// longer emits the shipped bundle — the desktop target (vite.desktop.config.ts)
// does, and the store UI has vite.store.config.ts. A plain `vite build` with
// this config lands in ui/dist, a scratch dir nothing consumes.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(__dirname, "src") } },
  server: {
    fs: { allow: fsAllow },
    // In dev, proxy API + SSE to the daemon's loopback web listener.
    proxy: {
      "/api": { target: "http://127.0.0.1:9990", changeOrigin: true },
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    // make check lowers this for agents that share one host.
    maxWorkers: Number(process.env.TARIBOY_VITEST_MAX_WORKERS) || 4,
    pool: "vmThreads",
    // A vmThreads worker keeps every test file it ran in its heap. Vitest
    // restarts it once the heap passes vmMemoryLimit, checked after each file.
    // The default, total memory / maxWorkers, can be above the worker's heap
    // limit, so the worker ran out of heap first and the run ended early.
    vmMemoryLimit: Math.floor(v8.getHeapStatistics().heap_size_limit * 0.6),
    // Workspace tests render whole pages and pass 5 s when agents share a host.
    testTimeout: 20_000,
    // vitest-file-ledger lets `npm test` (scripts/vitest-run.mjs) fail a run
    // that finished fewer test files than vitest found.
    reporters: ["agent", "./scripts/vitest-file-ledger.mjs"],
    setupFiles: ["./src/test/setup.ts"],
  },
});
