/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

// Store UI: a SECOND Vite entry sharing ui/'s toolchain + shadcn kit, built to a
// SEPARATE committed dist (internal/storeui/dist) embedded into tariboy-store.
// `make store-ui` runs this build; `make build` stays Node-free.
export default defineConfig({
  root: path.resolve(__dirname, "store"),
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(__dirname, "src") } },
  build: {
    outDir: path.resolve(__dirname, "../internal/storeui/dist"),
    emptyOutDir: true,
  },
  server: {
    // Dev proxy to a local store run with --allow-insecure (plain HTTP, dev only).
    proxy: {
      "/v1": { target: "http://127.0.0.1:8443", changeOrigin: true },
    },
  },
});
