import { readFileSync } from "node:fs"
import { resolve } from "node:path"
import { describe, expect, it } from "vitest"

// The theme is a flat list of custom properties, so it is testable as text.
// Two properties of the "metal" palette cannot be seen from a component test
// and are easy to lose in a later edit, so they are pinned here.
const theme = readFileSync(resolve("src/index.css"), "utf8")

/** Every `oklch(<lightness> <chroma> <hue>` triple in the file, with its line. */
function oklchColors(): { line: string; chroma: number; hue: number }[] {
  const found: { line: string; chroma: number; hue: number }[] = []
  for (const line of theme.split("\n")) {
    for (const match of line.matchAll(/oklch\(\s*[\d.]+%?\s+([\d.]+)\s+([\d.]+)/g)) {
      found.push({ line: line.trim(), chroma: Number(match[1]), hue: Number(match[2]) })
    }
  }
  return found
}

describe("metal theme", () => {
  it("has no warm-sand hue left on a neutral colour", () => {
    // Neutrals and shadows are the chrome; a hue between 60 and 120 is the sand
    // the metal palette replaced. Chromatic accents and statuses are exempt.
    const warm = oklchColors().filter(
      (color) => color.chroma <= 0.05 && color.hue >= 60 && color.hue <= 120,
    )
    expect(warm.map((color) => color.line)).toEqual([])
  })

  it("keeps the Tailwind background utility on the flat base, not the gradient", () => {
    // `--background` is a gradient; `bg-background` compiles to background-color,
    // which drops a gradient value on the floor. The utility therefore has to
    // resolve to the flat `--background-base`.
    expect(theme).toMatch(/--color-background:\s*var\(--background-base\)/)
    expect(theme).toMatch(/--background:\s*linear-gradient\(/)
    expect(theme).toMatch(/--background-base:\s*oklch\(/)
  })
})
