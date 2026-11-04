/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";
import { fsAllow } from "./vite.fsAllow";
import { vitestMaxWorkers } from "./vite.workers";

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
    // make check lowers this for agents that share one host. The minimum is 2:
    // one vmThreads worker runs every file in one VM context, and the run fails.
    maxWorkers: vitestMaxWorkers(process.env.TARIBOY_VITEST_MAX_WORKERS, console.warn),
    pool: "vmThreads",
    // A vmThreads worker keeps every test file it ran in its heap. Vitest
    // restarts it once the heap passes vmMemoryLimit, checked after each file.
    // The default, total memory / maxWorkers, can be above the worker's heap
    // limit, so the worker ran out of heap first and the run ended early.
    // The limit is absolute, not a share of the heap limit: a share grows with
    // the heap limit of the host (1344 MiB per worker on a 2240 MiB heap), and
    // each worker held that much, so the run peaked at 3.4 to 3.7 GB with two
    // workers. 336 MiB keeps every file running and bounds the peak at about
    // workers × 336 MiB + 0.6 to 0.9 GB on every host and Node.js version.
    // Each restart costs about 0.8 s. 512 MiB halves the restarts, but `make
    // check` on the host then peaks at 1.66 GB of anon memory, not 1.29 GB:
    // docs/superpowers/specs/2026-11-04-ui-test-vm-memory-limit-tuning.md.
    vmMemoryLimit: 336 * 1024 * 1024,
    // Workspace tests render whole pages and pass 5 s when agents share a host.
    testTimeout: 20_000,
    // vitest-file-ledger lets `npm test` (scripts/vitest-run.mjs) fail a run
    // that finished fewer test files than vitest found.
    reporters: ["agent", "./scripts/vitest-file-ledger.mjs"],
    setupFiles: ["./src/test/setup.ts"],
  },
});
