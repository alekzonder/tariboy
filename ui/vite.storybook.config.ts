import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";
import { fsAllow } from "./vite.fsAllow";

// Storybook's own Vite file. vite.config.ts proxies /api to the daemon at
// 127.0.0.1:9990, and Storybook reads it by default. Stories must never reach a
// daemon, so this file has the app's plugins and alias and no server.proxy.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(__dirname, "src") } },
  server: { fs: { allow: fsAllow } },
  // Drops the implicit same-origin host "This daemon (local)" (lib/aggregate.ts).
  // Its empty base URL would send /api requests to the Storybook origin.
  define: { "import.meta.env.VITE_TARIBOY_NO_LOCAL_HOST": '"1"' },
});
