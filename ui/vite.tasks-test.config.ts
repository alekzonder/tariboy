import path from "node:path";
import { fsAllow } from "./vite.fsAllow";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// make ui-dev passes free ports; the Playwright suites use the defaults.
const port = Number(process.env.TARIBOY_UI_DEV_PORT) || 4175;
const apiPort = Number(process.env.TARIBOY_UI_DEV_API_PORT) || 4176;

export default defineConfig({
  plugins: [react(), tailwindcss()],
  define: {
    "import.meta.env.TARIBOY_UI_DEV_API_URL": JSON.stringify(`http://127.0.0.1:${apiPort}`),
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
    },
  },
  server: {
    host: "127.0.0.1",
    port,
    strictPort: true,
    fs: { allow: fsAllow },
  },
});
