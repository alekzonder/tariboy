/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

// Shared dev/test config: `npm run dev` in a browser and the vitest suite. It no
// longer emits the shipped bundle — the desktop target (vite.desktop.config.ts)
// does, and the store UI has vite.store.config.ts. A plain `vite build` with
// this config lands in ui/dist, a scratch dir nothing consumes.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(__dirname, "src") } },
  server: {
    // In dev, proxy API + SSE to the daemon's loopback web listener.
    proxy: {
      "/api": { target: "http://127.0.0.1:9990", changeOrigin: true },
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    pool: "vmThreads",
    setupFiles: ["./src/test/setup.ts"],
  },
});
