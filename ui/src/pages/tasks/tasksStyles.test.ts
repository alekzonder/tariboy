import { execFileSync } from "node:child_process"
import { expect, it } from "vitest"

it("keeps the task dialog as a right-side sheet after production CSS minification", () => {
  const style = document.createElement("style")
  // Start with the centered DialogContent position that the sheet must override.
  style.textContent = '[data-slot="dialog-content"] { left: 50%; top: 50%; translate: -50% -50%; }\n'
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
    expect(getComputedStyle(dialog).translate).toBe("none")
    expect(getComputedStyle(dialog).width).toBe("var(--tasks-detail-width,50vw)")
  } finally {
    dialog.remove()
    style.remove()
  }
}, 20_000)
