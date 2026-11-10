import { readFileSync, readdirSync } from "node:fs";
import { join, resolve } from "node:path";
import { describe, expect, it } from "vitest";

// The design system policy. A class names a token of src/index.css and
// src/styles/tokens.css, never a value: no Tailwind default scale, no Tailwind
// palette, no arbitrary color, pixel size, shadow or layer. The Tailwind
// defaults are cleared in `@theme`, so such a class would produce no CSS and
// fail without a sign; this scan is the sign. A source scan, like
// webStoragePolicy.test.ts.
//
// Stories are checked for the names that no longer exist. They may size a demo
// frame in pixels (a 460px field, a 620px page frame).
const ui = resolve(__dirname, "../..");

/** The text sizes and radii that `@theme` declares. */
const TEXT = ["caption", "ui", "body", "title", "heading", "display"];
const RADIUS = ["mark", "chip", "control", "card", "panel"];

const PALETTE =
  "slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose";
const COLOR_UTILITY =
  "bg|text|border|border-[trblxyse]|ring|ring-offset|outline|fill|stroke|from|via|to|divide|decoration|caret|accent|shadow";
const SIDES = "(?:-(?:[trblse]|[tb][lr]|[se][se]))?";

/** A class starts after a quote, a space or a variant colon. */
const START = "(?<=[\"'`\\s:!])";
const END = "(?=[\"'`\\s])";

type Rule = { name: string; pattern: RegExp; stories: boolean };

const RULES: Rule[] = [
  {
    name: `text size outside the scale (use text-${TEXT.join(", text-")})`,
    pattern: new RegExp(
      `${START}text-(?:xs|sm|base|lg|[2-9]?xl|nano|micro|tiny|meta|body-sm|body-lg)${END}`,
      "g",
    ),
    stories: true,
  },
  {
    name: `radius outside the scale (use rounded-${RADIUS.join(", rounded-")}, rounded-full)`,
    pattern: new RegExp(
      `${START}rounded${SIDES}(?:-(?:xs|sm|md|lg|[2-4]?xl|hair))?${END}|rounded${SIDES}-[\\[(](?!inherit)`,
      "g",
    ),
    stories: true,
  },
  {
    // An inset line and a spread without blur are not elevation: a marker on a
    // row, a tab underline, the cut-out of the tour. They stay arbitrary.
    name: "shadow outside the scale (use shadow-raise, shadow-lift, shadow-none)",
    pattern: new RegExp(
      `${START}(?:inset-|drop-)?shadow(?:-(?:2xs|xs|sm|md|lg|2?xl))?${END}|${START}shadow-[\\[(](?!inset_|0_0_0_)`,
      "g",
    ),
    stories: true,
  },
  {
    name: "Tailwind palette color (use a role: destructive, warning, success, primary, muted)",
    pattern: new RegExp(
      `\\b[a-z-]*-(?:${PALETTE})-\\d{2,3}\\b|${START}(?:${COLOR_UTILITY})-(?:black|white)\\b`,
      "g",
    ),
    stories: true,
  },
  {
    name: "arbitrary color in a class (add a role to styles/tokens.css)",
    pattern: /-\[(?:#|color-mix|oklch|oklab|rgb|hsl)/g,
    stories: true,
  },
  {
    name: "arbitrary size in a class (use the 4px spacing scale: h-6.5 is 26px)",
    pattern: /\b[a-z][a-z0-9-]*-\[-?\d+(?:\.\d+)?(?:px|rem|em)\](?!:)/g,
    stories: false,
  },
  {
    name: "layer outside the scale (use z-sticky, z-overlay, z-dialog, z-alert; z-0 to z-9 inside a component)",
    pattern: new RegExp(`${START}z-(?:\\d{2,}|\\[[^\\]]*\\])${END}`, "g"),
    stories: false,
  },
  {
    name: "tint written by hand (use -soft, -mid, -line of the tone, or bg-subtle)",
    pattern: new RegExp(
      `${START}(?:${COLOR_UTILITY})-(?:primary|destructive|warning|success|muted|status-[a-z]+)/(?:\\d+|\\[)`,
      "g",
    ),
    stories: false,
  },
];

/** Values that a library needs as a literal. Each entry says why. */
const LITERAL_COLOR_OWNERS = [
  // xterm.js takes its theme as color strings, not as CSS variables.
  join("src", "components", "tui-screen", "useXtermTerminal.ts"),
];
const literalColor =
  /#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{3}\b(?![0-9a-zA-Z])|oklch\(|\brgba?\(|\bhsla?\(/g;

function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return entry.name === "test" ? [] : sources(path);
    return /\.(ts|tsx)$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name) ? [path] : [];
  });
}

function codeLines(file: string): { text: string; line: number }[] {
  return readFileSync(file, "utf8")
    .split("\n")
    .map((text, index) => ({ text, line: index + 1 }))
    .filter(({ text }) => {
      const code = text.trim();
      return !(code.startsWith("//") || code.startsWith("*") || code.startsWith("/*"));
    });
}

const files = ["src", "store"].flatMap((dir) => sources(join(ui, dir)));
const isStory = (file: string) => /\.stories\.tsx$/.test(file) || file.includes("/mockups/");

describe("design system policy", () => {
  for (const rule of RULES) {
    it(`has no ${rule.name}`, () => {
      const found: string[] = [];
      for (const file of files) {
        if (isStory(file) && !rule.stories) continue;
        for (const { text, line } of codeLines(file))
          for (const match of text.matchAll(rule.pattern))
            found.push(`${file.slice(ui.length + 1)}:${line} ${match[0]}`);
      }
      expect(found).toEqual([]);
    });
  }

  it("has no color literal in production code", () => {
    const found: string[] = [];
    for (const file of files) {
      const where = file.slice(ui.length + 1);
      if (isStory(file) || LITERAL_COLOR_OWNERS.includes(where)) continue;
      for (const { text, line } of codeLines(file))
        for (const match of text.matchAll(literalColor)) found.push(`${where}:${line} ${match[0]}`);
    }
    expect(found).toEqual([]);
  });
});
