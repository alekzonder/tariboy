import type { CSSProperties } from "react"
import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/** Whether `value` is a full `#rrggbb` hex color (case-insensitive). */
export function isValidHex(value: string): boolean {
  return /^#[0-9a-fA-F]{6}$/.test(value.trim())
}

/** Parse a `#rrggbb` hex into `[r, g, b]` (0-255), or null when invalid. */
function hexToRgb(hex: string): [number, number, number] | null {
  const v = hex.trim()
  if (!isValidHex(v)) return null
  const n = parseInt(v.slice(1), 16)
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255]
}

/** The hue angle (0-359) of a `#rrggbb` hex, or null when invalid. The header
 * tint is derived from this hue so one stored color reads well in both themes
 * (the per-theme lightness/chroma are fixed in CSS). */
export function hexToHue(hex?: string | null): number | null {
  if (typeof hex !== "string") return null
  const rgb = hexToRgb(hex)
  if (!rgb) return null
  const [r, g, b] = rgb.map((c) => c / 255)
  const max = Math.max(r, g, b)
  const min = Math.min(r, g, b)
  const d = max - min
  if (d === 0) return 0
  let h: number
  if (max === r) h = ((g - b) / d) % 6
  else if (max === g) h = (b - r) / d + 2
  else h = (r - g) / d + 4
  return ((Math.round(h * 60) % 360) + 360) % 360
}

/** Header-tint style: feeds the agent color's hue into the `--agent-hue` custom
 * property, consumed by the `.agent-header` CSS to derive a theme-aware tint.
 * Returns `undefined` for an invalid/absent color so callers fall back to the
 * neutral default. */
export function colorStyle(hex?: string | null): CSSProperties | undefined {
  const hue = hexToHue(hex)
  if (hue == null) return undefined
  return { "--agent-hue": String(hue) } as CSSProperties
}

/** Swatch style: shows the raw picked hex as the background (and border). Returns
 * `undefined` for an invalid/absent color so callers fall back to the neutral
 * `.agent-swatch` default. */
export function swatchStyle(hex?: string | null): CSSProperties | undefined {
  if (typeof hex !== "string" || !isValidHex(hex)) return undefined
  return { backgroundColor: hex.trim(), borderColor: hex.trim() }
}

/** A random `#rrggbb` hex. */
export function randomHex(): string {
  const n = Math.floor(Math.random() * 0x1000000)
  return "#" + n.toString(16).padStart(6, "0")
}

/** The label to show for an agent everywhere a name is rendered: `<alias> (<name>)`
 * when an alias is set, otherwise just `<name>`. Whitespace-only aliases are
 * treated as unset. */
export function displayName(agent: { name: string; alias?: string }): string {
  const alias = agent.alias?.trim()
  return alias ? `${alias} (${agent.name})` : agent.name
}
