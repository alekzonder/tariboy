import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

// The theme is a flat list of custom properties, so it is testable as text.
// The values are in src/styles/tokens.css and the utility names in
// src/index.css. Properties that cannot be seen from a component test and are
// easy to lose in a later edit are pinned here.
const tokens = readFileSync(resolve("src/styles/tokens.css"), "utf8");
const theme = readFileSync(resolve("src/index.css"), "utf8") + tokens;

/** The custom properties that a rule block of tokens.css declares. */
function declared(selector: string): Set<string> {
  const start = tokens.indexOf(`\n${selector} {`);
  if (start < 0) throw new Error(`no ${selector} block in tokens.css`);
  const block = tokens.slice(start, tokens.indexOf("\n}", start));
  return new Set([...block.matchAll(/^\s*(--[a-z0-9-]+):/gm)].map((m) => m[1]));
}

/** The angle of a `--hue-*` knob. */
function hue(name: string): number {
  const match = tokens.match(new RegExp(`--hue-${name}:\\s*([\\d.]+);`));
  if (!match) throw new Error(`no --hue-${name} in tokens.css`);
  return Number(match[1]);
}

/** Every `oklch(<lightness> <chroma> <hue>` triple, with its line. A hue is a
 *  number or a `var(--hue-*)` knob. */
function oklchColors(): { line: string; chroma: number; hue: number }[] {
  const found: { line: string; chroma: number; hue: number }[] = [];
  for (const line of theme.split("\n")) {
    for (const match of line.matchAll(
      /oklch\(\s*[\d.]+%?\s+([\d.]+)\s+(?:([\d.]+)|var\(--hue-([a-z]+)\))/g,
    )) {
      found.push({
        line: line.trim(),
        chroma: Number(match[1]),
        hue: match[2] === undefined ? hue(match[3]) : Number(match[2]),
      });
    }
  }
  return found;
}

describe("metal theme", () => {
  it("has no warm-sand hue left on a neutral colour", () => {
    // Neutrals and shadows are the chrome; a hue between 60 and 120 is the sand
    // the metal palette replaced. Chromatic accents and statuses are exempt.
    expect(oklchColors().length).toBeGreaterThan(40);
    const warm = oklchColors().filter(
      (color) => color.chroma <= 0.05 && color.hue >= 60 && color.hue <= 120,
    );
    expect(warm.map((color) => color.line)).toEqual([]);
  });

  it("keeps the Tailwind background utility on the flat base, not the gradient", () => {
    // `--background` is a gradient; `bg-background` compiles to background-color,
    // which drops a gradient value on the floor. The utility therefore has to
    // resolve to the flat `--background-base`.
    expect(theme).toMatch(/--color-background:\s*var\(--background-base\)/);
    expect(theme).toMatch(/--background:\s*linear-gradient\(/);
    expect(theme).toMatch(/--background-base:\s*oklch\(/);
  });

  it("gives every role a value in the dark theme", () => {
    // A role that only :root declares keeps its light value under `.dark`. The
    // hue knobs and the terminal, which is dark in both themes, are shared on
    // purpose.
    const shared = (name: string) => name.startsWith("--hue-") || name.startsWith("--terminal");
    const dark = declared(".dark");
    const missing = [...declared(":root")].filter((name) => !shared(name) && !dark.has(name));
    expect(missing).toEqual([]);
  });

  it("maps every role onto a utility", () => {
    // A role without a `--color-*` or `--shadow-*` line in index.css has no
    // class, so a component would have to spell out var(--role).
    const index = readFileSync(resolve("src/index.css"), "utf8");
    const knob = (name: string) =>
      /^--(hue|tint)-/.test(name) || name === "--background" || name === "--background-base";
    const unmapped = [...declared(":root")].filter(
      (name) => !knob(name) && !index.includes(`var(${name})`),
    );
    expect(unmapped).toEqual([]);
  });

  it("clears the Tailwind default scales", () => {
    const index = readFileSync(resolve("src/index.css"), "utf8");
    for (const scale of ["text", "radius", "shadow", "color"])
      expect(index, scale).toMatch(new RegExp(`--${scale}-\\*:\\s*initial;`));
  });
});
