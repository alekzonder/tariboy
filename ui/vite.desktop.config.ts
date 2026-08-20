/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

// Desktop bundle: the SAME src/ as the browser build, emitted into desktop/dist
// where tauri.conf.json's frontendDist picks it up. Unlike the old
// internal/webui/dist this output is NOT committed — `make desktop` rebuilds it
// every time, so ui/src and the shipped app can never drift.
//
// base: "./" keeps asset URLs relative, so index.html works no matter what root
// the Tauri custom protocol serves it from.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(__dirname, "src") } },
  base: "./",
  build: {
    outDir: path.resolve(__dirname, "../desktop/dist"),
    emptyOutDir: true,
  },
  server: {
    // `cargo tauri dev` points devUrl at this address; strictPort makes a busy
    // port a loud failure instead of a silent shift the Rust side won't follow.
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": { target: "http://127.0.0.1:9990", changeOrigin: true },
    },
  },
});
