import { execFileSync } from "node:child_process"
import { expect, it } from "vitest"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { Button } from "@/components/ui/button"
import { MarkdownContent } from "./TaskMarkdown"

it("preserves task sheet positioning, button colors and Markdown containment after CSS minification", () => {
  const style = document.createElement("style")
  // Start with the centered DialogContent position that the sheet must override.
  style.textContent = '[data-slot="dialog-content"] { left: 50%; top: 50%; translate: -50% -50%; }\n'
  // Model the Button utility color without relying on jsdom's Tailwind layer support.
  style.textContent += '.bg-primary { background: rgb(10, 20, 30); }\n'
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
    const button = renderToStaticMarkup(createElement(Button, null, "Send comment"))
    const markdown = renderToStaticMarkup(createElement(MarkdownContent, { children: "long".repeat(200) }))
    dialog.innerHTML = `${button}<section class="task-comments"><form>${button}</form><div class="task-comment-list"><article>${markdown}</article></div></section>`
    const buttons = dialog.querySelectorAll("button")
    expect(getComputedStyle(buttons[0]).backgroundColor).toBe("rgb(10, 20, 30)")
    expect(getComputedStyle(buttons[1]).backgroundColor).toBe(getComputedStyle(buttons[0]).backgroundColor)
    expect(getComputedStyle(dialog.querySelector("article > div")!).overflowWrap).toBe("anywhere")
    for (const selector of [".task-comment-list", ".task-comments form"]) {
      expect(getComputedStyle(dialog.querySelector(selector)!).gridTemplateColumns).toBe("minmax(0,1fr)")
    }
    expect(getComputedStyle(dialog).translate).toBe("none")
    expect(getComputedStyle(dialog).width).toBe("var(--tasks-detail-width,50vw)")
  } finally {
    dialog.remove()
    style.remove()
  }
}, 20_000)
