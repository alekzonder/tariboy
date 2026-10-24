import { execFileSync } from "node:child_process"
import { existsSync, mkdtempSync, readFileSync, readdirSync, rmSync } from "node:fs"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { afterAll, expect, it } from "vitest"

const ui = resolve(__dirname, "../..")
const designSync = join(ui, ".design-sync")
const config = JSON.parse(readFileSync(join(designSync, "config.json"), "utf8")) as {
  componentSrcMap: Record<string, string | null>
  overrides: Record<string, unknown>
}
const scratch = mkdtempSync(join(tmpdir(), "design-sync-entry-"))
afterAll(() => rmSync(scratch, { recursive: true, force: true }))

it("keeps the committed entry equal to a fresh generation", () => {
  const out = join(scratch, "entry.tsx")
  execFileSync(process.execPath, [join(designSync, "make-entry.mjs"), "--out", out], { stdio: "pipe" })
  expect(readFileSync(out, "utf8")).toBe(readFileSync(join(designSync, "entry.tsx"), "utf8"))
})

it("maps every catalog component to an existing source file", () => {
  for (const [name, source] of Object.entries(config.componentSrcMap)) {
    if (source === null) continue
    expect(existsSync(join(ui, source)), `${name} -> ${source}`).toBe(true)
  }
})

it("keeps previews and overrides limited to mapped components", () => {
  const mapped = new Set(Object.keys(config.componentSrcMap))
  const previews = readdirSync(join(designSync, "previews")).map((file) => file.replace(/\.tsx$/, ""))
  expect(previews.filter((name) => !mapped.has(name))).toEqual([])
  expect(Object.keys(config.overrides).filter((name) => !mapped.has(name))).toEqual([])
})
