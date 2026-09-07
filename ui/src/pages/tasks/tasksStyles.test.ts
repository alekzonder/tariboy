import { execFileSync } from "node:child_process"
import { expect, it } from "vitest"

it("preserves the fullscreen dialog translation reset after production CSS minification", () => {
  const style = document.createElement("style")
  // Start with the centered DialogContent translation that fullscreen must override.
  style.textContent = '[data-slot="dialog-content"] { translate: -50% -50%; }\n'
  // Rolldown's native bindings need a real Node realm, outside Vitest's VM pool.
  style.textContent += execFileSync(process.execPath, ["--input-type=module", "-e", `
    import { build } from "vite";
    const result = await build({
      configFile: "vite.desktop.config.ts",
      logLevel: "silent",
      build: { write: false, rollupOptions: { input: "src/pages/tasks/tasks.css" } },
    });
    for (const asset of result.output) {
      if (asset.type === "asset" && asset.fileName.endsWith(".css")) {
        process.stdout.write(asset.source);
      }
    }
  `], { encoding: "utf8", timeout: 20_000 })
  const dialog = document.createElement("div")
  dialog.className = "task-detail-dialog"
  dialog.dataset.slot = "dialog-content"
  document.head.append(style)
  document.body.append(dialog)
  try {
    // A transform reset cannot cancel the separate translate property used by Tailwind.
    expect(getComputedStyle(dialog).translate).toBe("none")
  } finally {
    dialog.remove()
    style.remove()
  }
}, 20_000)
