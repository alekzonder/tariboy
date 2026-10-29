/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";
import { fsAllow } from "./vite.fsAllow";

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
    license: { fileName: "THIRD-PARTY-LICENSES.md" },
    outDir: path.resolve(__dirname, "../desktop/dist"),
    emptyOutDir: true,
    // Vendor libraries in named chunks (Vite 8's replacement for
    // manualChunks). Each editor group is reached only through a lazily
    // loaded screen, so it stays out of the initial download; keep a group
    // narrow, because one module the shell needs pulls its whole group in.
    // `npm run bundle:budget` checks the initial size.
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            { name: "react", test: /node_modules[\\/](react|react-dom|scheduler|react-router|react-router-dom)[\\/]/ },
            { name: "codemirror", test: /node_modules[\\/](@codemirror|@lezer|@uiw|@marijn|crelt|style-mod|w3c-keyname)[\\/]/ },
            { name: "tiptap", test: /node_modules[\\/](@tiptap|prosemirror-[^\\/]+|orderedmap|rope-sequence|linkifyjs|marked)[\\/]/ },
            { name: "xterm", test: /node_modules[\\/]@xterm[\\/]/ },
            // The HTTP client and host registry: shared by the shell and most
            // lazy screens, so without a group the bundler may fold it into the
            // entry chunk once enough screens share it.
            { name: "api", test: /src[\\/]lib[\\/](api[\\/]|daemons\.ts|storage\.ts)/ },
            { name: "markdown", test: /node_modules[\\/](react-markdown|remark-[^\\/]+|rehype-[^\\/]+|unified|micromark[^\\/]*|mdast-[^\\/]+|hast-[^\\/]+|unist-[^\\/]+|highlight\.js|lowlight)[\\/]/ },
          ],
        },
      },
    },
  },
  server: {
    // `cargo tauri dev` points devUrl at this address; strictPort makes a busy
    // port a loud failure instead of a silent shift the Rust side won't follow.
    port: 5173,
    strictPort: true,
    fs: { allow: fsAllow },
    proxy: {
      "/api": { target: "http://127.0.0.1:9990", changeOrigin: true },
    },
  },
});
